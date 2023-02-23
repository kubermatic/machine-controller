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
	"strconv"
	"strings"
	"time"

	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/apis/plugin"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"
	"github.com/kubermatic/machine-controller/pkg/containerruntime"
	controllerutil "github.com/kubermatic/machine-controller/pkg/controller/util"
	kuberneteshelper "github.com/kubermatic/machine-controller/pkg/kubernetes"
	"github.com/kubermatic/machine-controller/pkg/node/eviction"
	"github.com/kubermatic/machine-controller/pkg/node/poddeletion"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/kubermatic/machine-controller/pkg/rhsm"
	userdatamanager "github.com/kubermatic/machine-controller/pkg/userdata/manager"
	userdataplugin "github.com/kubermatic/machine-controller/pkg/userdata/plugin"
	"github.com/kubermatic/machine-controller/pkg/userdata/rhel"
	"k8c.io/operating-system-manager/pkg/controllers/osc"
	osmresources "k8c.io/operating-system-manager/pkg/controllers/osc/resources"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	FinalizerDeleteInstance = "machine-delete-finalizer"
	FinalizerDeleteNode     = "machine-node-delete-finalizer"

	ControllerName = "machine_controller"

	// AnnotationMachineUninitialized indicates that a machine is not yet
	// ready to be worked on by the machine-controller. The machine-controller
	// will ignore all machines that have this anotation with any value
	// Its value should consist of one or more initializers, separated by a comma.
	AnnotationMachineUninitialized = "machine-controller.kubermatic.io/initializers"

	deletionRetryWaitPeriod = 10 * time.Second

	controllerNameLabelKey = "machine.k8s.io/controller"
	NodeOwnerLabelName     = "machine-controller/owned-by"

	// AnnotationAutoscalerIdentifier is used by the cluster-autoscaler
	// cluster-api provider to match Nodes to Machines.
	AnnotationAutoscalerIdentifier = "cluster.k8s.io/machine"

	CloudInitNotReadyError = "cloud-init configuration to %s machine: %v is not ready yet"
)

// Reconciler is the controller implementation for machine resources.
type Reconciler struct {
	kubeClient kubernetes.Interface
	client     ctrlruntimeclient.Client

	recorder record.EventRecorder

	metrics                          *MetricsCollection
	kubeconfigProvider               KubeconfigProvider
	providerData                     *cloudprovidertypes.ProviderData
	userDataManager                  *userdatamanager.Manager
	joinClusterTimeout               *time.Duration
	name                             string
	bootstrapTokenServiceAccountName *types.NamespacedName
	skipEvictionAfter                time.Duration
	nodeSettings                     NodeSettings
	redhatSubscriptionManager        rhsm.RedHatSubscriptionManager
	satelliteSubscriptionManager     rhsm.SatelliteSubscriptionManager

	useOSM                            bool
	nodePortRange                     string
	overrideBootstrapKubeletAPIServer string
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
	// Translates to feature gates on the kubelet.
	// Default: RotateKubeletServerCertificate=true
	KubeletFeatureGates map[string]bool
	// Used to set kubelet flag --cloud-provider=external
	ExternalCloudProvider bool
	// container runtime to install
	ContainerRuntime containerruntime.Config
	// Registry credentials secret object reference
	RegistryCredentialsSecretRef string
}

type KubeconfigProvider interface {
	GetKubeconfig(context.Context) (*clientcmdapi.Config, error)
	GetBearerToken() string
}

// MetricsCollection is a struct of all metrics used in
// this controller.
type MetricsCollection struct {
	Workers        prometheus.Gauge
	Errors         prometheus.Counter
	Provisioning   prometheus.Histogram
	Deprovisioning prometheus.Histogram
}

func (mc *MetricsCollection) MustRegister(registerer prometheus.Registerer) {
	registerer.MustRegister(
		mc.Errors,
		mc.Workers,
		mc.Provisioning,
		mc.Deprovisioning,
	)
}

func Add(
	ctx context.Context,
	mgr manager.Manager,
	kubeClient kubernetes.Interface,
	numWorkers int,
	metrics *MetricsCollection,
	kubeconfigProvider KubeconfigProvider,
	providerData *cloudprovidertypes.ProviderData,
	joinClusterTimeout *time.Duration,
	name string,
	bootstrapTokenServiceAccountName *types.NamespacedName,
	skipEvictionAfter time.Duration,
	nodeSettings NodeSettings,
	useOSM bool,
	nodePortRange string,
	overrideBootstrapKubeletAPIServer string,
) error {
	reconciler := &Reconciler{
		kubeClient:                       kubeClient,
		client:                           mgr.GetClient(),
		recorder:                         mgr.GetEventRecorderFor(ControllerName),
		metrics:                          metrics,
		kubeconfigProvider:               kubeconfigProvider,
		providerData:                     providerData,
		joinClusterTimeout:               joinClusterTimeout,
		name:                             name,
		bootstrapTokenServiceAccountName: bootstrapTokenServiceAccountName,
		skipEvictionAfter:                skipEvictionAfter,
		nodeSettings:                     nodeSettings,
		redhatSubscriptionManager:        rhsm.NewRedHatSubscriptionManager(),
		satelliteSubscriptionManager:     rhsm.NewSatelliteSubscriptionManager(),

		useOSM:                            useOSM,
		nodePortRange:                     nodePortRange,
		overrideBootstrapKubeletAPIServer: overrideBootstrapKubeletAPIServer,
	}
	m, err := userdatamanager.New()
	if err != nil {
		return fmt.Errorf("failed to create userdatamanager: %w", err)
	}
	reconciler.userDataManager = m

	utilruntime.ErrorHandlers = append(utilruntime.ErrorHandlers, func(error) {
		reconciler.metrics.Errors.Add(1)
	})

	c, err := controller.New(ControllerName, mgr,
		controller.Options{Reconciler: reconciler, MaxConcurrentReconciles: numWorkers})
	if err != nil {
		return err
	}
	if err := c.Watch(&source.Kind{Type: &clusterv1alpha1.Machine{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	metrics.Workers.Set(float64(numWorkers))

	return c.Watch(
		&source.Kind{Type: &corev1.Node{}},
		handler.EnqueueRequestsFromMapFunc(func(node client.Object) (result []reconcile.Request) {
			machinesList := &clusterv1alpha1.MachineList{}
			if err := mgr.GetClient().List(ctx, machinesList); err != nil {
				utilruntime.HandleError(fmt.Errorf("failed to list machines in lister: %w", err))
				return
			}

			var ownerUIDString string
			var exists bool
			if nodeLabels := node.GetLabels(); nodeLabels != nil {
				ownerUIDString, exists = nodeLabels[NodeOwnerLabelName]
			}
			if !exists {
				// We get triggered by node{Add,Update}, so enqeue machines if they
				// have no nodeRef yet to make matching happen ASAP
				for _, machine := range machinesList.Items {
					if machine.Status.NodeRef == nil {
						result = append(result, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: machine.Namespace,
								Name:      machine.Name}})
					}
				}
				return
			}

			for _, machine := range machinesList.Items {
				if string(machine.UID) == ownerUIDString {
					klog.V(6).Infof("Processing node: %s (machine=%s)", node.GetName(), machine.Name)
					return []reconcile.Request{{NamespacedName: types.NamespacedName{
						Namespace: machine.Namespace,
						Name:      machine.Name,
					}}}
				}
			}
			return
		}),
		predicate.Funcs{UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode := e.ObjectOld.(*corev1.Node)
			newNode := e.ObjectNew.(*corev1.Node)
			if newNode.ResourceVersion == oldNode.ResourceVersion {
				return false
			}
			// Don't do anything if the ready condition hasn't changed
			for _, newCondition := range newNode.Status.Conditions {
				if newCondition.Type != corev1.NodeReady {
					continue
				}
				for _, oldCondition := range oldNode.Status.Conditions {
					if oldCondition.Type != corev1.NodeReady {
						continue
					}
					if newCondition.Status == oldCondition.Status {
						return false
					}
				}
			}
			return true
		}},
	)
}

// clearMachineError is a convenience function to remove a error on the machine if its set.
// It does not return an error as it's used around the sync handler.
func (r *Reconciler) clearMachineError(machine *clusterv1alpha1.Machine) {
	if machine.Status.ErrorMessage != nil || machine.Status.ErrorReason != nil {
		if err := r.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			m.Status.ErrorMessage = nil
			m.Status.ErrorReason = nil
		}); err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to update machine: %w", err))
		}
	}
}

func nodeIsReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

func (r *Reconciler) getNodeByNodeRef(ctx context.Context, nodeRef *corev1.ObjectReference) (*corev1.Node, error) {
	node := &corev1.Node{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: nodeRef.Name}, node); err != nil {
		return nil, err
	}
	return node, nil
}

func (r *Reconciler) updateMachine(m *clusterv1alpha1.Machine, modify ...cloudprovidertypes.MachineModifier) error {
	return r.providerData.Update(m, modify...)
}

// updateMachine updates machine's ErrorMessage and ErrorReason regardless if they were set or not
// this essentially overwrites previous values.
func (r *Reconciler) updateMachineError(machine *clusterv1alpha1.Machine, reason common.MachineStatusError, message string) error {
	return r.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		m.Status.ErrorMessage = &message
		m.Status.ErrorReason = &reason
	})
}

// updateMachineErrorIfTerminalError is a convenience method that will update machine's Status if the given err is terminal
// and at the same time terminal error will be returned to the caller
// otherwise it will return formatted error according to errMsg.
func (r *Reconciler) updateMachineErrorIfTerminalError(machine *clusterv1alpha1.Machine, stReason common.MachineStatusError, stMessage string, err error, errMsg string) error {
	if ok, _, _ := cloudprovidererrors.IsTerminalError(err); ok {
		if errNested := r.updateMachineError(machine, stReason, stMessage); errNested != nil {
			return fmt.Errorf("failed to update machine error after due to %w, terminal error = %v", errNested, stMessage)
		}
		return err
	}
	return fmt.Errorf("%s, due to %w", errMsg, err)
}

func (r *Reconciler) createProviderInstance(ctx context.Context, prov cloudprovidertypes.Provider, machine *clusterv1alpha1.Machine, userdata string) (instance.Instance, error) {
	// Ensure finalizer is there.
	_, err := r.ensureDeleteFinalizerExists(machine)
	if err != nil {
		return nil, fmt.Errorf("failed to add %q finalizer: %w", FinalizerDeleteInstance, err)
	}
	i, err := prov.Create(ctx, machine, r.providerData, userdata)
	if err != nil {
		return nil, err
	}
	return i, nil
}

func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	machine := &clusterv1alpha1.Machine{}
	if err := r.client.Get(ctx, request.NamespacedName, machine); err != nil {
		if kerrors.IsNotFound(err) {
			klog.V(2).Infof("machine %q in work queue no longer exists", request.NamespacedName.String())
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if machine.Labels[controllerNameLabelKey] != r.name {
		klog.V(3).Infof("Ignoring machine %q because its worker-name doesn't match", request.NamespacedName.String())
		return reconcile.Result{}, nil
	}

	if machine.Annotations[AnnotationMachineUninitialized] != "" {
		klog.V(3).Infof("Ignoring machine %q because it has a non-empty %q annotation", machine.Name, AnnotationMachineUninitialized)
		return reconcile.Result{}, nil
	}

	recorderMachine := machine.DeepCopy()
	result, err := r.reconcile(ctx, machine)
	if err != nil {
		// We have no guarantee that machine is non-nil after reconciliation
		klog.Errorf("Failed to reconcile machine %q: %v", recorderMachine.Name, err)
		r.recorder.Eventf(recorderMachine, corev1.EventTypeWarning, "ReconcilingError", "%v", err)
	} else {
		r.clearMachineError(machine)
	}
	if result == nil {
		result = &reconcile.Result{}
	}
	return *result, err
}

func (r *Reconciler) reconcile(ctx context.Context, machine *clusterv1alpha1.Machine) (*reconcile.Result, error) {
	// This must stay in the controller, it can not be moved into the webhook
	// as the webhook does not get the name of machineset controller generated
	// machines on the CREATE request, because they only have `GenerateName` set,
	// not name: https://github.com/kubernetes-sigs/cluster-api/blob/852541448c3a1d847513a2ecf2cb75e2d4b91c2d/pkg/controller/machineset/controller.go#L290
	if machine.Spec.Name == "" {
		machine.Spec.Name = machine.Name
	}

	providerConfig, err := providerconfigtypes.GetConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider config: %w", err)
	}
	skg := providerconfig.NewConfigVarResolver(ctx, r.client)
	prov, err := cloudprovider.ForProvider(providerConfig.CloudProvider, skg)
	if err != nil {
		return nil, fmt.Errorf("failed to get cloud provider %q: %w", providerConfig.CloudProvider, err)
	}

	// step 2: check if a user requested to delete the machine
	if machine.DeletionTimestamp != nil {
		return r.deleteMachine(ctx, prov, providerConfig.CloudProvider, machine)
	}

	// Step 3: Essentially creates an instance for the given machine.
	userdataPlugin, err := r.userDataManager.ForOS(providerConfig.OperatingSystem)
	if err != nil {
		return nil, fmt.Errorf("failed to userdata provider for '%s': %w", providerConfig.OperatingSystem, err)
	}

	// case 3.2: creates an instance if there is no node associated with the given machine
	if machine.Status.NodeRef == nil {
		return r.ensureInstanceExistsForMachine(ctx, prov, machine, userdataPlugin, providerConfig)
	}

	node, err := r.getNodeByNodeRef(ctx, machine.Status.NodeRef)
	if err != nil {
		// In case we cannot find a node for the NodeRef we must remove the NodeRef & recreate an instance on the next sync
		if kerrors.IsNotFound(err) {
			klog.V(3).Infof("found invalid NodeRef on machine %s. Deleting reference...", machine.Name)
			return nil, r.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
				m.Status.NodeRef = nil
			})
		}
		return nil, fmt.Errorf("failed to check if node for machine exists: '%w'", err)
	}

	if nodeIsReady(node) {
		// We must do this to ensure the informers in the machineSet and machineDeployment controller
		// get triggered as soon as a ready node exists for a machine
		if err := r.ensureMachineHasNodeReadyCondition(machine); err != nil {
			return nil, fmt.Errorf("failed to set nodeReady condition on machine: %w", err)
		}
	} else {
		// Node is not ready anymore? Maybe it got deleted
		return r.ensureInstanceExistsForMachine(ctx, prov, machine, userdataPlugin, providerConfig)
	}

	// case 3.3: if the node exists make sure if it has labels and taints attached to it.
	return nil, r.ensureNodeLabelsAnnotationsAndTaints(ctx, node, machine)
}

func (r *Reconciler) ensureMachineHasNodeReadyCondition(machine *clusterv1alpha1.Machine) error {
	for _, condition := range machine.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return nil
		}
	}

	r.metrics.Provisioning.Observe(time.Until(machine.CreationTimestamp.Time).Abs().Seconds())

	return r.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		m.Status.Conditions = append(m.Status.Conditions, corev1.NodeCondition{Type: corev1.NodeReady,
			Status: corev1.ConditionTrue,
		})
	})
}

func (r *Reconciler) shouldCleanupVolumes(ctx context.Context, machine *clusterv1alpha1.Machine, providerName providerconfigtypes.CloudProvider) (bool, error) {
	// we need to wait for volumeAttachments clean up only for vSphere
	if providerName != providerconfigtypes.CloudProviderVsphere {
		return false, nil
	}

	// No node - No volumeAttachments to be collected
	if machine.Status.NodeRef == nil {
		klog.V(4).Infof("Skipping eviction for machine %q since it does not have a node", machine.Name)
		return false, nil
	}

	node := &corev1.Node{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: machine.Status.NodeRef.Name}, node); err != nil {
		// Node does not exist - No volumeAttachments to be collected
		if kerrors.IsNotFound(err) {
			klog.V(4).Infof("Skipping eviction for machine %q since it does not have a node", machine.Name)
			return false, nil
		}
		return false, fmt.Errorf("failed to get node %q", machine.Status.NodeRef.Name)
	}
	return true, nil
}

// evictIfNecessary checks if the machine has a node and evicts it if necessary.
func (r *Reconciler) shouldEvict(ctx context.Context, machine *clusterv1alpha1.Machine) (bool, error) {
	// If the deletion got triggered a few hours ago, skip eviction.
	// We assume here that the eviction is blocked by misconfiguration or a misbehaving kubelet and/or controller-runtime
	if time.Since(machine.DeletionTimestamp.Time) > r.skipEvictionAfter {
		klog.V(0).Infof("Skipping eviction for machine %q since the deletion got triggered %.2f minutes ago", machine.Name, r.skipEvictionAfter.Minutes())
		return false, nil
	}

	// No node - Nothing to evict
	if machine.Status.NodeRef == nil {
		klog.V(4).Infof("Skipping eviction for machine %q since it does not have a node", machine.Name)
		return false, nil
	}

	node := &corev1.Node{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: machine.Status.NodeRef.Name}, node); err != nil {
		// Node does not exist  - Nothing to evict
		if kerrors.IsNotFound(err) {
			klog.V(4).Infof("Skipping eviction for machine %q since it does not have a node", machine.Name)
			return false, nil
		}
		return false, fmt.Errorf("failed to get node %q", machine.Status.NodeRef.Name)
	}

	// We must check if an eviction is actually possible and only then return true
	// An eviction is possible when either:
	// * There is at least one machine without a valid NodeRef because that means it probably just got created
	// * There is at least one Node that is schedulable (`.Spec.Unschedulable == false`)
	machines := &clusterv1alpha1.MachineList{}
	if err := r.client.List(ctx, machines); err != nil {
		return false, fmt.Errorf("failed to get machines from lister: %w", err)
	}
	for _, machine := range machines.Items {
		if machine.Status.NodeRef == nil {
			return true, nil
		}
	}
	nodes := &corev1.NodeList{}
	if err := r.client.List(ctx, nodes); err != nil {
		return false, fmt.Errorf("failed to get nodes from lister: %w", err)
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
	// find any node that is schedulable, so eviction can't succeed
	klog.V(4).Infof("Skipping eviction for machine %q since there is no possible target for an eviction", machine.Name)
	return false, nil
}

// deleteMachine makes sure that an instance has gone in a series of steps.
func (r *Reconciler) deleteMachine(ctx context.Context, prov cloudprovidertypes.Provider, providerName providerconfigtypes.CloudProvider, machine *clusterv1alpha1.Machine) (*reconcile.Result, error) {
	shouldEvict, err := r.shouldEvict(ctx, machine)
	if err != nil {
		return nil, err
	}
	shouldCleanUpVolumes, err := r.shouldCleanupVolumes(ctx, machine, providerName)
	if err != nil {
		return nil, err
	}

	var evictedSomething, deletedSomething bool
	var volumesFree = true
	if shouldEvict {
		evictedSomething, err = eviction.New(ctx, machine.Status.NodeRef.Name, r.client, r.kubeClient).Run()
		if err != nil {
			return nil, fmt.Errorf("failed to evict node %s: %w", machine.Status.NodeRef.Name, err)
		}
	}
	if shouldCleanUpVolumes {
		deletedSomething, volumesFree, err = poddeletion.New(ctx, machine.Status.NodeRef.Name, r.client, r.kubeClient).Run()
		if err != nil {
			return nil, fmt.Errorf("failed to delete pods bound to volumes running on node %s: %w", machine.Status.NodeRef.Name, err)
		}
	}

	if evictedSomething || deletedSomething || !volumesFree {
		return &reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if result, err := r.deleteCloudProviderInstance(ctx, prov, machine); result != nil || err != nil {
		return result, err
	}

	// Delete the node object only after the instance is gone, `deleteCloudProviderInstance`
	// returns with a nil-error after it triggers the instance deletion but it is async for
	// some providers hence the instance deletion may not been executed yet
	// `FinalizerDeleteInstance` stays until the instance is really gone thought, so we check
	// for that here
	if sets.NewString(machine.Finalizers...).Has(FinalizerDeleteInstance) {
		return nil, nil
	}

	nodes, err := r.retrieveNodesRelatedToMachine(ctx, machine)
	if err != nil {
		return nil, err
	}

	if err := r.deleteNodeForMachine(ctx, nodes, machine); err != nil {
		return nil, err
	}

	r.metrics.Deprovisioning.Observe(time.Until(machine.DeletionTimestamp.Time).Abs().Seconds())

	return nil, nil
}

func (r *Reconciler) retrieveNodesRelatedToMachine(ctx context.Context, machine *clusterv1alpha1.Machine) ([]*corev1.Node, error) {
	nodes := make([]*corev1.Node, 0)

	// If there's NodeRef on the Machine object, retrieve the node by using the
	// value of the NodeRef. If there's no NodeRef, try to find the Node by
	// listing nodes using the NodeOwner label selector.
	if machine.Status.NodeRef != nil {
		objKey := ctrlruntimeclient.ObjectKey{Name: machine.Status.NodeRef.Name}
		node := &corev1.Node{}
		if err := r.client.Get(ctx, objKey, node); err != nil {
			if !kerrors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get node %s: %w", machine.Status.NodeRef.Name, err)
			}
			klog.V(2).Infof("node %q does not longer exist for machine %q", machine.Status.NodeRef.Name, machine.Spec.Name)
		} else {
			nodes = append(nodes, node)
		}
	} else {
		selector, err := labels.Parse(NodeOwnerLabelName + "=" + string(machine.UID))
		if err != nil {
			return nil, fmt.Errorf("failed to parse label selector: %w", err)
		}
		listOpts := &ctrlruntimeclient.ListOptions{LabelSelector: selector}
		nodeList := &corev1.NodeList{}
		if err := r.client.List(ctx, nodeList, listOpts); err != nil {
			return nil, fmt.Errorf("failed to list nodes: %w", err)
		}
		if len(nodeList.Items) == 0 {
			// We just want log that we didn't found the node.
			klog.V(3).Infof("No node found for the machine %s", machine.Spec.Name)
		}

		for i := range nodeList.Items {
			nodes = append(nodes, &nodeList.Items[i])
		}
	}

	return nodes, nil
}

func (r *Reconciler) deleteCloudProviderInstance(ctx context.Context, prov cloudprovidertypes.Provider, machine *clusterv1alpha1.Machine) (*reconcile.Result, error) {
	finalizers := sets.NewString(machine.Finalizers...)
	if !finalizers.Has(FinalizerDeleteInstance) {
		return nil, nil
	}

	// Delete the instance
	completelyGone, err := prov.Cleanup(ctx, machine, r.providerData)
	if err != nil {
		message := fmt.Sprintf("%v. Please manually delete %s finalizer from the machine object.", err, FinalizerDeleteInstance)
		return nil, r.updateMachineErrorIfTerminalError(machine, common.DeleteMachineError, message, err, "failed to delete machine at cloud provider")
	}

	if !completelyGone {
		// As the instance is not completely gone yet, we need to recheck in a few seconds.
		return &reconcile.Result{RequeueAfter: deletionRetryWaitPeriod}, nil
	}

	machineConfig, err := providerconfigtypes.GetConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider config: %w", err)
	}

	if machineConfig.OperatingSystem == providerconfigtypes.OperatingSystemRHEL {
		rhelConfig, err := rhel.LoadConfig(machineConfig.OperatingSystemSpec)
		if err != nil {
			return nil, fmt.Errorf("failed to get rhel os specs: %w", err)
		}

		machineName := machine.Name
		if machineConfig.CloudProvider == providerconfigtypes.CloudProviderAWS {
			for _, address := range machine.Status.Addresses {
				if address.Type == corev1.NodeInternalDNS {
					machineName = address.Address
				}
			}
		}

		if rhelConfig.RHSMOfflineToken != "" {
			if err := r.redhatSubscriptionManager.UnregisterInstance(ctx, rhelConfig.RHSMOfflineToken, machineName); err != nil {
				return nil, fmt.Errorf("failed to delete subscription for machine name %s: %w", machine.Name, err)
			}
		}

		if rhelConfig.RHELUseSatelliteServer {
			if kuberneteshelper.HasFinalizer(machine, rhsm.RedhatSubscriptionFinalizer) {
				err = r.satelliteSubscriptionManager.DeleteSatelliteHost(
					machineName,
					rhelConfig.RHELSubscriptionManagerUser,
					rhelConfig.RHELSubscriptionManagerPassword,
					rhelConfig.RHELSatelliteServer)
				if err != nil {
					return nil, fmt.Errorf("failed to delete redhat satellite host for machine name %s: %w", machine.Name, err)
				}
			}
		}

		if err := rhsm.RemoveRHELSubscriptionFinalizer(machine, r.updateMachine); err != nil {
			return nil, fmt.Errorf("failed to remove redhat subscription finalizer: %w", err)
		}
	}

	return nil, r.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		finalizers := sets.NewString(m.Finalizers...)
		finalizers.Delete(FinalizerDeleteInstance)
		m.Finalizers = finalizers.List()
	})
}

func (r *Reconciler) deleteNodeForMachine(ctx context.Context, nodes []*corev1.Node, machine *clusterv1alpha1.Machine) error {
	// iterates on all nodes and delete them. Finally, remove the finalizer on the machine
	for _, node := range nodes {
		if err := r.client.Delete(ctx, node); err != nil {
			if !kerrors.IsNotFound(err) {
				return err
			}
			klog.V(2).Infof("node %q does not longer exist for machine %q", machine.Status.NodeRef.Name, machine.Spec.Name)
		}
	}

	return r.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		finalizers := sets.NewString(m.Finalizers...)
		if finalizers.Has(FinalizerDeleteNode) {
			finalizers := sets.NewString(m.Finalizers...)
			finalizers.Delete(FinalizerDeleteNode)
			m.Finalizers = finalizers.List()
		}
	})
}

func (r *Reconciler) ensureInstanceExistsForMachine(
	ctx context.Context,
	prov cloudprovidertypes.Provider,
	machine *clusterv1alpha1.Machine,
	userdataPlugin userdataplugin.Provider,
	providerConfig *providerconfigtypes.Config,
) (*reconcile.Result, error) {
	klog.V(6).Infof("Requesting instance for machine '%s' from cloudprovider because no associated node with status ready found...", machine.Name)

	providerInstance, err := prov.Get(ctx, machine, r.providerData)

	// case 2: retrieving instance from provider was not successful
	if err != nil {
		// case 2.1: instance was not found and we are going to create one
		if errors.Is(err, cloudprovidererrors.ErrInstanceNotFound) {
			klog.V(3).Infof("Validated machine spec of %s", machine.Name)

			var kubeconfig *clientcmdapi.Config

			// OSM will take care of the bootstrap kubeconfig and token by itself.
			if !r.useOSM {
				kubeconfig, err = r.createBootstrapKubeconfig(ctx, machine.Name)
				if err != nil {
					return nil, fmt.Errorf("failed to create bootstrap kubeconfig: %w", err)
				}
			}

			cloudConfig, kubeletCloudProviderName, err := prov.GetCloudConfig(machine.Spec)
			if err != nil {
				return nil, fmt.Errorf("failed to render cloud config: %w", err)
			}

			// grab kubelet featureGates from the annotations
			kubeletFeatureGates := common.GetKubeletFeatureGates(machine.GetAnnotations())
			if len(kubeletFeatureGates) == 0 {
				// fallback to command-line input
				kubeletFeatureGates = r.nodeSettings.KubeletFeatureGates
			}

			// grab kubelet general options from the annotations
			kubeletFlags := common.GetKubeletFlags(machine.GetAnnotations())
			kubeletConfigs := common.GetKubeletConfigs(machine.GetAnnotations())

			// look up for ExternalCloudProvider feature, with fallback to command-line input
			externalCloudProvider := r.nodeSettings.ExternalCloudProvider
			if val, ok := kubeletFlags[common.ExternalCloudProviderKubeletFlag]; ok {
				externalCloudProvider, _ = strconv.ParseBool(val)
			}

			registryCredentials, err := containerruntime.GetContainerdAuthConfig(ctx, r.client, r.nodeSettings.RegistryCredentialsSecretRef)
			if err != nil {
				return nil, fmt.Errorf("failed to get containerd auth config: %w", err)
			}

			crRuntime := r.nodeSettings.ContainerRuntime
			crRuntime.RegistryCredentials = registryCredentials

			if val, ok := kubeletConfigs[common.ContainerLogMaxSizeKubeletConfig]; ok {
				crRuntime.ContainerLogMaxSize = val
			}

			if val, ok := kubeletConfigs[common.ContainerLogMaxFilesKubeletConfig]; ok {
				crRuntime.ContainerLogMaxFiles = val
			}

			req := plugin.UserDataRequest{
				MachineSpec:              machine.Spec,
				Kubeconfig:               kubeconfig,
				CloudConfig:              cloudConfig,
				CloudProviderName:        string(providerConfig.CloudProvider),
				ExternalCloudProvider:    externalCloudProvider,
				DNSIPs:                   r.nodeSettings.ClusterDNSIPs,
				PauseImage:               r.nodeSettings.PauseImage,
				KubeletCloudProviderName: kubeletCloudProviderName,
				KubeletFeatureGates:      kubeletFeatureGates,
				KubeletConfigs:           kubeletConfigs,
				NoProxy:                  r.nodeSettings.NoProxy,
				HTTPProxy:                r.nodeSettings.HTTPProxy,
				ContainerRuntime:         crRuntime,
				NodePortRange:            r.nodePortRange,
			}

			// Here we do stuff!
			var userdata string

			if r.useOSM {
				referencedMachineDeployment, machineDeploymentRevision, err := controllerutil.GetMachineDeploymentNameAndRevisionForMachine(ctx, machine, r.client)
				if err != nil {
					return nil, fmt.Errorf("failed to find machine's MachineDployment: %w", err)
				}

				// We need to ensure that both provisoning and bootstrapping secrets have been created. And that the revision
				// matches with the machine deployment revision
				provisioningSecretName := fmt.Sprintf(osmresources.CloudConfigSecretNamePattern,
					referencedMachineDeployment,
					machine.Namespace,
					osmresources.ProvisioningCloudConfig)

				// Ensure that the provisioning secret exists
				provisioningSecret := &corev1.Secret{}
				if err := r.client.Get(ctx,
					types.NamespacedName{Name: provisioningSecretName, Namespace: util.CloudInitNamespace},
					provisioningSecret); err != nil {
					klog.Errorf(CloudInitNotReadyError, osmresources.ProvisioningCloudConfig, machine.Name)
					return nil, err
				}

				provisioningSecretRevision := provisioningSecret.Annotations[osc.MachineDeploymentRevision]
				if provisioningSecretRevision != machineDeploymentRevision {
					return nil, fmt.Errorf(CloudInitNotReadyError, osmresources.ProvisioningCloudConfig, machine.Name)
				}

				bootstrapSecretName := fmt.Sprintf(osmresources.CloudConfigSecretNamePattern,
					referencedMachineDeployment,
					machine.Namespace,
					osmresources.BootstrapCloudConfig)

				bootstrapSecret := &corev1.Secret{}
				if err := r.client.Get(ctx,
					types.NamespacedName{Name: bootstrapSecretName, Namespace: util.CloudInitNamespace},
					bootstrapSecret); err != nil {
					klog.Errorf(CloudInitNotReadyError, osmresources.BootstrapCloudConfig, machine.Name)
					return nil, err
				}

				bootstrapSecretRevision := bootstrapSecret.Annotations[osc.MachineDeploymentRevision]
				if bootstrapSecretRevision != machineDeploymentRevision {
					return nil, fmt.Errorf(CloudInitNotReadyError, osmresources.BootstrapCloudConfig, machine.Name)
				}

				userdata = getOSMBootstrapUserdata(req.MachineSpec.Name, *bootstrapSecret)
			} else {
				userdata, err = userdataPlugin.UserData(req)
				if err != nil {
					return nil, fmt.Errorf("failed get userdata: %w", err)
				}
			}

			// Create the instance
			if _, err = r.createProviderInstance(ctx, prov, machine, userdata); err != nil {
				message := fmt.Sprintf("%v. Unable to create a machine.", err)
				return nil, r.updateMachineErrorIfTerminalError(machine, common.CreateMachineError, message, err, "failed to create machine at cloudprovider")
			}
			if providerConfig.OperatingSystem == providerconfigtypes.OperatingSystemRHEL {
				if err := rhsm.AddRHELSubscriptionFinalizer(machine, r.updateMachine); err != nil {
					return nil, fmt.Errorf("failed to add redhat subscription finalizer: %w", err)
				}
			}
			r.recorder.Event(machine, corev1.EventTypeNormal, "Created", "Successfully created instance")
			klog.V(3).Infof("Created machine %s at cloud provider", machine.Name)
			// Reqeue the machine to make sure we notice if creation failed silently
			return &reconcile.Result{RequeueAfter: 30 * time.Second}, nil
		}

		// case 2.2: terminal error was returned and manual interaction is required to recover
		if ok, _, _ := cloudprovidererrors.IsTerminalError(err); ok {
			message := fmt.Sprintf("%v. Unable to create a machine.", err)
			return nil, r.updateMachineErrorIfTerminalError(machine, common.CreateMachineError, message, err, "failed to get instance from provider")
		}

		// case 2.3: transient error was returned, requeue the request and try again in the future
		return nil, fmt.Errorf("failed to get instance from provider: %w", err)
	}
	// Instance exists, so ensure finalizer does as well
	machine, err = r.ensureDeleteFinalizerExists(machine)
	if err != nil {
		return nil, err
	}

	// case 3: retrieving the instance from cloudprovider was successful
	// Emit an event and update .Status.Addresses
	addresses := providerInstance.Addresses()
	eventMessage := fmt.Sprintf("Found instance at cloud provider, addresses: %v", addresses)
	r.recorder.Event(machine, corev1.EventTypeNormal, "InstanceFound", eventMessage)
	// It might happen that we got here, but we still don't have IP addresses
	// for the instance. In that case it doesn't make sense to proceed because:
	//   * if we match Node by ProviderID, Machine will get NodeOwnerRef, but
	//     there will be no IP address on that Machine object. Since we
	//     successfully set NodeOwnerRef, Machine will not be reconciled again,
	//     so it will never get IP addresses. This breaks the NodeCSRApprover
	//     workflow because NodeCSRApprover cannot validate certificates without
	//     IP addresses, resulting in a broken Node
	//   * if we can't match Node by ProviderID, fallback to matching by IP
	//     address will not have any result because we still don't have IP
	//     addresses for that instance
	// Considering that, we just retry after 15 seconds, hoping that we'll
	// get IP addresses by then.
	if len(addresses) == 0 {
		return &reconcile.Result{RequeueAfter: 15 * time.Second}, nil
	}

	machineAddresses := []corev1.NodeAddress{}
	for address, addressType := range addresses {
		machineAddresses = append(machineAddresses, corev1.NodeAddress{Address: address, Type: addressType})
	}
	if err := r.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
		m.Status.Addresses = machineAddresses
	}); err != nil {
		return nil, fmt.Errorf("failed to update machine after setting .status.addresses: %w", err)
	}
	return r.ensureNodeOwnerRefAndConfigSource(ctx, providerInstance, machine, providerConfig)
}

func (r *Reconciler) ensureNodeOwnerRefAndConfigSource(ctx context.Context, providerInstance instance.Instance, machine *clusterv1alpha1.Machine, providerConfig *providerconfigtypes.Config) (*reconcile.Result, error) {
	node, exists, err := r.getNode(ctx, providerInstance, providerConfig.CloudProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get node for machine %s: %w", machine.Name, err)
	}

	if exists {
		if val := node.Labels[NodeOwnerLabelName]; val != string(machine.UID) {
			if err := r.updateNode(ctx, node, func(n *corev1.Node) {
				n.Labels[NodeOwnerLabelName] = string(machine.UID)
			}); err != nil {
				return nil, fmt.Errorf("failed to update node %q after adding owner label: %w", node.Name, err)
			}
		}

		if node.Spec.ConfigSource == nil && machine.Spec.ConfigSource != nil {
			if err := r.updateNode(ctx, node, func(node *corev1.Node) {
				node.Spec.ConfigSource = machine.Spec.ConfigSource
			}); err != nil {
				return nil, fmt.Errorf("failed to update node %s after setting the config source: %w", node.Name, err)
			}
			klog.V(3).Infof("Added config source to node %s (machine %s)", node.Name, machine.Name)
		}
		if err := r.updateMachineStatus(machine, node); err != nil {
			return nil, fmt.Errorf("failed to update machine status: %w", err)
		}
	} else {
		// If the machine has an owner Ref and joinClusterTimeout is configured and reached, delete it to have it re-created by the MachineSet controller
		// Check if the machine is a potential candidate for triggering deletion
		if r.joinClusterTimeout != nil && ownerReferencesHasMachineSetKind(machine.OwnerReferences) {
			if time.Since(machine.CreationTimestamp.Time) > *r.joinClusterTimeout {
				klog.V(3).Infof("Join cluster timeout expired for machine %s, deleting it", machine.Name)
				if err := r.client.Delete(ctx, machine); err != nil {
					return nil, fmt.Errorf("failed to delete machine %s/%s that didn't join cluster within expected period of %s: %w",
						machine.Namespace, machine.Name, r.joinClusterTimeout.String(), err)
				}
				return nil, nil
			}
			// Re-enqueue the machine, because if it never joins the cluster nothing will trigger another sync on it once the timeout is reached
			return &reconcile.Result{RequeueAfter: 1 * time.Minute}, nil
		}
	}
	return nil, nil
}

func ownerReferencesHasMachineSetKind(ownerReferences []metav1.OwnerReference) bool {
	for _, ownerReference := range ownerReferences {
		if ownerReference.Kind == "MachineSet" {
			return true
		}
	}
	return false
}

func (r *Reconciler) ensureNodeLabelsAnnotationsAndTaints(ctx context.Context, node *corev1.Node, machine *clusterv1alpha1.Machine) error {
	var modifiers []func(*corev1.Node)

	for k, v := range machine.Spec.Labels {
		if _, exists := node.Labels[k]; !exists {
			f := func(k, v string) func(*corev1.Node) {
				return func(n *corev1.Node) {
					n.Labels[k] = v
				}
			}
			modifiers = append(modifiers, f(k, v))
		}
	}

	for k, v := range machine.Spec.Annotations {
		if _, exists := node.Annotations[k]; !exists {
			f := func(k, v string) func(*corev1.Node) {
				return func(n *corev1.Node) {
					n.Annotations[k] = v
				}
			}
			modifiers = append(modifiers, f(k, v))
		}
	}
	autoscalerAnnotationValue := fmt.Sprintf("%s/%s", machine.Namespace, machine.Name)
	if node.Annotations[AnnotationAutoscalerIdentifier] != autoscalerAnnotationValue {
		f := func(k, v string) func(*corev1.Node) {
			return func(n *corev1.Node) {
				n.Annotations[k] = v
			}
		}
		modifiers = append(modifiers, f(AnnotationAutoscalerIdentifier, autoscalerAnnotationValue))
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
			f := func(t corev1.Taint) func(*corev1.Node) {
				return func(n *corev1.Node) {
					n.Spec.Taints = append(node.Spec.Taints, t)
				}
			}
			modifiers = append(modifiers, f(t))
		}
	}

	if len(modifiers) > 0 {
		if err := r.updateNode(ctx, node, modifiers...); err != nil {
			return fmt.Errorf("failed to update node %s after setting labels/annotations/taints: %w", node.Name, err)
		}
		r.recorder.Event(machine, corev1.EventTypeNormal, "LabelsAnnotationsTaintsUpdated", "Successfully updated labels/annotations/taints")
		klog.V(3).Infof("Added labels/annotations/taints to node %s (machine %s)", node.Name, machine.Name)
	}

	return nil
}

func (r *Reconciler) updateMachineStatus(machine *clusterv1alpha1.Machine, node *corev1.Node) error {
	if node == nil {
		return nil
	}

	ref, err := reference.GetReference(scheme.Scheme, node)
	if err != nil {
		return fmt.Errorf("failed to get node reference for %s : %w", node.Name, err)
	}
	if !equality.Semantic.DeepEqual(machine.Status.NodeRef, ref) ||
		machine.Status.Versions == nil ||
		machine.Status.Versions.Kubelet != node.Status.NodeInfo.KubeletVersion {
		if err := r.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			m.Status.NodeRef = ref
			m.Status.Versions = &clusterv1alpha1.MachineVersionInfo{Kubelet: node.Status.NodeInfo.KubeletVersion}
		}); err != nil {
			return fmt.Errorf("failed to update machine after setting its status: %w", err)
		}
	}

	return nil
}

func (r *Reconciler) getNode(ctx context.Context, instance instance.Instance, provider providerconfigtypes.CloudProvider) (node *corev1.Node, exists bool, err error) {
	if instance == nil {
		return nil, false, fmt.Errorf("getNode called with nil provider instance")
	}
	nodes := &corev1.NodeList{}
	if err := r.client.List(ctx, nodes); err != nil {
		return nil, false, err
	}

	for _, node := range nodes.Items {
		// Try to find Node by providerID. Should work if CCM is deployed.
		if node := findNodeByProviderID(instance, provider, nodes.Items); node != nil {
			klog.V(4).Infof("Found node %q by providerID", node.Name)
			return node, true, nil
		}

		// If we were unable to find Node by ProviderID, fallback to IP address matching.
		// This usually happens if there's no CCM deployed in the cluster.
		//
		// This mechanism is not always reliable, as providers reuse the IP addresses after
		// some time.
		//
		// If we rollout a Machine, it can happen that a new instance has the same
		// IP addresses as the instance that has just been deleted. If machine-controller
		// processes the new Machine before removing the old Machine and the corresponding
		// Node object, machine-controller could update the NodeOwner label on the old Node
		// object to point to the Machine that just got created, as IP addresses would match.
		// This causes machine-controller to fail to delete the old Node object, which could
		// then cause cluster stability issues in some cases.
		for _, nodeAddress := range node.Status.Addresses {
			for instanceAddress := range instance.Addresses() {
				// We observed that the issue described above happens often on Hetzner.
				// As we know that the Node and the instance name will always be same
				// on Hetzner, we can use it as an additional check to prevent this
				// issue.
				// TODO: We should do this for other providers, but there are providers where
				// the node and the instance names will not match, so it requires further
				// investigation (e.g. AWS).
				if provider == providerconfigtypes.CloudProviderHetzner && node.Name != instance.Name() {
					continue
				}
				if nodeAddress.Address == instanceAddress {
					klog.V(4).Infof("Found node %q by IP address", node.Name)
					return node.DeepCopy(), true, nil
				}
			}
		}
	}
	return nil, false, nil
}

func findNodeByProviderID(instance instance.Instance, provider providerconfigtypes.CloudProvider, nodes []corev1.Node) *corev1.Node {
	providerID := instance.ProviderID()
	if providerID == "" {
		return nil
	}

	for _, node := range nodes {
		if strings.EqualFold(node.Spec.ProviderID, providerID) {
			return node.DeepCopy()
		}

		// AWS has two different providerID notations:
		//   * aws:///<availability-zone>/<instance-id>
		//   * aws:///<instance-id>
		// The first case is handled above, while the second here is handled here.
		if provider == providerconfigtypes.CloudProviderAWS {
			pid := strings.Split(node.Spec.ProviderID, "aws:///")
			if len(pid) == 2 && pid[1] == instance.ID() {
				return node.DeepCopy()
			}
		}
	}

	return nil
}

func (r *Reconciler) ReadinessChecks(ctx context.Context) map[string]healthcheck.Check {
	return map[string]healthcheck.Check{
		"valid-info-kubeconfig": func() error {
			cm, err := r.kubeconfigProvider.GetKubeconfig(ctx)
			if err != nil {
				err := fmt.Errorf("failed to get cluster-info configmap: %w", err)
				klog.V(2).Info(err)
				return err
			}
			if len(cm.Clusters) != 1 {
				err := errors.New("invalid kubeconfig: no clusters found")
				klog.V(2).Info(err)
				return err
			}
			for name, c := range cm.Clusters {
				if len(c.CertificateAuthorityData) == 0 {
					err := fmt.Errorf("invalid kubeconfig: no certificate authority data was specified for kuberconfig.clusters.['%s']", name)
					klog.V(2).Info(err)
					return err
				}
				if len(c.Server) == 0 {
					err := fmt.Errorf("invalid kubeconfig: no server was specified for kuberconfig.clusters.['%s']", name)
					klog.V(2).Info(err)
					return err
				}
			}
			return nil
		},
	}
}

func (r *Reconciler) ensureDeleteFinalizerExists(machine *clusterv1alpha1.Machine) (*clusterv1alpha1.Machine, error) {
	if !sets.NewString(machine.Finalizers...).Has(FinalizerDeleteInstance) {
		if err := r.updateMachine(machine, func(m *clusterv1alpha1.Machine) {
			finalizers := sets.NewString(m.Finalizers...)
			finalizers.Insert(FinalizerDeleteInstance)
			finalizers.Insert(FinalizerDeleteNode)
			m.Finalizers = finalizers.List()
		}); err != nil {
			return nil, fmt.Errorf("failed to update machine after adding the delete instance finalizer: %w", err)
		}
		klog.V(3).Infof("Added delete finalizer to machine %s", machine.Name)
	}
	return machine, nil
}

func (r *Reconciler) updateNode(ctx context.Context, node *corev1.Node, modifiers ...func(*corev1.Node)) error {
	// Store name here, because the object can be nil if an update failed
	name := types.NamespacedName{Name: node.Name}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.client.Get(ctx, name, node); err != nil {
			return err
		}
		for _, modify := range modifiers {
			modify(node)
		}
		return r.client.Update(ctx, node)
	})
}
