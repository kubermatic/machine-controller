/*
Copyright 2019 The Machine Controller Authors.

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
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/heptiolabs/healthcheck"
	"github.com/kubermatic/machine-controller/pkg/apis/plugin"
	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/node/eviction"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	userdatamanager "github.com/kubermatic/machine-controller/pkg/userdata/manager"
	userdataplugin "github.com/kubermatic/machine-controller/pkg/userdata/plugin"
)

const (
	FinalizerDeleteInstance = "machine-delete-finalizer"
	FinalizerDeleteNode     = "machine-node-delete-finalizer"

	// AnnotationMachineUninitialized indicates that a machine is not yet
	// ready to be worked on by the machine-controller. The machine-controller
	// will ignore all machines that have this anotation with any value
	// Its value should consist of one or more initializers, separated by a comma
	AnnotationMachineUninitialized = "machine-controller.kubermatic.io/initializers"

	deletionRetryWaitPeriod = 10 * time.Second

	NodeOwnerLabelName = "machine-controller/owned-by"

	// AnnotationAutoscalerIdentifier is used by the cluster-autoscaler
	// cluster-api provider to match Nodes to Machines
	AnnotationAutoscalerIdentifier = "cluster.k8s.io/machine"
)

// Controller is the controller implementation for machine resources
type Controller struct {
	ctx        context.Context
	kubeClient kubernetes.Interface
	client     ctrlruntimeclient.Client

	workqueue workqueue.RateLimitingInterface
	recorder  record.EventRecorder

	metrics                          *MetricsCollection
	kubeconfigProvider               KubeconfigProvider
	providerData                     *cloudprovidertypes.ProviderData
	userDataManager                  *userdatamanager.Manager
	joinClusterTimeout               *time.Duration
	externalCloudProvider            bool
	name                             string
	bootstrapTokenServiceAccountName *types.NamespacedName
	skipEvictionAfter                time.Duration
	nodeSettings                     NodeSettings
}

type NodeSettings struct {
	// Translates to --cluster-dns on the kubelet.
	ClusterDNSIPs []net.IP
	// If set, this proxy will be configured on all nodes.
	HTTPProxy string
	// If set this will be set as NO_PROXY on the node.
	NoProxy string
	// If set, those registries will be configured as insecure on the container runtime.
	InsecureRegistries []string
	// If set, these mirrors will be take for pulling all required images on the node.
	RegistryMirrors []string
	// Translates to --pod-infra-container-image on the kubelet. If not set, the kubelet will default it.
	PauseImage string
	// The hyperkube image to use. Currently only Container Linux uses it.
	HyperkubeImage string
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

// NewMachineController returns a new machine controller.
func NewMachineController(
	kubeClient kubernetes.Interface,
	client ctrlruntimeclient.Client,
	recorder record.EventRecorder,
	metrics *MetricsCollection,
	prometheusRegistry prometheus.Registerer,
	machineInformer cache.SharedIndexInformer,
	nodeInformer cache.SharedIndexInformer,
	kubeconfigProvider KubeconfigProvider,
	providerData *cloudprovidertypes.ProviderData,
	joinClusterTimeout *time.Duration,
	externalCloudProvider bool,
	name string,
	bootstrapTokenServiceAccountName *types.NamespacedName,
	skipEvictionAfter time.Duration,
	nodeSettings NodeSettings,
) (*Controller, error) {

	if prometheusRegistry != nil {
		prometheusRegistry.MustRegister(metrics.Errors, metrics.Workers)
	}

	controller := &Controller{
		kubeClient: kubeClient,
		client:     client,

		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 5*time.Minute), "Machines"),
		recorder:  recorder,

		metrics:                          metrics,
		kubeconfigProvider:               kubeconfigProvider,
		providerData:                     providerData,
		joinClusterTimeout:               joinClusterTimeout,
		externalCloudProvider:            externalCloudProvider,
		name:                             name,
		bootstrapTokenServiceAccountName: bootstrapTokenServiceAccountName,
		skipEvictionAfter:                skipEvictionAfter,
		nodeSettings:                     nodeSettings,
	}

	m, err := userdatamanager.New()
	if err != nil {
		return nil, err
	}
	controller.userDataManager = m

	machineInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			controller.enqueueMachine(obj.(metav1.Object))
		},
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueMachine(new.(metav1.Object))
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
			// Dont do anything if the ready condition hasnt changed
			for _, newCondition := range newNode.Status.Conditions {
				if newCondition.Type != corev1.NodeReady {
					continue
				}
				for _, oldCondition := range oldNode.Status.Conditions {
					if oldCondition.Type != corev1.NodeReady {
						continue
					}
					if newCondition.Status == oldCondition.Status {
						return
					}
				}
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	utilruntime.ErrorHandlers = append(utilruntime.ErrorHandlers, func(err error) {
		controller.metrics.Errors.Add(1)
	})

	return controller, nil
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
	machine := &clusterv1alpha1.Machine{}
	if err := c.client.Get(c.ctx, types.NamespacedName{Namespace: namespace, Name: name}, machine); err != nil {
		if !kerrors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("failed to get Machine from lister: %v", err))
		}
		return
	}

	if machine.Status.ErrorMessage != nil || machine.Status.ErrorReason != nil {
		if err := c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			m.Status.ErrorMessage = nil
			m.Status.ErrorReason = nil
		}); err != nil {
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
	glog.V(6).Infof("Finished processing machine %s", key)
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
	node := &corev1.Node{}
	if err := c.client.Get(c.ctx, types.NamespacedName{Name: nodeRef.Name}, node); err != nil {
		return nil, err
	}
	return node, nil
}

func (c *Controller) updateMachine(m *clusterv1alpha1.Machine, modify ...cloudprovidertypes.MachineModifier) error {
	return c.providerData.Update(m, modify...)
}

// updateMachine updates machine's ErrorMessage and ErrorReason regardless if they were set or not
// this essentially overwrites previous values
func (c *Controller) updateMachineError(machine *clusterv1alpha1.Machine, reason common.MachineStatusError, message string) error {
	return c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		m.Status.ErrorMessage = &message
		m.Status.ErrorReason = &reason
	})
}

// updateMachineErrorIfTerminalError is a convenience method that will update machine's Status if the given err is terminal
// and at the same time terminal error will be returned to the caller
// otherwise it will return formatted error according to errMsg
func (c *Controller) updateMachineErrorIfTerminalError(machine *clusterv1alpha1.Machine, stReason common.MachineStatusError, stMessage string, err error, errMsg string) error {
	if ok, _, _ := cloudprovidererrors.IsTerminalError(err); ok {
		if errNested := c.updateMachineError(machine, stReason, stMessage); errNested != nil {
			return fmt.Errorf("failed to update machine error after due to %v, terminal error = %v", errNested, stMessage)
		}
		return err
	}
	return fmt.Errorf("%s, due to %v", errMsg, err)
}

func (c *Controller) createProviderInstance(prov cloudprovidertypes.Provider, machine *clusterv1alpha1.Machine, userdata string) (instance.Instance, error) {
	instance, err := prov.Create(machine, c.providerData, userdata)
	if err != nil {
		return nil, err
	}
	// Ensure finalizer is there
	_, err = c.ensureDeleteFinalizerExists(machine)
	return instance, err
}

func (c *Controller) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("failed to split metaNamespaceKey: %v", err)
	}
	machine := &clusterv1alpha1.Machine{}
	if err := c.client.Get(c.ctx, types.NamespacedName{Namespace: namespace, Name: name}, machine); err != nil {
		if kerrors.IsNotFound(err) {
			glog.V(2).Infof("machine '%s' in work queue no longer exists", key)
			return nil
		}
		return err
	}

	if machine.Annotations[AnnotationMachineUninitialized] != "" {
		glog.V(3).Infof("Ignoring machine %q because it has a non-empty %q annotation", machine.Name, AnnotationMachineUninitialized)
		return nil
	}

	recorderMachine := machine.DeepCopy()
	if err := c.sync(machine); err != nil {
		// We have no guarantee that machine is non-nil after reconciliation
		glog.Errorf("Failed to reconcile machine %q: %v", recorderMachine.Name, err)
		c.recorder.Eventf(recorderMachine, corev1.EventTypeWarning, "ReconcilingError", "%v", err)
	}
	return err
}

func (c *Controller) sync(machine *clusterv1alpha1.Machine) error {

	// This must stay in the controller, it can not be moved into the webhook
	// as the webhook does not get the name of machineset controller generated
	// machines on the CREATE request, because they only have `GenerateName` set,
	// not name: https://github.com/kubernetes-sigs/cluster-api/blob/852541448c3a1d847513a2ecf2cb75e2d4b91c2d/pkg/controller/machineset/controller.go#L290
	if machine.Spec.Name == "" {
		machine.Spec.Name = machine.Name
	}

	providerConfig, err := providerconfig.GetConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to get provider config: %v", err)
	}
	skg := providerconfig.NewConfigVarResolver(c.ctx, c.client)
	prov, err := cloudprovider.ForProvider(providerConfig.CloudProvider, skg)
	if err != nil {
		return fmt.Errorf("failed to get cloud provider %q: %v", providerConfig.CloudProvider, err)
	}

	// step 2: check if a user requested to delete the machine
	if machine.DeletionTimestamp != nil {
		return c.deleteMachine(prov, machine)
	}

	// Step 3: Essentially creates an instance for the given machine.
	userdataPlugin, err := c.userDataManager.ForOS(providerConfig.OperatingSystem)
	if err != nil {
		return fmt.Errorf("failed to userdata provider for '%s': %v", providerConfig.OperatingSystem, err)
	}

	// case 3.2: creates an instance if there is no node associated with the given machine
	if machine.Status.NodeRef == nil {
		return c.ensureInstanceExistsForMachine(prov, machine, userdataPlugin, providerConfig)
	}

	node, err := c.getNodeByNodeRef(machine.Status.NodeRef)
	if err != nil {
		//In case we cannot find a node for the NodeRef we must remove the NodeRef & recreate an instance on the next sync
		if kerrors.IsNotFound(err) {
			glog.V(3).Infof("found invalid NodeRef on machine %s. Deleting reference...", machine.Name)
			return c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
				m.Status.NodeRef = nil
			})
		}
		return fmt.Errorf("failed to check if node for machine exists: '%s'", err)
	}

	if c.nodeIsReady(node) {
		// We must do this to ensure the informers in the machineSet and machineDeployment controller
		// get triggered as soon as a ready node exists for a machine
		if err := c.ensureMachineHasNodeReadyCondition(machine); err != nil {
			return fmt.Errorf("failed to set nodeReady condition on machine: %v", err)
		}
	} else {
		// Node is not ready anymore? Maybe it got deleted
		return c.ensureInstanceExistsForMachine(prov, machine, userdataPlugin, providerConfig)
	}

	// case 3.3: if the node exists make sure if it has labels and taints attached to it.
	return c.ensureNodeLabelsAnnotationsAndTaints(node, machine)
}

func (c *Controller) ensureMachineHasNodeReadyCondition(machine *clusterv1alpha1.Machine) error {
	for _, condition := range machine.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return nil
		}
	}
	return c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		m.Status.Conditions = append(m.Status.Conditions, corev1.NodeCondition{Type: corev1.NodeReady,
			Status: corev1.ConditionTrue,
		})
	})
}

// evictIfNecessary checks if the machine has a node and evicts it if necessary
func (c *Controller) shouldEvict(machine *clusterv1alpha1.Machine) (bool, error) {
	// If the deletion got triggered a few hours ago, skip eviction.
	// We assume here that the eviction is blocked by misconfiguration or a misbehaving kubelet and/or controller-runtime
	if time.Since(machine.DeletionTimestamp.Time) > c.skipEvictionAfter {
		glog.V(0).Infof("Skipping eviction for machine %q since the deletion got triggered %.2f minutes ago", machine.Name, c.skipEvictionAfter.Minutes())
		return false, nil
	}

	// No node - Nothing to evict
	if machine.Status.NodeRef == nil {
		glog.V(4).Infof("Skipping eviction for machine %q since it does not have a node", machine.Name)
		return false, nil
	}

	node := &corev1.Node{}
	if err := c.client.Get(c.ctx, types.NamespacedName{Name: machine.Status.NodeRef.Name}, node); err != nil {
		// Node does not exist  - Nothing to evict
		if kerrors.IsNotFound(err) {
			glog.V(4).Infof("Skipping eviction for machine %q since it does not have a node", machine.Name)
			return false, nil
		}
		return false, fmt.Errorf("failed to get node %q", machine.Status.NodeRef.Name)
	}

	// We must check if an eviction is actually possible and only then return true
	// An eviction is possible when either:
	// * There is at least one machine without a valid NodeRef because that means it probably just got created
	// * There is at least one Node that is schedulable (`.Spec.Unschedulable == false`)
	machines := &clusterv1alpha1.MachineList{}
	if err := c.client.List(c.ctx, &ctrlruntimeclient.ListOptions{}, machines); err != nil {
		return false, fmt.Errorf("failed to get machines from lister: %v", err)
	}
	for _, machine := range machines.Items {
		if machine.Status.NodeRef == nil {
			return true, nil
		}
	}
	nodes := &corev1.NodeList{}
	if err := c.client.List(c.ctx, &ctrlruntimeclient.ListOptions{}, nodes); err != nil {
		return false, fmt.Errorf("failed to get nodes from lister: %v", err)
	}
	for _, node := range nodes.Items {
		// Don't consider our own node a valid target
		if node.Name == machine.Status.NodeRef.Name {
			continue
		}
		if !node.Spec.Unschedulable {
			return true, nil
		}
	}

	// If we arrived here we didn't find any machine without a NodeRef and we didn't
	// find any node that is schedulable, so eviction cant succeed
	glog.V(4).Infof("Skipping eviction for machine %q since there is no possible target for an eviction", machine.Name)
	return false, nil
}

// deleteMachine makes sure that an instance has gone in a series of steps.
func (c *Controller) deleteMachine(prov cloudprovidertypes.Provider, machine *clusterv1alpha1.Machine) error {
	shouldEvict, err := c.shouldEvict(machine)
	if err != nil {
		return err
	}

	if shouldEvict {
		evictedSomething, err := eviction.New(c.ctx, machine.Status.NodeRef.Name, c.client, c.kubeClient).Run()
		if err != nil {
			return fmt.Errorf("failed to evict node %s: %v", machine.Status.NodeRef.Name, err)
		}
		if evictedSomething {
			c.enqueueMachineAfter(machine, 10*time.Second)
			return nil
		}
	}

	if err := c.deleteCloudProviderInstance(prov, machine); err != nil {
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
		return err
	}

	return nil
}

func (c *Controller) deleteCloudProviderInstance(prov cloudprovidertypes.Provider, machine *clusterv1alpha1.Machine) error {
	finalizers := sets.NewString(machine.Finalizers...)
	if !finalizers.Has(FinalizerDeleteInstance) {
		return nil
	}

	// Delete the instance
	completelyGone, err := prov.Cleanup(machine, c.providerData)
	if err != nil {
		message := fmt.Sprintf("%v. Please manually delete %s finalizer from the machine object.", err, FinalizerDeleteInstance)
		return c.updateMachineErrorIfTerminalError(machine, common.DeleteMachineError, message, err, "failed to delete machine at cloud provider")
	}

	if !completelyGone {
		// As the instance is not completely gone yet, we need to recheck in a few seconds.
		c.enqueueMachineAfter(machine, deletionRetryWaitPeriod)
		return nil
	}

	return c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		finalizers := sets.NewString(m.Finalizers...)
		finalizers.Delete(FinalizerDeleteInstance)
		m.Finalizers = finalizers.List()
	})
}

func (c *Controller) deleteNodeForMachine(machine *clusterv1alpha1.Machine) error {
	requirement, err := labels.NewRequirement(NodeOwnerLabelName, selection.Equals, []string{string(machine.UID)})
	if err != nil {
		return fmt.Errorf("failed to parse requirement: %v", err)
	}
	listOpts := &ctrlruntimeclient.ListOptions{LabelSelector: labels.NewSelector().Add(*requirement)}
	nodes := &corev1.NodeList{}
	if err := c.client.List(c.ctx, listOpts, nodes); err != nil {
		return fmt.Errorf("failed to list nodes: %v", err)
	}

	for _, node := range nodes.Items {
		if err := c.client.Delete(c.ctx, &node); err != nil {
			return err
		}
	}

	return c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		finalizers := sets.NewString(m.Finalizers...)
		if finalizers.Has(FinalizerDeleteNode) {
			finalizers := sets.NewString(m.Finalizers...)
			finalizers.Delete(FinalizerDeleteNode)
			m.Finalizers = finalizers.List()
		}
	})
}

func (c *Controller) ensureInstanceExistsForMachine(prov cloudprovidertypes.Provider, machine *clusterv1alpha1.Machine, userdataPlugin userdataplugin.Provider, providerConfig *providerconfig.Config) error {
	glog.V(6).Infof("Requesting instance for machine '%s' from cloudprovider because no associated node with status ready found...", machine.Name)

	providerInstance, err := prov.Get(machine, c.providerData)

	// case 2: retrieving instance from provider was not successful
	if err != nil {

		// case 2.1: instance was not found and we are going to create one
		if err == cloudprovidererrors.ErrInstanceNotFound {
			glog.V(3).Infof("Validated machine spec of %s", machine.Name)

			kubeconfig, err := c.createBootstrapKubeconfig(machine.Name)
			if err != nil {
				return fmt.Errorf("failed to create bootstrap kubeconfig: %v", err)
			}

			cloudConfig, cloudProviderName, err := prov.GetCloudConfig(machine.Spec)
			if err != nil {
				return fmt.Errorf("failed to render cloud config: %v", err)
			}

			req := plugin.UserDataRequest{
				MachineSpec:           machine.Spec,
				Kubeconfig:            kubeconfig,
				CloudConfig:           cloudConfig,
				CloudProviderName:     cloudProviderName,
				ExternalCloudProvider: c.externalCloudProvider,
				DNSIPs:                c.nodeSettings.ClusterDNSIPs,
				InsecureRegistries:    c.nodeSettings.InsecureRegistries,
				RegistryMirrors:       c.nodeSettings.RegistryMirrors,
				PauseImage:            c.nodeSettings.PauseImage,
				HyperkubeImage:        c.nodeSettings.HyperkubeImage,
				NoProxy:               c.nodeSettings.NoProxy,
				HTTPProxy:             c.nodeSettings.HTTPProxy,
			}
			userdata, err := userdataPlugin.UserData(req)
			if err != nil {
				return fmt.Errorf("failed get userdata: %v", err)
			}

			// Create the instance
			if _, err = c.createProviderInstance(prov, machine, userdata); err != nil {
				message := fmt.Sprintf("%v. Unable to create a machine.", err)
				return c.updateMachineErrorIfTerminalError(machine, common.CreateMachineError, message, err, "failed to create machine at cloudprover")
			}
			c.recorder.Event(machine, corev1.EventTypeNormal, "Created", "Successfully created instance")
			glog.V(3).Infof("Created machine %s at cloud provider", machine.Name)
			// Reqeue the machine to make sure we notice if creation failed silently
			c.enqueueMachineAfter(machine, 30*time.Second)
			return nil
		}

		// case 2.2: terminal error was returned and manual interaction is required to recover
		if ok, _, _ := cloudprovidererrors.IsTerminalError(err); ok {
			message := fmt.Sprintf("%v. Unable to create a machine.", err)
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

	// case 3: retrieving the instance from cloudprovider was successful
	// Emit an event and update .Status.Addresses
	addresses := providerInstance.Addresses()
	eventMessage := fmt.Sprintf("Found instance at cloud provider, addresses: %v", addresses)
	c.recorder.Event(machine, corev1.EventTypeNormal, "InstanceFound", eventMessage)
	machineAddresses := []corev1.NodeAddress{}
	for _, address := range addresses {
		machineAddresses = append(machineAddresses, corev1.NodeAddress{Address: address})
	}
	if err := c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		m.Status.Addresses = machineAddresses
	}); err != nil {
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
			if err := c.updateNode(node, func(n *corev1.Node) {
				n.Labels[NodeOwnerLabelName] = string(machine.UID)
			}); err != nil {
				return fmt.Errorf("failed to update node %q after adding owner label: %v", node.Name, err)
			}
		}

		if node.Spec.ConfigSource == nil && machine.Spec.ConfigSource != nil {
			if err := c.updateNode(node, func(n *corev1.Node) {
				n.Spec.ConfigSource = machine.Spec.ConfigSource
			}); err != nil {
				return fmt.Errorf("failed to update node %s after setting the config source: %v", node.Name, err)
			}
			glog.V(3).Infof("Added config source to node %s (machine %s)", node.Name, machine.Name)
		}
		err = c.updateMachineStatus(machine, node)
		if err != nil {
			return fmt.Errorf("failed to update machine status: %v", err)
		}
	} else {
		// If the machine has an owner Ref and joinClusterTimeout is configured and reached, delete it to have it re-created by the MachineSet controller
		// Check if the machine is a potential candidate for triggering deletion
		if c.joinClusterTimeout != nil && ownerReferencesHasMachineSetKind(machine.OwnerReferences) {
			if time.Since(machine.CreationTimestamp.Time) > *c.joinClusterTimeout {
				if err := c.client.Delete(c.ctx, machine); err != nil {
					return fmt.Errorf("failed to delete machine %s/%s that didn't join cluster within expected period of %s: %v",
						machine.Namespace, machine.Name, c.joinClusterTimeout.String(), err)
				}
				return nil
			}
			// Re-enqueue the machine, because if it never joins the cluster nothing will trigger another sync on it once the timeout is reached
			c.enqueueMachineAfter(machine, 5*time.Minute)
		}
	}
	return nil
}

func ownerReferencesHasMachineSetKind(ownerReferences []metav1.OwnerReference) bool {
	for _, ownerReference := range ownerReferences {
		if ownerReference.Kind == "MachineSet" {
			return true
		}
	}
	return false
}

func (c *Controller) ensureNodeLabelsAnnotationsAndTaints(node *corev1.Node, machine *clusterv1alpha1.Machine) error {
	var modifiers []func(*corev1.Node)

	for k, v := range machine.Spec.Labels {
		if _, exists := node.Labels[k]; !exists {
			modifiers = append(modifiers, func(n *corev1.Node) {
				n.Labels[k] = v
			})
		}
	}

	for k, v := range machine.Spec.Annotations {
		if _, exists := node.Annotations[k]; !exists {
			modifiers = append(modifiers, func(n *corev1.Node) {
				n.Annotations[k] = v
			})
		}
	}
	autoscalerAnnotationValue := fmt.Sprintf("%s/%s", machine.Namespace, machine.Name)
	if node.Annotations[AnnotationAutoscalerIdentifier] != autoscalerAnnotationValue {
		modifiers = append(modifiers, func(n *corev1.Node) {
			n.Annotations[AnnotationAutoscalerIdentifier] = autoscalerAnnotationValue
		})
	}

	taintExists := func(node *corev1.Node, taint corev1.Taint) bool {
		for _, t := range node.Spec.Taints {
			if t.MatchTaint(&taint) {
				return true
			}
		}
		return false
	}
	for _, t := range machine.Spec.Taints {
		if !taintExists(node, t) {
			modifiers = append(modifiers, func(n *corev1.Node) {
				n.Spec.Taints = append(node.Spec.Taints, t)
			})
		}
	}

	if len(modifiers) > 0 {
		if err := c.updateNode(node, modifiers...); err != nil {
			return fmt.Errorf("failed to update node %s after setting labels/annotations/taints: %v", node.Name, err)
		}
		c.recorder.Event(machine, corev1.EventTypeNormal, "LabelsAnnotationsTaintsUpdated", "Successfully updated labels/annotations/taints")
		glog.V(3).Infof("Added labels/annotations/taints to node %s (machine %s)", node.Name, machine.Name)
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
		if err := c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
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
	nodes := &corev1.NodeList{}
	if err := c.client.List(c.ctx, &ctrlruntimeclient.ListOptions{}, nodes); err != nil {
		return nil, false, err
	}

	// We trim leading slashes in raw ID, since we always want three slashes in full ID
	providerID := fmt.Sprintf("%s:///%s", provider, strings.TrimLeft(instance.ID(), "/"))
	for _, node := range nodes.Items {
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

func (c *Controller) enqueueMachine(obj metav1.Object) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

func (c *Controller) enqueueMachineAfter(obj metav1.Object, after time.Duration) {
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
		glog.V(3).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}

	machinesList := &clusterv1alpha1.MachineList{}
	if err := c.client.List(c.ctx, &ctrlruntimeclient.ListOptions{}, machinesList); err != nil {
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
		for _, machine := range machinesList.Items {
			if machine.Status.NodeRef == nil {
				c.enqueueMachine(&machine)
			}
		}
	}

	for _, machine := range machinesList.Items {
		if string(machine.UID) == ownerUIDString {
			glog.V(6).Infof("Processing node: %s (machine=%s)", object.GetName(), machine.Name)
			c.enqueueMachine(&machine)
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
		if err := c.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			finalizers := sets.NewString(m.Finalizers...)
			finalizers.Insert(FinalizerDeleteInstance)
			finalizers.Insert(FinalizerDeleteNode)
			m.Finalizers = finalizers.List()
		}); err != nil {
			return nil, fmt.Errorf("failed to update machine after adding the delete instance finalizer: %v", err)
		}
		glog.V(3).Infof("Added delete finalizer to machine %s", machine.Name)
	}
	return machine, nil
}

func (c *Controller) updateNode(node *corev1.Node, modifiers ...func(*corev1.Node)) error {
	// Store name here, because the object can be nil if an update failed
	name := types.NamespacedName{Name: node.Name}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := c.client.Get(c.ctx, name, node); err != nil {
			return err
		}
		for _, modify := range modifiers {
			modify(node)
		}
		return c.client.Update(c.ctx, node)
	})
}
