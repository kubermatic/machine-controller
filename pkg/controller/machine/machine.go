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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/heptiolabs/healthcheck"
	machineclientset "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned"
	machinescheme "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned/scheme"
	machinelistersv1alpha1 "github.com/kubermatic/machine-controller/pkg/client/listers/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/cloud"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/containerruntime"
	"github.com/kubermatic/machine-controller/pkg/containerruntime/docker"
	machinev1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
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
)

const (
	finalizerDeleteInstance = "machine-delete-finalizer"

	metricsUpdatePeriod     = 10 * time.Second
	deletionRetryWaitPeriod = 10 * time.Second

	machineKind = "Machine"

	controllerNameAnnotationKey = "machine.k8s.io/controller"

	latestKubernetesVersion = "1.9.6"
)

// Controller is the controller implementation for machine resources
type Controller struct {
	kubeClient    kubernetes.Interface
	machineClient machineclientset.Interface

	nodesLister          listerscorev1.NodeLister
	machinesLister       machinelistersv1alpha1.MachineLister
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
	machineClient machineclientset.Interface,
	nodeInformer cache.SharedIndexInformer,
	nodeLister listerscorev1.NodeLister,
	machineInformer cache.SharedIndexInformer,
	machineLister machinelistersv1alpha1.MachineLister,
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

func (c *Controller) processNextWorkItem() bool {
	key, quit := c.workqueue.Get()
	if quit {
		return false
	}

	defer c.workqueue.Done(key)

	glog.V(6).Infof("Processing machine: %s", key)
	err := c.syncHandler(key.(string))
	if err == nil {
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

func (c *Controller) updateMachine(name string, modify func(*machinev1alpha1.Machine)) (*machinev1alpha1.Machine, error) {
	var updatedMachine *machinev1alpha1.Machine
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var retryErr error

		//Get latest version from API
		currentMachine, err := c.machineClient.Machine().Machines().Get(name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// Apply modifications
		modify(currentMachine)

		// Update the machine
		updatedMachine, retryErr = c.machineClient.MachineV1alpha1().Machines().Update(currentMachine)
		return retryErr
	})

	return updatedMachine, err
}

func (c *Controller) clearMachineErrorIfSet(machine *machinev1alpha1.Machine) (*machinev1alpha1.Machine, error) {
	if machine.Status.ErrorMessage != nil || machine.Status.ErrorReason != nil {
		return c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
			m.Status.ErrorMessage = nil
			m.Status.ErrorReason = nil
		})
	}
	return machine, nil
}

// updateMachine updates machine's ErrorMessage and ErrorReason regardless if they were set or not
// this essentially overwrites previous values
func (c *Controller) updateMachineError(machine *machinev1alpha1.Machine, reason machinev1alpha1.MachineStatusError, message string) (*machinev1alpha1.Machine, error) {
	return c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
		m.Status.ErrorMessage = &message
		m.Status.ErrorReason = &reason
	})
}

// updateMachineErrorIfTerminalError is a convenience method that will update machine's Status if the given err is terminal
// and at the same time terminal error will be returned to the caller
// otherwise it will return formatted error according to errMsg
func (c *Controller) updateMachineErrorIfTerminalError(machine *machinev1alpha1.Machine, stReason machinev1alpha1.MachineStatusError, stMessage string, err error, errMsg string) error {
	c.recorder.Eventf(machine, corev1.EventTypeWarning, string(stReason), stMessage)
	if ok, _, _ := cloudprovidererrors.IsTerminalError(err); ok {
		if _, errNested := c.updateMachineError(machine, stReason, stMessage); errNested != nil {
			return fmt.Errorf("failed to update machine error after due to %v, terminal error = %v", errNested, stMessage)
		}
		return err
	}
	return fmt.Errorf("%s, due to %v", errMsg, err)
}

func (c *Controller) getProviderInstance(prov cloud.Provider, machine *machinev1alpha1.Machine) (instance.Instance, error) {
	return prov.Get(machine)
}

func (c *Controller) deleteProviderInstance(prov cloud.Provider, machine *machinev1alpha1.Machine) error {
	return prov.Delete(machine, c.updateMachine)
}

func (c *Controller) createProviderInstance(prov cloud.Provider, machine *machinev1alpha1.Machine, userdata string) (instance.Instance, error) {
	// Ensure finalizer is there
	machine, err := c.ensureDeleteFinalizerExists(machine)
	if err != nil {
		return nil, err
	}
	return prov.Create(machine, c.updateMachine, userdata)
}

func (c *Controller) validateMachine(prov cloud.Provider, machine *machinev1alpha1.Machine) error {
	err := prov.Validate(machine.Spec)
	if err != nil {
		c.recorder.Eventf(machine, corev1.EventTypeWarning, "ValidationFailed", "Validation failed: %v", err)
		return err
	}
	c.recorder.Event(machine, corev1.EventTypeNormal, "ValidationSucceeded", "Validation succeeded")
	return nil
}

func (c *Controller) syncHandler(key string) error {
	listerMachine, err := c.machinesLister.Get(key)
	if err != nil {
		if kerrors.IsNotFound(err) {
			glog.V(2).Infof("machine '%s' in work queue no longer exists", key)
			return nil
		}
		return err
	}
	machine := listerMachine.DeepCopy()

	// step 1: check if the machine can be processed by this controller.
	// set the annotation "machine.k8s.io/controller": my-controller
	// and the flag --name=my-controller to make only this controller process a node
	machineControllerName := machine.Annotations[controllerNameAnnotationKey]
	if machineControllerName != c.name {
		glog.V(6).Infof("skipping machine '%s' as it is not meant for this controller", machine.Name)
		if machineControllerName == "" && c.name != "" {
			glog.V(6).Infof("this controller is configured to only process machines with the annotation %s:%s", controllerNameAnnotationKey, c.name)
			return nil
		}
		if machineControllerName != "" && c.name == "" {
			glog.V(6).Infof("this controller is configured to process all machines which have no controller specified via annotation %s. The machine has %s:%s", controllerNameAnnotationKey, controllerNameAnnotationKey, machineControllerName)
			return nil
		}

		glog.V(6).Infof("this controller is configured to process machines which the annotation %s:%s. The machine has %s:%s", controllerNameAnnotationKey, c.name, controllerNameAnnotationKey, machineControllerName)
		return nil
	}

	if machine.Spec.Name == "" {
		machine, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
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
		if err := c.deleteMachineAndProviderInstance(prov, machine); err != nil {
			return err
		}
		// As the deletion got triggered but the instance might not been gone yet, we need to recheck in a few seconds.
		c.workqueue.AddAfter(machine.Name, deletionRetryWaitPeriod)
		return nil
	}

	// step 3: essentially creates an instance for the given machine
	userdataProvider, err := userdata.ForOS(providerConfig.OperatingSystem)
	if err != nil {
		return fmt.Errorf("failed to userdata provider for '%s': %v", providerConfig.OperatingSystem, err)
	}

	// We use a new variable here to be able to put the Event on the machine even thought
	// c.defaultContainerRuntime returns a nil pointer for the machine in case of an error
	machineWithDefaultedContainerRuntime, err := c.defaultContainerRuntime(machine, userdataProvider)
	if err != nil {
		errorMessage := fmt.Sprintf("failed to default the container runtime version: %v", err)
		c.recorder.Event(machine, corev1.EventTypeWarning, "ContainerRuntimeDefaultingFailed", errorMessage)
		return errors.New(errorMessage)
	}
	machine = machineWithDefaultedContainerRuntime

	// case 3.2: creates an instance if there is no node associated with the given machine
	if machine.Status.NodeRef == nil {
		return c.ensureInstanceExistsForMachine(prov, machine, userdataProvider, providerConfig)
	}

	node, err := c.getNodeByNodeRef(machine.Status.NodeRef)
	if err != nil {
		//In case we cannot find a node for the NodeRef we must remove the NodeRef & recreate an instance on the next sync
		if kerrors.IsNotFound(err) {
			glog.V(4).Infof("found invalid NodeRef on machine %s. Deleting reference...", machine.Name)
			_, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
				m.Status.NodeRef = nil
			})
			return err
		}
		return fmt.Errorf("failed to check if node for machine exists: '%s'", err)
	}

	if c.nodeIsReady(node) {
		// If we have an ready node, we should clear the error in case one was set.
		// Useful when there was a network outage & a cloud-provider api outage at the same time
		if machine, err = c.clearMachineErrorIfSet(machine); err != nil {
			return fmt.Errorf("failed to clear machine error: %v", err)
		}
	} else {
		// Node is not ready anymore? Maybe it got deleted
		return c.ensureInstanceExistsForMachine(prov, machine, userdataProvider, providerConfig)
	}

	// case 3.3: if the node exists make sure if it has labels and taints attached to it.
	return c.ensureNodeLabelsAnnotationsAndTaints(node, machine)
}

func (c *Controller) cleanupMachineAfterDeletion(machine *machinev1alpha1.Machine) error {
	var err error
	glog.V(4).Infof("Removing finalizers from machine machine %s", machine.Name)

	if machine, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
		finalizers := sets.NewString(m.Finalizers...)
		finalizers.Delete(finalizerDeleteInstance)
		m.Finalizers = finalizers.List()
	}); err != nil {
		return fmt.Errorf("failed to update machine after removing the delete instance finalizer: %v", err)
	}

	glog.V(4).Infof("Removed delete finalizer from machine %s", machine.Name)
	return nil
}

// deleteMachineAndProviderInstance makes sure that an instance has gone in a series of steps.
func (c *Controller) deleteMachineAndProviderInstance(prov cloud.Provider, machine *machinev1alpha1.Machine) error {
	if err := c.deleteProviderInstance(prov, machine); err != nil {
		message := fmt.Sprintf("%v. Please manually delete finalizers from the machine object.", err)
		c.recorder.Eventf(machine, corev1.EventTypeWarning, "DeletionFailed", "Failed to delete machine: %v", err)
		return c.updateMachineErrorIfTerminalError(machine, machinev1alpha1.DeleteMachineError, message, err, "failed to delete machine at cloudprovider")
	}
	return c.cleanupMachineAfterDeletion(machine)
}

func (c *Controller) ensureInstanceExistsForMachine(prov cloud.Provider, machine *machinev1alpha1.Machine, userdataProvider userdata.Provider, providerConfig *providerconfig.Config) error {
	glog.V(6).Infof("Requesting instance for machine '%s' from cloudprovider because no associated node with status ready found...", machine.Name)
	// case 1: validate the machine spec before getting the instance from cloud provider.
	// even though this is a little bit premature and inefficient, it helps us detect invalid specification
	defaultedMachineSpec, changed, err := prov.AddDefaults(machine.Spec)
	if err != nil {
		return c.updateMachineErrorIfTerminalError(machine, machinev1alpha1.InvalidConfigurationMachineError, err.Error(), err, "failed to add defaults to machine")
	}
	if changed {
		glog.V(4).Infof("updating machine '%s' with defaults...", machine.Name)
		c.recorder.Event(machine, corev1.EventTypeNormal, "Defaulted", "Updated machine with defaults")
		if machine, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
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
			if _, errNested := c.updateMachineError(machine, machinev1alpha1.InvalidConfigurationMachineError, err.Error()); errNested != nil {
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
			// remove an error message in case it was set
			if machine, err = c.clearMachineErrorIfSet(machine); err != nil {
				return fmt.Errorf("failed to update machine after removing the failed validation error: %v", err)
			}
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
				return c.updateMachineErrorIfTerminalError(machine, machinev1alpha1.CreateMachineError, message, err, "failed to create machine at cloudprover")
			}
			c.recorder.Event(machine, corev1.EventTypeNormal, "Created", "Successfully created instance")
			// remove error message in case it was set
			if machine, err = c.clearMachineErrorIfSet(machine); err != nil {
				return fmt.Errorf("failed to update machine after removing the create machine error: %v", err)
			}
			glog.V(4).Infof("Created machine %s at cloud provider", machine.Name)
			return nil
		}

		// case 2.2: terminal error was returned and manual interaction is required to recover
		if ok, _, message := cloudprovidererrors.IsTerminalError(err); ok {
			message = fmt.Sprintf("%v. Unable to create a machine.", err)
			return c.updateMachineErrorIfTerminalError(machine, machinev1alpha1.CreateMachineError, message, err, "failed to get instance from provider")
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
	eventMessage := fmt.Sprintf("Found instance at cloud provider, addresses: %v", providerInstance.Addresses())
	c.recorder.Event(machine, corev1.EventTypeNormal, "InstanceFound", eventMessage)
	return c.ensureNodeOwnerRefAndConfigSource(providerInstance, machine, providerConfig)
}

func (c *Controller) ensureNodeOwnerRefAndConfigSource(providerInstance instance.Instance, machine *machinev1alpha1.Machine, providerConfig *providerconfig.Config) error {
	node, exists, err := c.getNode(providerInstance, providerConfig.CloudProvider)
	if err != nil {
		return fmt.Errorf("failed to get node for machine %s: %v", machine.Name, err)
	}
	if exists {
		ownerRef := metav1.GetControllerOf(node)
		if ownerRef == nil {
			gv := machinev1alpha1.SchemeGroupVersion
			node.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(machine, gv.WithKind(machineKind))}
			node, err = c.kubeClient.CoreV1().Nodes().Update(node)
			if err != nil {
				return fmt.Errorf("failed to update node %s after adding the owner ref: %v", node.Name, err)
			}
			glog.V(4).Infof("Added owner ref to node %s (machine=%s)", node.Name, machine.Name)
			c.recorder.Eventf(machine, corev1.EventTypeNormal, "NodeMatched", "Successfully matched machine to node %s", node.Name)
		}

		if node.Spec.ConfigSource == nil && machine.Spec.ConfigSource != nil {
			node.Spec.ConfigSource = machine.Spec.ConfigSource
			node, err = c.kubeClient.CoreV1().Nodes().Update(node)
			if err != nil {
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

func (c *Controller) ensureNodeLabelsAnnotationsAndTaints(node *corev1.Node, machine *machinev1alpha1.Machine) error {
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

func (c *Controller) updateMachineStatus(machine *machinev1alpha1.Machine, node *corev1.Node) error {
	if node == nil {
		return nil
	}

	var (
		runtimeName, runtimeVersion string
		err                         error
	)

	ref, err := reference.GetReference(scheme.Scheme, node)
	if err != nil {
		return fmt.Errorf("failed to get node reference for %s : %v", node.Name, err)
	}

	if !equality.Semantic.DeepEqual(machine.Status.NodeRef, ref) {
		if machine, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
			m.Status.NodeRef = ref
		}); err != nil {
			return fmt.Errorf("failed to update machine: %v", err)
		}
	}

	if machine.Status.Versions == nil {
		if machine, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
			m.Status.Versions = &machinev1alpha1.MachineVersionInfo{}
		}); err != nil {
			return fmt.Errorf("failed to update machine: %v", err)
		}
	}

	if node.Status.NodeInfo.ContainerRuntimeVersion != "" {
		runtimeName, runtimeVersion, err = parseContainerRuntime(node.Status.NodeInfo.ContainerRuntimeVersion)
		if err != nil {
			glog.V(2).Infof("failed to parse container runtime from node %s: %v", node.Name, err)
			runtimeName = "unknown"
			runtimeVersion = "unknown"
		}
		if machine.Status.Versions.ContainerRuntime.Name != runtimeName || machine.Status.Versions.ContainerRuntime.Version != runtimeVersion {
			if machine, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
				m.Status.Versions.ContainerRuntime.Name = runtimeName
				m.Status.Versions.ContainerRuntime.Version = runtimeVersion
			}); err != nil {
				return fmt.Errorf("failed to update machine: %v", err)
			}
		}
	}

	if machine.Status.Versions.Kubelet != node.Status.NodeInfo.KubeletVersion {
		if machine, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
			m.Status.Versions.Kubelet = node.Status.NodeInfo.KubeletVersion
		}); err != nil {
			return fmt.Errorf("failed to update machine: %v", err)
		}
	}

	return nil
}

var (
	containerRuntime = regexp.MustCompile(`(docker)://(.*)`)
)

func parseContainerRuntime(s string) (runtime, version string, err error) {
	res := containerRuntime.FindStringSubmatch(s)
	if len(res) == 3 {
		return res[1], res[2], nil
	}
	return "", "", fmt.Errorf("invalid format. Expected 'runtime://version'")
}

func (c *Controller) getNode(instance instance.Instance, provider providerconfig.CloudProvider) (node *corev1.Node, exists bool, err error) {
	if instance == nil {
		return nil, false, fmt.Errorf("getNode called with nil provider instance!")
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

func (c *Controller) defaultContainerRuntime(machine *machinev1alpha1.Machine, prov userdata.Provider) (*machinev1alpha1.Machine, error) {
	var err error

	if machine.Spec.Versions.Kubelet == "" {
		if machine, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
			m.Spec.Versions.Kubelet = latestKubernetesVersion
		}); err != nil {
			return nil, err
		}
	}

	if machine.Spec.Versions.ContainerRuntime.Name == "" {
		if machine, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
			m.Spec.Versions.ContainerRuntime.Name = containerruntime.Docker
		}); err != nil {
			return nil, err
		}
	}

	if machine.Spec.Versions.ContainerRuntime.Version == "" {
		var (
			defaultVersions []string
			err             error
		)
		switch machine.Spec.Versions.ContainerRuntime.Name {
		case containerruntime.Docker:
			defaultVersions, err = docker.GetOfficiallySupportedVersions(machine.Spec.Versions.Kubelet)
			if err != nil {
				return nil, fmt.Errorf("failed to get a officially supported docker version for the given kubelet version: %v", err)
			}
		default:
			return nil, fmt.Errorf("invalid container runtime. Supported: '%s'", containerruntime.Docker)
		}

		var newVersion string
		providerSupportedVersions := prov.SupportedContainerRuntimes()
		for _, v := range defaultVersions {
			for _, sv := range providerSupportedVersions {
				if sv.Version == v {
					// we should not return asap as we prefer the highest supported version
					newVersion = sv.Version
				}
			}
		}
		if newVersion == "" {
			return nil, fmt.Errorf("no supported versions available for '%s'", machine.Spec.Versions.ContainerRuntime.Name)
		}
		machine, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
			m.Spec.Versions.ContainerRuntime.Version = newVersion
		})
		if err != nil {
			return nil, err
		}
		return machine, err
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

	ownerRef := metav1.GetControllerOf(object)
	if ownerRef != nil {
		if ownerRef.Kind != "Machine" {
			return
		}
		machine, err := c.machinesLister.Get(ownerRef.Name)
		if err != nil {
			glog.V(4).Infof("ignoring orphaned object '%s' of machine '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		glog.V(6).Infof("Processing node: %s (machine=%s)", object.GetName(), machine.Name)
		c.enqueueMachine(machine)
		return
	}

	if ownerRef == nil {
		machines, err := c.machinesLister.List(labels.Everything())
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("error listing machines: '%v'", err))
			return
		}
		for _, machine := range machines {
			// We get triggered by node{Add,Update}, so enqeue machines if they
			// have no nodeRef yet to make matching happen ASAP
			if machine.Status.NodeRef == nil {
				c.enqueueMachine(machine)
			}
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

func (c *Controller) ensureDeleteFinalizerExists(machine *machinev1alpha1.Machine) (*machinev1alpha1.Machine, error) {
	if !sets.NewString(machine.Finalizers...).Has(finalizerDeleteInstance) {
		var err error
		if machine, err = c.updateMachine(machine.Name, func(m *machinev1alpha1.Machine) {
			finalizers := sets.NewString(m.Finalizers...)
			finalizers.Insert(finalizerDeleteInstance)
			m.Finalizers = finalizers.List()
		}); err != nil {
			return nil, fmt.Errorf("failed to update machine after adding the delete instance finalizer: %v", err)
		}
		glog.V(4).Infof("Added delete finalizer to machine %s", machine.Name)
	}
	return machine, nil
}
