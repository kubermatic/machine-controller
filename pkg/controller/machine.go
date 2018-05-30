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
	"time"

	"github.com/go-kit/kit/metrics"
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
	"github.com/kubermatic/machine-controller/pkg/containerruntime/crio"
	"github.com/kubermatic/machine-controller/pkg/containerruntime/docker"
	machinev1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata"

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
	"k8s.io/client-go/util/workqueue"
)

const (
	finalizerDeleteInstance = "machine-delete-finalizer"

	metricsUpdatePeriod     = 10 * time.Second
	deletionRetryWaitPeriod = 10 * time.Second
	initialDeleteWaitPeriod = 5 * time.Second

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
	metrics            MetricsCollection
	kubeconfigProvider KubeconfigProvider

	validationCache map[string]bool

	name string
}

type KubeconfigProvider interface {
	GetKubeconfig() (*clientcmdapi.Config, error)
}

// MetricsCollection is a struct of all metrics used in
// this controller.
type MetricsCollection struct {
	Machines            metrics.Gauge
	Nodes               metrics.Gauge
	Workers             metrics.Gauge
	Errors              metrics.Counter
	ControllerOperation metrics.Histogram
	NodeJoinDuration    metrics.Histogram
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
	metrics MetricsCollection,
	kubeconfigProvider KubeconfigProvider,
	name string) *Controller {

	machinescheme.AddToScheme(scheme.Scheme)
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.V(4).Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})

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
	go wait.Until(c.updateMetrics, metricsUpdatePeriod, stopCh)

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

func (c *Controller) doesNodeForMachineExistAndIsReady(machine *machinev1alpha1.Machine) (*corev1.Node, bool, error) {
	if machine.Status.NodeRef != nil {
		listerNode, err := c.nodesLister.Get(machine.Status.NodeRef.Name)
		if err != nil {
			return nil, false, err
		}
		node := listerNode.DeepCopy()
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady {
				if condition.Status == corev1.ConditionTrue {
					return node, true, nil
				}
			}
		}
	}

	return nil, false, nil
}

func (c *Controller) updateMachine(machine *machinev1alpha1.Machine) (*machinev1alpha1.Machine, error) {
	machine.Status.LastUpdated = metav1.Now()
	return c.machineClient.MachineV1alpha1().Machines().Update(machine)
}

func (c *Controller) clearMachineErrorIfSet(machine *machinev1alpha1.Machine) (*machinev1alpha1.Machine, error) {
	if machine.Status.ErrorMessage != nil || machine.Status.ErrorReason != nil {
		machine.Status.ErrorMessage = nil
		machine.Status.ErrorReason = nil
		return c.updateMachine(machine)
	}
	return machine, nil
}

// updateMachine updates machine's ErrorMessage and ErrorReason regardless if they were set or not
// this essentially overwrites previous values
func (c *Controller) updateMachineError(machine *machinev1alpha1.Machine, reason machinev1alpha1.MachineStatusError, message string) (*machinev1alpha1.Machine, error) {
	machine.Status.ErrorMessage = &message
	machine.Status.ErrorReason = &reason
	return c.updateMachine(machine)
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
	start := time.Now()
	defer c.metrics.ControllerOperation.With("operation", "get-cloud-instance").Observe(time.Since(start).Seconds())
	return prov.Get(machine)
}

func (c *Controller) deleteProviderInstance(prov cloud.Provider, machine *machinev1alpha1.Machine, instance instance.Instance) error {
	start := time.Now()
	defer c.metrics.ControllerOperation.With("operation", "delete-cloud-instance").Observe(time.Since(start).Seconds())
	return prov.Delete(machine, instance)
}

func (c *Controller) createProviderInstance(prov cloud.Provider, machine *machinev1alpha1.Machine, userdata string) (instance.Instance, error) {
	start := time.Now()
	defer c.metrics.ControllerOperation.With("operation", "create-cloud-instance").Observe(time.Since(start).Seconds())
	return prov.Create(machine, userdata)
}

func (c *Controller) validateMachine(prov cloud.Provider, machine *machinev1alpha1.Machine) error {
	start := time.Now()
	defer c.metrics.ControllerOperation.With("operation", "validate-machine").Observe(time.Since(start).Seconds())
	return prov.Validate(machine.Spec)
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
	if machine.DeletionTimestamp != nil && sets.NewString(machine.Finalizers...).Has(finalizerDeleteInstance) {
		return c.deleteMachineAndProviderInstance(prov, machine)
	}

	// step 3: essentially creates an instance for the given machine
	//
	// case 3.1: first let's create the delete finalizer before actually creating the instance.
	// otherwise the machine gets created at the cloud provider and the machine resource gets deleted meanwhile
	// which causes a orphaned instance
	if !sets.NewString(machine.Finalizers...).Has(finalizerDeleteInstance) {
		finalizers := sets.NewString(machine.Finalizers...)
		finalizers.Insert(finalizerDeleteInstance)
		machine.Finalizers = finalizers.List()
		if machine, err = c.updateMachine(machine); err != nil {
			return fmt.Errorf("failed to update machine after adding the delete instance finalizer: %v", err)
		}
		glog.V(4).Infof("Added delete finalizer to machine %s", machine.Name)
	}

	userdataProvider, err := userdata.ForOS(providerConfig.OperatingSystem)
	if err != nil {
		return fmt.Errorf("failed to userdata provider for '%s': %v", providerConfig.OperatingSystem, err)
	}
	if machine, err = c.defaultContainerRuntime(machine, userdataProvider); err != nil {
		return fmt.Errorf("failed to default the container runtime version: %v", err)
	}

	// case 3.2: creates an instance if there is no node associated with the given machine
	node, nodeExistsAndIsReady, err := c.doesNodeForMachineExistAndIsReady(machine)
	if err != nil {
		return fmt.Errorf("failed to check if node for machine exists ans is ready: '%s'", err)
	}
	if !nodeExistsAndIsReady {
		glog.V(6).Infof("Requesting instance for machine '%s' from cloudprovider because no associated node with status ready found...", machine.Name)
		err = c.ensureInstanceExistsForMachine(prov, machine, userdataProvider, providerConfig)
		if err != nil {
			return err
		}
	}

	if nodeExistsAndIsReady {
		// If we have an ready node, we should clear the error in case one was set.
		// Useful when there was a network outage & a cloud-provider api outage at the same time
		if machine, err = c.clearMachineErrorIfSet(machine); err != nil {
			return fmt.Errorf("failed to clear machine error: %v", err)
		}
	}

	// case 3.3: if the node exists make sure if it has labels and taints attached to it.
	if node != nil {
		return c.ensureNodeLabelsAnnotationsAndTaints(node, machine)
	}

	return nil
}

func (c *Controller) cleanupMachineAfterDeletion(machine *machinev1alpha1.Machine) error {
	var err error
	glog.V(4).Infof("Removing finalizers from machine machine %s", machine.Name)

	finalizers := sets.NewString(machine.Finalizers...)
	finalizers.Delete(finalizerDeleteInstance)
	machine.Finalizers = finalizers.List()
	if machine, err = c.updateMachine(machine); err != nil {
		return fmt.Errorf("failed to update machine after removing the delete instance finalizer: %v", err)
	}

	glog.V(4).Infof("Removed delete finalizer from machine %s", machine.Name)
	return nil
}

// deleteMachineAndProviderInstance makes sure that an instance has gone in a series of steps.
func (c *Controller) deleteMachineAndProviderInstance(prov cloud.Provider, machine *machinev1alpha1.Machine) error {

	// step 1: get the provider instance.
	providerInstance, err := c.getProviderInstance(prov, machine)
	if err != nil {
		// step 1.1: failed to get instance, because of some unknown error -> return and see if we need to handle a terminal error here
		if err != cloudprovidererrors.ErrInstanceNotFound {
			return c.updateMachineErrorIfTerminalError(machine, machinev1alpha1.DeleteMachineError, err.Error(), err, fmt.Sprintf("failed to get instance for machine %s after the delete got triggered", machine.Name))
		}
		glog.V(4).Infof("Provider has no instance for %s. Considering it as deleted", machine.Name)

		// step 1.2 the instance could not be found -> it's gone, so we remove the finalizer. This essentially will remove the machine object from the system
		return c.cleanupMachineAfterDeletion(machine)
	}

	// step 2: we still have an instance on the cloud provider
	// step 2.1: Check if its in deleting state - if so, the provider normally does some own cleanup. We wait until the instance is completely gone.
	if instance.StatusDeleting == providerInstance.Status() {
		glog.V(4).Infof("deletion of instance %s got triggered. Waiting until it fully disappears", providerInstance.ID())
		c.workqueue.AddAfter(machine.Name, deletionRetryWaitPeriod)
		return nil
	}

	// step 2.2: The instance exists at the provider, but its considered dead
	if instance.StatusDeleted == providerInstance.Status() {
		glog.V(4).Infof("Provider says the instance for %s is deleted", machine.Name)
		return c.cleanupMachineAfterDeletion(machine)
	}

	// step 2.3: delete provider instance
	if err = c.deleteProviderInstance(prov, machine, providerInstance); err != nil {
		message := fmt.Sprintf("%v. Please manually delete finalizers from the machine object.", err)
		c.recorder.Eventf(machine, corev1.EventTypeWarning, "DeletionFailed", "Failed to delete machine: %v", err)
		return c.updateMachineErrorIfTerminalError(machine, machinev1alpha1.DeleteMachineError, message, err, "failed to delete machine at cloudprovider")
	}

	// step 3: remove error message in case it was set
	if machine, err = c.clearMachineErrorIfSet(machine); err != nil {
		return fmt.Errorf("failed to update machine after removing the delete error: %v", err)
	}

	// step 4: put machine back into the queue as we just triggered the deletion
	c.workqueue.AddAfter(machine.Name, initialDeleteWaitPeriod)
	return nil
}

func (c *Controller) ensureInstanceExistsForMachine(prov cloud.Provider, machine *machinev1alpha1.Machine, userdataProvider userdata.Provider, providerConfig *providerconfig.Config) error {
	// case 1: validate the machine spec before getting the instance from cloud provider.
	// even though this is a little bit premature and inefficient, it helps us detect invalid specification
	defaultedMachineSpec, changed, err := prov.AddDefaults(machine.Spec)
	if err != nil {
		return c.updateMachineErrorIfTerminalError(machine, machinev1alpha1.InvalidConfigurationMachineError, err.Error(), err, "failed to add defaults to machine")
	}
	if changed {
		glog.V(4).Infof("updating machine '%s' with defaults...", machine.Name)
		c.recorder.Event(machine, corev1.EventTypeNormal, "Defaulted", "Updated machine with defaults")
		machine.Spec = defaultedMachineSpec
		if machine, err = c.updateMachine(machine); err != nil {
			return fmt.Errorf("failed to update machine '%s' after adding defaults: '%v'", machine.Name, err)
		}
		glog.V(4).Infof("Successfully updated machine '%s' with defaults!", machine.Name)
	}

	cacheKey := string(machine.UID) + machine.ResourceVersion
	if !c.validationCache[cacheKey] {
		if err := c.validateMachine(prov, machine); err != nil {
			if _, errNested := c.updateMachineError(machine, machinev1alpha1.InvalidConfigurationMachineError, err.Error()); errNested != nil {
				return fmt.Errorf("failed to update machine error after failed validation: %v", errNested)
			}
			c.recorder.Eventf(machine, corev1.EventTypeWarning, "ValidationFailed", "Validation failed: %v", err)
			return fmt.Errorf("invalid provider config: %v", err)
		}
		c.recorder.Event(machine, corev1.EventTypeNormal, "ValidationSucceeded", "Validation succeeded")
		c.validationCache[cacheKey] = true
	} else {
		glog.V(6).Infof("Skipping validation as the machine was already successfully validated before")
	}
	providerInstance, err := prov.Get(machine)

	// case 2: retrieving instance from provider was not successful
	if err != nil {
		//First invalidate the validation cache to make sure we run the validation on the next sync.
		//This might happen in case the user invalidates his provider credentials...
		c.validationCache[cacheKey] = false

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

	// case 3: retrieving the instance from cloudprovider was successfull
	c.recorder.Event(machine, corev1.EventTypeNormal, "InstanceFound", "Found instance at cloud provider")
	return c.ensureNodeOwnerRefAndConfigSource(providerInstance, machine, providerConfig)
}

func (c *Controller) ensureNodeOwnerRefAndConfigSource(providerInstance instance.Instance, machine *machinev1alpha1.Machine, providerConfig *providerconfig.Config) error {
	node, exists, err := c.getNode(providerInstance, string(providerConfig.CloudProvider))
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
			c.metrics.NodeJoinDuration.Observe(node.CreationTimestamp.Sub(machine.CreationTimestamp.Time).Seconds())
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
		c.recorder.Event(machine, corev1.EventTypeNormal, "LabelsAnnotationsTainsUpdated", "Sucecssfully updated labels/annotations/taints")
		glog.V(4).Infof("Added labels/annotations/taints to node %s (machine %s)", node.Name, machine.Name)
	}

	return nil

}

func (c *Controller) updateMachineStatus(machine *machinev1alpha1.Machine, node *corev1.Node) error {
	if node == nil {
		return nil
	}

	var (
		updated                     bool
		runtimeName, runtimeVersion string
		err                         error
	)

	ref, err := reference.GetReference(scheme.Scheme, node)
	if err != nil {
		return fmt.Errorf("failed to get node reference for %s : %v", node.Name, err)
	}
	if !equality.Semantic.DeepEqual(machine.Status.NodeRef, ref) {
		machine.Status.NodeRef = ref
		updated = true
	}

	if machine.Status.Versions == nil {
		machine.Status.Versions = &machinev1alpha1.MachineVersionInfo{}
	}

	if node.Status.NodeInfo.ContainerRuntimeVersion != "" {
		runtimeName, runtimeVersion, err = parseContainerRuntime(node.Status.NodeInfo.ContainerRuntimeVersion)
		if err != nil {
			glog.V(2).Infof("failed to parse container runtime from node %s: %v", node.Name, err)
			runtimeName = "unknown"
			runtimeVersion = "unknown"
		}
		if machine.Status.Versions.ContainerRuntime.Name != runtimeName || machine.Status.Versions.ContainerRuntime.Version != runtimeVersion {
			machine.Status.Versions.ContainerRuntime.Name = runtimeName
			machine.Status.Versions.ContainerRuntime.Version = runtimeVersion
			updated = true
		}
	}

	if machine.Status.Versions.Kubelet != node.Status.NodeInfo.KubeletVersion {
		machine.Status.Versions.Kubelet = node.Status.NodeInfo.KubeletVersion
		updated = true
	}

	if updated {
		if machine, err = c.updateMachine(machine); err != nil {
			return fmt.Errorf("failed to update machine: %v", err)
		}
	}
	return nil
}

var (
	containerRuntime = regexp.MustCompile(`(docker|cri-o)://(.*)`)
)

func parseContainerRuntime(s string) (runtime, version string, err error) {
	res := containerRuntime.FindStringSubmatch(s)
	if len(res) == 3 {
		return res[1], res[2], nil
	}
	return "", "", fmt.Errorf("invalid format. Expected 'runtime://version'")
}

func (c *Controller) getNode(instance instance.Instance, provider string) (node *corev1.Node, exists bool, err error) {
	if instance == nil {
		return nil, false, fmt.Errorf("getNode called with nil provider instance!")
	}
	nodes, err := c.nodesLister.List(labels.Everything())
	if err != nil {
		return nil, false, err
	}

	providerID := fmt.Sprintf("%s:///%s", provider, instance.ID())
	for _, node := range nodes {
		if node.Spec.ProviderID == providerID {
			return node.DeepCopy(), true, nil
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
	if machine.Spec.Versions.Kubelet == "" {
		machine.Spec.Versions.Kubelet = latestKubernetesVersion
	}

	var err error
	if machine.Spec.Versions.ContainerRuntime.Name == "" {
		machine.Spec.Versions.ContainerRuntime.Name = containerruntime.Docker
		if machine, err = c.updateMachine(machine); err != nil {
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
		case containerruntime.CRIO:
			defaultVersions, err = crio.GetOfficiallySupportedVersions(machine.Spec.Versions.Kubelet)
			if err != nil {
				return nil, fmt.Errorf("failed to get a officially supported cri-o version for the given kubelet version: %v", err)
			}
		default:
			return nil, fmt.Errorf("invalid container runtime. Supported: '%s', '%s' ", containerruntime.Docker, containerruntime.CRIO)
		}

		providerSupportedVersions := prov.SupportedContainerRuntimes()
		for _, v := range defaultVersions {
			for _, sv := range providerSupportedVersions {
				if sv.Version == v {
					// we should not return asap as we prefer the highest supported version
					machine.Spec.Versions.ContainerRuntime.Version = sv.Version
				}
			}
		}
		if machine.Spec.Versions.ContainerRuntime.Version == "" {
			return nil, fmt.Errorf("no supported versions available for '%s'", machine.Spec.Versions.ContainerRuntime.Name)
		}
		if machine, err = c.updateMachine(machine); err != nil {
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

func (c *Controller) updateMachinesMetric() {
	machines, err := c.machinesLister.List(labels.Everything())
	if err != nil {
		glog.Errorf("failed to list machines for machines metric: %v", err)
		return
	}
	c.metrics.Machines.Set(float64(len(machines)))
}

func (c *Controller) updateNodesMetric() {
	nodes, err := c.nodesLister.List(labels.Everything())
	if err != nil {
		glog.Errorf("failed to list nodes for machine nodes metric: %v", err)
		return
	}

	machineNodes := 0
	for _, n := range nodes {
		ownerRef := metav1.GetControllerOf(n)
		if ownerRef != nil && ownerRef.Kind == machineKind {
			machineNodes++
		}
	}
	c.metrics.Nodes.Set(float64(machineNodes))
}

func (c *Controller) updateMetrics() {
	c.updateMachinesMetric()
	c.updateNodesMetric()
}
