/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/heptiolabs/healthcheck"
	clusterv1alpha1conversions "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1/conversions"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/node/eviction"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata"
	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterv1alpha1clientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	machinescheme "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/scheme"
	clusterlistersv1alpha1 "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
)

const (
	FinalizerDeleteInstance = "machine-delete-finalizer"
	FinalizerDeleteNode     = "machine-node-delete-finalizer"

	deletionRetryWaitPeriod = 10 * time.Second

	latestKubernetesVersion = "1.9.6"

	NodeOwnerLabelName = "machine-controller/owned-by"
)

// Controller is the controller implementation for machine resources
type Controller struct {
	kubeClient    kubernetes.Interface
	machineClient clusterv1alpha1clientset.Interface

	nodesLister          listerscorev1.NodeLister
	machinesLister       clusterlistersv1alpha1.MachineLister
	secretSystemNsLister listerscorev1.SecretLister

	workqueue workqueue.RateLimitingInterface
	recorder  record.EventRecorder

	clusterDNSIPs      []net.IP
	metrics            *MetricsCollection
	kubeconfigProvider KubeconfigProvider

	validationCache      map[string]bool
	validationCacheMutex sync.Mutex

	name string
}

type KubeconfigProvider interface {
	GetKubeconfig() (*clientcmdapi.Config, error)
}

// MetricsCollection is a struct of all metrics used in
// this controller.
type MetricsCollection struct {
	Workers prometheus.Gauge
	Errors  prometheus.Counter
}

// NewMachineController returns a new machine controller
func NewMachineController(
	kubeClient kubernetes.Interface,
	machineClient clusterv1alpha1clientset.Interface,
	nodeInformer cache.SharedIndexInformer,
	nodeLister listerscorev1.NodeLister,
	machineInformer cache.SharedIndexInformer,
	machineLister clusterlistersv1alpha1.MachineLister,
	secretSystemNsLister listerscorev1.SecretLister,
	clusterDNSIPs []net.IP,
	metrics *MetricsCollection,
	prometheusRegistry prometheus.Registerer,
	kubeconfigProvider KubeconfigProvider,
	name string) *Controller {

	machinescheme.AddToScheme(scheme.Scheme)
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.V(4).Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})

	if prometheusRegistry != nil {
		prometheusRegistry.MustRegister(metrics.Errors)
		prometheusRegistry.MustRegister(metrics.Workers)
	}

	controller := &Controller{
		kubeClient:  kubeClient,
		nodesLister: nodeLister,

		machineClient:        machineClient,
		machinesLister:       machineLister,
		secretSystemNsLister: secretSystemNsLister,

		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 5*time.Minute), "Machines"),
		recorder:  eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "machine-controller"}),

		clusterDNSIPs:      clusterDNSIPs,
		metrics:            metrics,
		kubeconfigProvider: kubeconfigProvider,
		validationCache:    map[string]bool{},

		name: name,
	}

	machineInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueMachine,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueMachine(new)
		},
	})

	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newNode := new.(*corev1.Node)
			oldNode := old.(*corev1.Node)
			if newNode.ResourceVersion == oldNode.ResourceVersion {
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	utilruntime.ErrorHandlers = append(utilruntime.ErrorHandlers, func(err error) {
		controller.metrics.Errors.Add(1)
	})

	return controller
}

// Run starts the control loop
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	c.metrics.Workers.Set(float64(threadiness))

	<-stopCh
	return nil
}

func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// clearMachineError is a convenience function to remove a error on the machine if its set.
// It does not return an error as it's used around the sync handler
func (c *Controller) clearMachineError(key string) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to split metaNamespaceKey: %v", err))
		return
	}
	listerMachine, err := c.machinesLister.Machines(namespace).Get(name)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to get Machine from lister: %v", err))
		return
	}
	machine := listerMachine.DeepCopy()

	if machine.Status.ErrorMessage != nil || machine.Status.ErrorReason != nil {
		_, err := c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			m.Status.ErrorMessage = nil
			m.Status.ErrorReason = nil
		})
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to update machine: %v", err))
			return
		}
	}
}

func (c *Controller) processNextWorkItem() bool {
	key, quit := c.workqueue.Get()
	if quit {
		return false
	}

	defer c.workqueue.Done(key)

	glog.V(6).Infof("Processing machine: %s", key)
	err := c.syncHandler(key.(string))
	if err == nil {
		// Every time we successfully sync a Machine, we should check if we should remove the error if its set
		c.clearMachineError(key.(string))
		c.workqueue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed with: %v", key, err))
	c.workqueue.AddRateLimited(key)

	return true
}

func (c *Controller) nodeIsReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

func (c *Controller) getNodeByNodeRef(nodeRef *corev1.ObjectReference) (*corev1.Node, error) {
	listerNode, err := c.nodesLister.Get(nodeRef.Name)
	if err != nil {
		return nil, err
	}
	return listerNode.DeepCopy(), nil
}

func (c *Controller) updateMachine(machine *clusterv1alpha1.Machine, modify func(*clusterv1alpha1.Machine)) (*clusterv1alpha1.Machine, error) {
	var updatedMachine *clusterv1alpha1.Machine

	// Both machine and updatedMachine can be nil later on, so we store the namespace and name here
	namespace := machine.Namespace
	name := machine.Name
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var getErr, retryErr error

		// Apply modifications
		// machine could be nil here if the get after a failed update failed
		if machine == nil {
			machine, getErr = c.machineClient.ClusterV1alpha1().Machines(namespace).Get(name, metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
		}
		modify(machine)

		// Update the machine, if that fails, get the latest version from the api
		// we deliberately try to update first via the provided machine object
		// to not have at least one get call per update as we do a lot of updates
		// which gets us ratelimited very quickly which then creates issues when there is an actual conflict
		updatedMachine, retryErr = c.machineClient.ClusterV1alpha1().Machines(namespace).Update(machine)
		if retryErr != nil {
			//Get latest version from API
			machine, getErr = c.machineClient.ClusterV1alpha1().Machines(namespace).Get(name, metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			return retryErr
		}
		return retryErr

	})

	return updatedMachine, err
}

// updateMachine updates machine's ErrorMessage and ErrorReason regardless if they were set or not
// this essentially overwrites previous values
func (c *Controller) updateMachineError(machine *clusterv1alpha1.Machine, reason common.MachineStatusError, message string) (*clusterv1alpha1.Machine, error) {
	return c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		m.Status.ErrorMessage = &message
		m.Status.ErrorReason = &reason
	})
}

// updateMachineErrorIfTerminalError is a convenience method that will update machine's Status if the given err is terminal
// and at the same time terminal error will be returned to the caller
// otherwise it will return formatted error according to errMsg
func (c *Controller) updateMachineErrorIfTerminalError(machine *clusterv1alpha1.Machine, stReason common.MachineStatusError, stMessage string, err error, errMsg string) error {
	c.recorder.Eventf(machine, corev1.EventTypeWarning, string(stReason), stMessage)
	if ok, _, _ := cloudprovidererrors.IsTerminalError(err); ok {
		if _, errNested := c.updateMachineError(machine, stReason, stMessage); errNested != nil {
			return fmt.Errorf("failed to update machine error after due to %v, terminal error = %v", errNested, stMessage)
		}
		return err
	}
	return fmt.Errorf("%s, due to %v", errMsg, err)
}

func (c *Controller) createProviderInstance(prov cloud.Provider, machine *clusterv1alpha1.Machine, userdata string) (instance.Instance, error) {
	// Ensure finalizer is there
	machine, err := c.ensureDeleteFinalizerExists(machine)
	if err != nil {
		return nil, err
	}
	return prov.Create(machine, c.updateMachine, userdata)
}

func (c *Controller) validateMachine(prov cloud.Provider, machine *clusterv1alpha1.Machine) error {
	err := prov.Validate(machine.Spec)
	if err != nil {
		c.recorder.Eventf(machine, corev1.EventTypeWarning, "ValidationFailed", "Validation failed: %v", err)
		return err
	}
	c.recorder.Event(machine, corev1.EventTypeNormal, "ValidationSucceeded", "Validation succeeded")
	return nil
}

func (c *Controller) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("failed to split metaNamespaceKey: %v", err)
	}
	listerMachine, err := c.machinesLister.Machines(namespace).Get(name)
	if err != nil {
		if kerrors.IsNotFound(err) {
			glog.V(2).Infof("machine '%s' in work queue no longer exists", key)
			return nil
		}
		return err
	}
	machine := listerMachine.DeepCopy()

	// Add type revision annotation to be able
	// to migrate in case there is a backwards-incompatible change on the upstream machine types
	if machine.Annotations == nil {
		machine.Annotations = map[string]string{}
	}
	if _, ok := machine.Annotations[clusterv1alpha1conversions.TypeRevisionAnnotationName]; !ok {
		machine, err := c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			if m.Annotations == nil {
				m.Annotations = map[string]string{}
			}
			m.Annotations[clusterv1alpha1conversions.TypeRevisionAnnotationName] = clusterv1alpha1conversions.TypeRevisionCurrentVersion
		})
		if err != nil {
			return fmt.Errorf("failed to set type revision annotation on machine: %v", err)
		}
		glog.V(4).Infof("Set type revision annotation on machine %s/%s to %s",
			machine.Namespace, machine.Name, clusterv1alpha1conversions.TypeRevisionCurrentVersion)
	}

	// step 1: verify machine spec and provider config
	if machine.Spec.Name == "" {
		machine, err = c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			m.Spec.Name = m.Name
		})
		if err != nil {
			return fmt.Errorf("failed to default machine.Spec.Name to %s: %v", listerMachine.Name, err)
		}
		c.recorder.Eventf(machine, corev1.EventTypeNormal, "NodeName defaulted", "Defaulted nodename to %s", machine.Name)
	}

	providerConfig, err := providerconfig.GetConfig(machine.Spec.ProviderConfig)
	if err != nil {
		return fmt.Errorf("failed to get provider config: %v", err)
	}
	skg := providerconfig.NewConfigVarResolver(c.kubeClient)
	prov, err := cloudprovider.ForProvider(providerConfig.CloudProvider, skg)
	if err != nil {
		return fmt.Errorf("failed to get cloud provider %q: %v", providerConfig.CloudProvider, err)
	}

	// step 2: check if a user requested to delete the machine
	if machine.DeletionTimestamp != nil {
		if err := c.deleteMachine(prov, machine); err != nil {
			return err
		}
		// As the deletion got triggered but the instance might not been gone yet, we need to recheck in a few seconds.
		c.enqueueMachineAfter(machine, deletionRetryWaitPeriod)
		return nil
	}

	// step 3: essentially creates an instance for the given machine
	userdataProvider, err := userdata.ForOS(providerConfig.OperatingSystem)
	if err != nil {
		return fmt.Errorf("failed to userdata provider for '%s': %v", providerConfig.OperatingSystem, err)
	}

	// We use a new variable here to be able to put the Event on the machine even thought
	// c.defaultMachine returns a nil pointer for the machine in case of an error
	defaultedMachine, err := c.defaultMachine(machine, userdataProvider)
	if err != nil {
		errorMessage := fmt.Sprintf("failed to default the Machine specs: %v", err)
		c.recorder.Event(machine, corev1.EventTypeWarning, "MachineDefaultingFailed", errorMessage)
		return errors.New(errorMessage)
	}
	machine = defaultedMachine

	// case 3.2: creates an instance if there is no node associated with the given machine
	if machine.Status.NodeRef == nil {
		return c.ensureInstanceExistsForMachine(prov, machine, userdataProvider, providerConfig)
	}

	node, err := c.getNodeByNodeRef(machine.Status.NodeRef)
	if err != nil {
		//In case we cannot find a node for the NodeRef we must remove the NodeRef & recreate an instance on the next sync
		if kerrors.IsNotFound(err) {
			glog.V(4).Infof("found invalid NodeRef on machine %s. Deleting reference...", machine.Name)
			_, err = c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
				m.Status.NodeRef = nil
			})
			return err
		}
		return fmt.Errorf("failed to check if node for machine exists: '%s'", err)
	}

	if c.nodeIsReady(node) {
		// We must do this to ensure the informers in the machineSet and machineDeployment controller
		// get triggered as soon as a ready node exists for a machine
		if machine, err = c.ensureMachineHasNodeReadyCondition(machine); err != nil {
			return fmt.Errorf("failed to set nodeReady condition on machine: %v", err)
		}
	} else {
		// Node is not ready anymore? Maybe it got deleted
		return c.ensureInstanceExistsForMachine(prov, machine, userdataProvider, providerConfig)
	}

	// case 3.3: if the node exists make sure if it has labels and taints attached to it.
	return c.ensureNodeLabelsAnnotationsAndTaints(node, machine)
}

func (c *Controller) ensureMachineHasNodeReadyCondition(machine *clusterv1alpha1.Machine) (*clusterv1alpha1.Machine, error) {
	for _, condition := range machine.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return machine, nil
		}
	}
	return c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		m.Status.Conditions = append(m.Status.Conditions, corev1.NodeCondition{Type: corev1.NodeReady,
			Status: corev1.ConditionTrue,
		})
	})
}

// deleteMachine makes sure that an instance has gone in a series of steps.
func (c *Controller) deleteMachine(prov cloud.Provider, machine *clusterv1alpha1.Machine) error {
	if machine.Status.NodeRef != nil {
		_, err := c.nodesLister.Get(machine.Status.NodeRef.Name)
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return fmt.Errorf("failed to get node %s for machine %s/%s: %v", machine.Status.NodeRef.Name, machine.Namespace, machine.Name, err)
			}
			// if kerrors.IsNotFound(err) => continue by deleting cloud provider instance
			// only if err == nil => evict node
		} else if err := eviction.New(machine.Status.NodeRef.Name, c.nodesLister, c.kubeClient).Run(); err != nil {
			return fmt.Errorf("failed to evict node %s: %v", machine.Status.NodeRef.Name, err)
		}
	}

	if err := c.deleteCloudProviderInstance(prov, machine); err != nil {
		c.recorder.Eventf(machine, corev1.EventTypeWarning, "DeletionFailed", "Failed to delete instance at cloud provider: %v", err)
		return err
	}

	// Delete the node object only after the instance is gone, `deleteCloudProviderInstance`
	// returns with a nil-error after it triggers the instance deletion but it is async for
	// some providers hence the instance deletion may not been executed yet
	// `FinalizerDeleteInstance` stays until the instance is really gone thought, so we check
	// for that here
	if sets.NewString(machine.Finalizers...).Has(FinalizerDeleteInstance) {
		return nil
	}

	if err := c.deleteNodeForMachine(machine); err != nil {
		c.recorder.Eventf(machine, corev1.EventTypeWarning, "DeletionFailed", "Failed to delete node: %v", err)
		return err
	}

	return nil
}

func (c *Controller) deleteCloudProviderInstance(prov cloud.Provider, machine *clusterv1alpha1.Machine) error {
	finalizers := sets.NewString(machine.Finalizers...)
	if !finalizers.Has(FinalizerDeleteInstance) {
		return nil
	}

	// Retrieve the instance from the cloud provider
	if _, err := prov.Get(machine); err != nil {
		if err == cloudprovidererrors.ErrInstanceNotFound {
			// Only remove the finalizers if the instance is really gone. This ensures that consumers of this API can safely do follow up actions.
			machine, err = c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
				finalizers.Delete(FinalizerDeleteInstance)
				m.Finalizers = finalizers.List()
			})
			return err
		}

		message := fmt.Sprintf("%v. Please manually delete the instance at the cloud provider and remove the %s finalizer from the machine object.", err, FinalizerDeleteInstance)
		return c.updateMachineErrorIfTerminalError(machine, common.DeleteMachineError, message, err, "failed to retrieve instance from cloud provider")
	}

	// Delete the instance
	if err := prov.Delete(machine, c.updateMachine); err != nil {
		if err == cloudprovidererrors.ErrInstanceNotFound {
			// Only remove the finalizers if the instance is really gone. This ensures that consumers of this API can safely do follow up actions.
			return nil
		}

		message := fmt.Sprintf("%v. Please manually delete %s finalizer from the machine object.", err, FinalizerDeleteInstance)
		return c.updateMachineErrorIfTerminalError(machine, common.DeleteMachineError, message, err, "failed to delete machine at cloud provider")
	}
	return nil
}

func ownedNodesPredicateFactory(machine *clusterv1alpha1.Machine) func(*corev1.Node) bool {
	return func(node *corev1.Node) bool {
		labels := node.GetLabels()
		if labels == nil {
			return false
		}
		if ownerUID, exists := labels[NodeOwnerLabelName]; exists && string(machine.UID) == ownerUID {
			return true
		}
		return false
	}
}

func (c *Controller) deleteNodeForMachine(machine *clusterv1alpha1.Machine) error {
	nodesList, err := c.nodesLister.ListWithPredicate(ownedNodesPredicateFactory(machine))
	if err != nil {
		return fmt.Errorf("failed to list nodes: %v", err)
	}

	for _, node := range nodesList {
		if err := c.kubeClient.CoreV1().Nodes().Delete(node.Name, nil); err != nil {
			return err
		}
	}

	finalizers := sets.NewString(machine.Finalizers...)
	if finalizers.Has(FinalizerDeleteNode) {
		machine, err = c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			finalizers.Delete(FinalizerDeleteNode)
			m.Finalizers = finalizers.List()
		})
	}

	return err
}

func (c *Controller) ensureInstanceExistsForMachine(prov cloud.Provider, machine *clusterv1alpha1.Machine, userdataProvider userdata.Provider, providerConfig *providerconfig.Config) error {
	glog.V(6).Infof("Requesting instance for machine '%s' from cloudprovider because no associated node with status ready found...", machine.Name)
	// case 1: validate the machine spec before getting the instance from cloud provider.
	// even though this is a little bit premature and inefficient, it helps us detect invalid specification
	defaultedMachineSpec, changed, err := prov.AddDefaults(machine.Spec)
	if err != nil {
		return c.updateMachineErrorIfTerminalError(machine, common.InvalidConfigurationMachineError, err.Error(), err, "failed to add defaults to machine")
	}
	if changed {
		glog.V(4).Infof("updating machine '%s' with defaults...", machine.Name)
		c.recorder.Event(machine, corev1.EventTypeNormal, "Defaulted", "Updated machine with defaults")
		if machine, err = c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			m.Spec = defaultedMachineSpec
		}); err != nil {
			return fmt.Errorf("failed to update machine '%s' after adding defaults: '%v'", machine.Name, err)
		}

		glog.V(4).Infof("Successfully updated machine '%s' with defaults!", machine.Name)
	}

	cacheKey := string(machine.UID) + machine.ResourceVersion
	c.validationCacheMutex.Lock()
	validationSuccess := c.validationCache[cacheKey]
	c.validationCacheMutex.Unlock()
	if !validationSuccess {
		if err := c.validateMachine(prov, machine); err != nil {
			if _, errNested := c.updateMachineError(machine, common.InvalidConfigurationMachineError, err.Error()); errNested != nil {
				return fmt.Errorf("failed to update machine error after failed validation: %v", errNested)
			}
			return fmt.Errorf("invalid provider config: %v", err)
		}
		c.validationCacheMutex.Lock()
		c.validationCache[cacheKey] = true
		c.validationCacheMutex.Unlock()
	} else {
		glog.V(6).Infof("Skipping validation as the machine was already successfully validated before")
	}
	providerInstance, err := prov.Get(machine)

	// case 2: retrieving instance from provider was not successful
	if err != nil {
		//First invalidate the validation cache to make sure we run the validation on the next sync.
		//This might happen in case the user invalidates his provider credentials...
		c.validationCacheMutex.Lock()
		c.validationCache[cacheKey] = false
		c.validationCacheMutex.Unlock()

		// case 2.1: instance was not found and we are going to create one
		if err == cloudprovidererrors.ErrInstanceNotFound {
			glog.V(4).Infof("Validated machine spec of %s", machine.Name)

			kubeconfig, err := c.createBootstrapKubeconfig(machine.Name)
			if err != nil {
				c.recorder.Eventf(machine, corev1.EventTypeWarning, "CreateBootstrapKubeconfigFailed", "Creating bootstrap kubeconfig failed: %v", err)
				return fmt.Errorf("failed to create bootstrap kubeconfig: %v", err)
			}

			userdata, err := userdataProvider.UserData(machine.Spec, kubeconfig, prov, c.clusterDNSIPs)
			if err != nil {
				c.recorder.Eventf(machine, corev1.EventTypeWarning, "UserdataRenderingFailed", "Userdata rendering failed: %v", err)
				return fmt.Errorf("failed get userdata: %v", err)
			}

			// Create the instance
			if providerInstance, err = c.createProviderInstance(prov, machine, userdata); err != nil {
				c.recorder.Eventf(machine, corev1.EventTypeWarning, "CreateInstanceFailed", "Instance creation failed: %v", err)
				message := fmt.Sprintf("%v. Unable to create a machine.", err)
				return c.updateMachineErrorIfTerminalError(machine, common.CreateMachineError, message, err, "failed to create machine at cloudprover")
			}
			c.recorder.Event(machine, corev1.EventTypeNormal, "Created", "Successfully created instance")
			glog.V(4).Infof("Created machine %s at cloud provider", machine.Name)
			return nil
		}

		// case 2.2: terminal error was returned and manual interaction is required to recover
		if ok, _, message := cloudprovidererrors.IsTerminalError(err); ok {
			message = fmt.Sprintf("%v. Unable to create a machine.", err)
			return c.updateMachineErrorIfTerminalError(machine, common.CreateMachineError, message, err, "failed to get instance from provider")
		}

		// case 2.3: transient error was returned, requeue the request and try again in the future
		return fmt.Errorf("failed to get instance from provider: %v", err)
	}
	// Instance exists, so ensure finalizer does as well
	machine, err = c.ensureDeleteFinalizerExists(machine)
	if err != nil {
		return err
	}

	// case 3: retrieving the instance from cloudprovider was successfull
	// Emit an event and update .Status.Addresses
	addresses := providerInstance.Addresses()
	eventMessage := fmt.Sprintf("Found instance at cloud provider, addresses: %v", addresses)
	c.recorder.Event(machine, corev1.EventTypeNormal, "InstanceFound", eventMessage)
	machineAddresses := []corev1.NodeAddress{}
	for _, address := range addresses {
		machineAddresses = append(machineAddresses, corev1.NodeAddress{Address: address})
	}
	machine, err = c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		m.Status.Addresses = machineAddresses
	})
	if err != nil {
		return fmt.Errorf("failed to update machine after setting .status.addresses: %v", err)
	}
	return c.ensureNodeOwnerRefAndConfigSource(providerInstance, machine, providerConfig)
}

func (c *Controller) ensureNodeOwnerRefAndConfigSource(providerInstance instance.Instance, machine *clusterv1alpha1.Machine, providerConfig *providerconfig.Config) error {
	node, exists, err := c.getNode(providerInstance, providerConfig.CloudProvider)
	if err != nil {
		return fmt.Errorf("failed to get node for machine %s: %v", machine.Name, err)
	}
	if exists {
		if val := node.Labels[NodeOwnerLabelName]; val != string(machine.UID) {
			if _, err := c.updateNode(node.Name, func(n *corev1.Node) {
				n.Labels[NodeOwnerLabelName] = string(machine.UID)
			}); err != nil {
				return err
			}
		}

		if node.Spec.ConfigSource == nil && machine.Spec.ConfigSource != nil {
			if _, err := c.updateNode(node.Name, func(n *corev1.Node) {
				n.Spec.ConfigSource = machine.Spec.ConfigSource
			}); err != nil {
				return fmt.Errorf("failed to update node %s after setting the config source: %v", node.Name, err)
			}
			glog.V(4).Infof("Added config source to node %s (machine %s)", node.Name, machine.Name)
		}
		err = c.updateMachineStatus(machine, node)
		if err != nil {
			return fmt.Errorf("failed to update machine status: %v", err)
		}
	}
	return nil
}

func (c *Controller) ensureNodeLabelsAnnotationsAndTaints(node *corev1.Node, machine *clusterv1alpha1.Machine) error {
	var labelsUpdated bool
	for k, v := range machine.Spec.Labels {
		if _, exists := node.Labels[k]; !exists {
			labelsUpdated = true
			node.Labels[k] = v
		}
	}

	var annotationsUpdated bool
	for k, v := range machine.Spec.Annotations {
		if _, exists := node.Annotations[k]; !exists {
			annotationsUpdated = true
			node.Annotations[k] = v
		}
	}

	taintExists := func(node *corev1.Node, taint corev1.Taint) bool {
		for _, t := range node.Spec.Taints {
			if t.MatchTaint(&taint) {
				return true
			}
		}
		return false
	}
	var taintsUpdated bool
	for _, t := range machine.Spec.Taints {
		if !taintExists(node, t) {
			node.Spec.Taints = append(node.Spec.Taints, t)
			taintsUpdated = true
		}
	}
	if labelsUpdated || annotationsUpdated || taintsUpdated {
		node, err := c.kubeClient.CoreV1().Nodes().Update(node)
		if err != nil {
			return fmt.Errorf("failed to update node %s after setting labels/annotations/taints: %v", node.Name, err)
		}
		c.recorder.Event(machine, corev1.EventTypeNormal, "LabelsAnnotationsTaintsUpdated", "Successfully updated labels/annotations/taints")
		glog.V(4).Infof("Added labels/annotations/taints to node %s (machine %s)", node.Name, machine.Name)
	}

	return nil

}

func (c *Controller) updateMachineStatus(machine *clusterv1alpha1.Machine, node *corev1.Node) error {
	if node == nil {
		return nil
	}

	ref, err := reference.GetReference(scheme.Scheme, node)
	if err != nil {
		return fmt.Errorf("failed to get node reference for %s : %v", node.Name, err)
	}
	if !equality.Semantic.DeepEqual(machine.Status.NodeRef, ref) ||
		machine.Status.Versions == nil ||
		machine.Status.Versions.Kubelet != node.Status.NodeInfo.KubeletVersion {
		if machine, err = c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			m.Status.NodeRef = ref
			m.Status.Versions = &clusterv1alpha1.MachineVersionInfo{Kubelet: node.Status.NodeInfo.KubeletVersion}
		}); err != nil {
			return fmt.Errorf("failed to update machine after setting its status: %v", err)
		}
	}

	return nil
}

func (c *Controller) getNode(instance instance.Instance, provider providerconfig.CloudProvider) (node *corev1.Node, exists bool, err error) {
	if instance == nil {
		return nil, false, fmt.Errorf("getNode called with nil provider instance")
	}
	nodes, err := c.nodesLister.List(labels.Everything())
	if err != nil {
		return nil, false, err
	}

	// We trim leading slashes in raw ID, since we always want three slashes in full ID
	providerID := fmt.Sprintf("%s:///%s", provider, strings.TrimLeft(instance.ID(), "/"))
	for _, node := range nodes {
		if provider == providerconfig.CloudProviderAzure {
			// Azure IDs are case-insensitive
			if strings.EqualFold(node.Spec.ProviderID, providerID) {
				return node.DeepCopy(), true, nil
			}
		} else {
			if node.Spec.ProviderID == providerID {
				return node.DeepCopy(), true, nil
			}
		}
		for _, nodeAddress := range node.Status.Addresses {
			for _, instanceAddress := range instance.Addresses() {
				if nodeAddress.Address == instanceAddress {
					return node.DeepCopy(), true, nil
				}
			}
		}
	}
	return nil, false, nil
}

func (c *Controller) defaultMachine(machine *clusterv1alpha1.Machine, prov userdata.Provider) (*clusterv1alpha1.Machine, error) {
	var err error

	if machine.Spec.Versions.Kubelet == "" {
		if machine, err = c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			m.Spec.Versions.Kubelet = latestKubernetesVersion
		}); err != nil {
			return nil, err
		}
	}

	return machine, nil
}

func (c *Controller) enqueueMachine(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

func (c *Controller) enqueueMachineAfter(obj interface{}, after time.Duration) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddAfter(key, after)
}

func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		glog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}

	machinesList, err := c.machinesLister.List(labels.Everything())
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Failed to list machines in lister: %v", err))
		return
	}

	var ownerUIDString string
	var exists bool
	if labels := object.GetLabels(); labels != nil {
		ownerUIDString, exists = labels[NodeOwnerLabelName]
	}
	if !exists {
		// We get triggered by node{Add,Update}, so enqeue machines if they
		// have no nodeRef yet to make matching happen ASAP
		for _, machine := range machinesList {
			if machine.Status.NodeRef == nil {
				c.enqueueMachine(machine)
			}
		}
	}

	for _, machine := range machinesList {
		if string(machine.UID) == ownerUIDString {
			glog.V(6).Infof("Processing node: %s (machine=%s)", object.GetName(), machine.Name)
			c.enqueueMachine(machine)
			break
		}
	}

}

func (c *Controller) ReadinessChecks() map[string]healthcheck.Check {
	return map[string]healthcheck.Check{
		"valid-info-kubeconfig": func() error {
			cm, err := c.kubeconfigProvider.GetKubeconfig()
			if err != nil {
				err := fmt.Errorf("failed to get cluster-info configmap: %v", err)
				glog.V(2).Info(err)
				return err
			}
			if len(cm.Clusters) != 1 {
				err := errors.New("invalid kubeconfig: no clusters found")
				glog.V(2).Info(err)
				return err
			}
			for name, c := range cm.Clusters {
				if len(c.CertificateAuthorityData) == 0 {
					err := fmt.Errorf("invalid kubeconfig: no certificate authority data was specified for kuberconfig.clusters.['%s']", name)
					glog.V(2).Info(err)
					return err
				}
				if len(c.Server) == 0 {
					err := fmt.Errorf("invalid kubeconfig: no server was specified for kuberconfig.clusters.['%s']", name)
					glog.V(2).Info(err)
					return err
				}
			}
			return nil
		},
	}
}

func (c *Controller) ensureDeleteFinalizerExists(machine *clusterv1alpha1.Machine) (*clusterv1alpha1.Machine, error) {
	if !sets.NewString(machine.Finalizers...).Has(FinalizerDeleteInstance) {
		var err error
		if machine, err = c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			finalizers := sets.NewString(m.Finalizers...)
			finalizers.Insert(FinalizerDeleteInstance)
			finalizers.Insert(FinalizerDeleteNode)
			m.Finalizers = finalizers.List()
		}); err != nil {
			return nil, fmt.Errorf("failed to update machine after adding the delete instance finalizer: %v", err)
		}
		glog.V(4).Infof("Added delete finalizer to machine %s", machine.Name)
	}
	return machine, nil
}

func (c *Controller) updateNode(name string, modify func(*corev1.Node)) (*corev1.Node, error) {
	var updatedNode *corev1.Node
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var retryErr error

		//Get latest version from API
		currentNode, err := c.kubeClient.CoreV1().Nodes().Get(name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// Apply modifications
		modify(currentNode)

		// Update the node
		updatedNode, retryErr = c.kubeClient.CoreV1().Nodes().Update(currentNode)
		return retryErr
	})

	return updatedNode, err
}
