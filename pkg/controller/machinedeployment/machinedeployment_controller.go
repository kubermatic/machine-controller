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

package machinedeployment

import (
	"context"
	"reflect"

	"github.com/pkg/errors"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// controllerName is the name of this controller.
const controllerName = "machinedeployment-controller"

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = v1alpha1.SchemeGroupVersion.WithKind("MachineDeployment")
)

// ReconcileMachineDeployment reconciles a MachineDeployment object.
type ReconcileMachineDeployment struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// newReconciler returns a new reconcile.Reconciler.
func newReconciler(mgr manager.Manager) *ReconcileMachineDeployment {
	return &ReconcileMachineDeployment{Client: mgr.GetClient(), scheme: mgr.GetScheme(), recorder: mgr.GetEventRecorderFor(controllerName)}
}

// Add creates a new MachineDeployment Controller and adds it to the Manager with default RBAC.
func Add(mgr manager.Manager) error {
	r := newReconciler(mgr)
	return add(mgr, newReconciler(mgr), r.MachineSetToDeployments)
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler.
func add(mgr manager.Manager, r reconcile.Reconciler, mapFn handler.MapFunc) error {
	// Create a new controller.
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to MachineDeployment.
	err = c.Watch(&source.Kind{
		Type: &v1alpha1.MachineDeployment{}},
		&handler.EnqueueRequestForObject{},
	)
	if err != nil {
		return err
	}

	// Watch for changes to MachineSet and reconcile the owner MachineDeployment.
	err = c.Watch(
		&source.Kind{Type: &v1alpha1.MachineSet{}},
		&handler.EnqueueRequestForOwner{OwnerType: &v1alpha1.MachineDeployment{}, IsController: true},
	)
	if err != nil {
		return err
	}

	// Watch for changes to MachineSets using a mapping function to MachineDeployment.
	// This watcher is required for use cases like adoption. In case a MachineSet doesn't have
	// a controller reference, it'll look for potential matching MachineDeployments to reconcile.
	err = c.Watch(
		&source.Kind{Type: &v1alpha1.MachineSet{}},
		handler.EnqueueRequestsFromMapFunc(mapFn),
	)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a MachineDeployment object and makes changes based on the state read
// and what is in the MachineDeployment.Spec.
//
// +kubebuilder:rbac:groups=cluster.k8s.io,resources=machinedeployments;machinedeployments/status,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileMachineDeployment) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Fetch the MachineDeployment instance
	d := &v1alpha1.MachineDeployment{}
	if err := r.Get(ctx, request.NamespacedName, d); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Ignore deleted MachineDeployments, this can happen when foregroundDeletion
	// is enabled
	if d.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	result, err := r.reconcile(ctx, d)
	if err != nil {
		klog.Errorf("Failed to reconcile MachineDeployment %q: %v", request.NamespacedName, err)
		r.recorder.Eventf(d, corev1.EventTypeWarning, "ReconcileError", "%v", err)
	}

	return result, err
}

func (r *ReconcileMachineDeployment) reconcile(ctx context.Context, d *v1alpha1.MachineDeployment) (reconcile.Result, error) {
	v1alpha1.PopulateDefaultsMachineDeployment(d)

	everything := metav1.LabelSelector{}
	if reflect.DeepEqual(d.Spec.Selector, &everything) {
		if d.Status.ObservedGeneration < d.Generation {
			d.Status.ObservedGeneration = d.Generation
			if err := r.Status().Update(ctx, d); err != nil {
				klog.Warningf("Failed to update status for MachineDeployment %q: %v", d.Name, err)
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Make sure that label selector can match the template's labels.
	// TODO(vincepri): Move to a validation (admission) webhook when supported.
	selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to parse MachineDeployment %q label selector", d.Name)
	}

	if !selector.Matches(labels.Set(d.Spec.Template.Labels)) {
		return reconcile.Result{}, errors.Errorf("failed validation on MachineDeployment %q label selector, cannot match Machine template labels", d.Name)
	}

	if !contains(d.Finalizers, metav1.FinalizerDeleteDependents) {
		d.Finalizers = append(d.ObjectMeta.Finalizers, metav1.FinalizerDeleteDependents)
		if err := r.Client.Update(ctx, d); err != nil {
			klog.Infof("Failed to add finalizers to MachineSet %q: %v", d.Name, err)
			return reconcile.Result{}, err
		}

		// Since adding the finalizer updates the object return to avoid later update issues
		return reconcile.Result{Requeue: true}, nil
	}

	msList, err := r.getMachineSetsForDeployment(ctx, d)
	if err != nil {
		return reconcile.Result{}, err
	}

	machineMap, err := r.getMachineMapForDeployment(ctx, d, msList)
	if err != nil {
		return reconcile.Result{}, err
	}

	if d.DeletionTimestamp != nil {
		return reconcile.Result{}, r.sync(ctx, d, msList, machineMap)
	}

	if d.Spec.Paused {
		return reconcile.Result{}, r.sync(ctx, d, msList, machineMap)
	}

	switch d.Spec.Strategy.Type {
	case common.RollingUpdateMachineDeploymentStrategyType:
		return reconcile.Result{}, r.rolloutRolling(ctx, d, msList, machineMap)
	}

	return reconcile.Result{}, errors.Errorf("unexpected deployment strategy type: %s", d.Spec.Strategy.Type)
}

// getMachineSetsForDeployment returns a list of MachineSets associated with a MachineDeployment.
func (r *ReconcileMachineDeployment) getMachineSetsForDeployment(ctx context.Context, d *v1alpha1.MachineDeployment) ([]*v1alpha1.MachineSet, error) {
	// List all MachineSets to find those we own but that no longer match our selector.
	machineSets := &v1alpha1.MachineSetList{}
	listOptions := &client.ListOptions{Namespace: d.Namespace}
	if err := r.Client.List(ctx, machineSets, listOptions); err != nil {
		return nil, err
	}

	filtered := make([]*v1alpha1.MachineSet, 0, len(machineSets.Items))
	for idx := range machineSets.Items {
		ms := &machineSets.Items[idx]

		selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
		if err != nil {
			klog.Errorf("Skipping MachineSet %q, failed to get label selector from spec selector: %v", ms.Name, err)
			continue
		}

		// If a MachineDeployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() {
			klog.Warningf("Skipping MachineSet %q as the selector is empty", ms.Name)
			continue
		}

		if !selector.Matches(labels.Set(ms.Labels)) {
			klog.V(4).Infof("Skipping MachineSet %v, label mismatch", ms.Name)
			continue
		}

		// Attempt to adopt machine if it meets previous conditions and it has no controller references.
		if metav1.GetControllerOf(ms) == nil {
			if err := r.adoptOrphan(ctx, d, ms); err != nil {
				klog.Warningf("Failed to adopt MachineSet %q into MachineDeployment %q: %v", ms.Name, d.Name, err)
				continue
			}
		}

		if !metav1.IsControlledBy(ms, d) {
			continue
		}

		filtered = append(filtered, ms)
	}

	return filtered, nil
}

// adoptOrphan sets the MachineDeployment as a controller OwnerReference to the MachineSet.
func (r *ReconcileMachineDeployment) adoptOrphan(ctx context.Context, deployment *v1alpha1.MachineDeployment, machineSet *v1alpha1.MachineSet) error {
	newRef := *metav1.NewControllerRef(deployment, controllerKind)
	machineSet.OwnerReferences = append(machineSet.OwnerReferences, newRef)
	return r.Client.Update(ctx, machineSet)
}

// getMachineMapForDeployment returns the Machines managed by a Deployment.
//
// It returns a map from MachineSet UID to a list of Machines controlled by that MachineSet,
// according to the Machine's ControllerRef.
func (r *ReconcileMachineDeployment) getMachineMapForDeployment(ctx context.Context, d *v1alpha1.MachineDeployment, msList []*v1alpha1.MachineSet) (map[types.UID]*v1alpha1.MachineList, error) {
	// TODO(droot): double check if previous selector maps correctly to new one.
	// _, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)

	// Get all Machines that potentially belong to this Deployment.
	selector, err := metav1.LabelSelectorAsMap(&d.Spec.Selector)
	if err != nil {
		return nil, err
	}

	machines := &v1alpha1.MachineList{}
	listOptions := &client.ListOptions{Namespace: d.Namespace}
	if err = r.Client.List(ctx, machines, listOptions, client.MatchingLabels(selector)); err != nil {
		return nil, err
	}

	// Group Machines by their controller (if it's in msList).
	machineMap := make(map[types.UID]*v1alpha1.MachineList, len(msList))
	for _, ms := range msList {
		machineMap[ms.UID] = &v1alpha1.MachineList{}
	}

	for idx := range machines.Items {
		machine := &machines.Items[idx]

		// Do not ignore inactive Machines because Recreate Deployments need to verify that no
		// Machines from older versions are running before spinning up new Machines.
		controllerRef := metav1.GetControllerOf(machine)
		if controllerRef == nil {
			continue
		}

		// Only append if we care about this UID.
		if machineList, ok := machineMap[controllerRef.UID]; ok {
			machineList.Items = append(machineList.Items, *machine)
		}
	}

	return machineMap, nil
}

// getMachineDeploymentsForMachineSet returns a list of MachineDeployments that could potentially match a MachineSet.
func (r *ReconcileMachineDeployment) getMachineDeploymentsForMachineSet(ctx context.Context, ms *v1alpha1.MachineSet) []*v1alpha1.MachineDeployment {
	if len(ms.Labels) == 0 {
		klog.Warningf("No machine deployments found for MachineSet %q because it has no labels", ms.Name)
		return nil
	}

	dList := &v1alpha1.MachineDeploymentList{}
	listOptions := &client.ListOptions{Namespace: ms.Namespace}
	if err := r.Client.List(ctx, dList, listOptions); err != nil {
		klog.Warningf("Failed to list machine deployments: %v", err)
		return nil
	}

	deployments := make([]*v1alpha1.MachineDeployment, 0, len(dList.Items))
	for idx, d := range dList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
		if err != nil {
			continue
		}

		// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(ms.Labels)) {
			continue
		}

		deployments = append(deployments, &dList.Items[idx])
	}

	return deployments
}

// MachineSetTodeployments is a handler.MapFunc to be used to enqeue requests for reconciliation
// for MachineDeployments that might adopt an orphaned MachineSet.
func (r *ReconcileMachineDeployment) MachineSetToDeployments(o client.Object) []reconcile.Request {
	result := []reconcile.Request{}
	ctx := context.Background()

	ms := &v1alpha1.MachineSet{}
	key := client.ObjectKey{Namespace: o.GetNamespace(), Name: o.GetName()}
	if err := r.Client.Get(ctx, key, ms); err != nil {
		if !apierrors.IsNotFound(err) {
			klog.Errorf("Unable to retrieve MachineSet %q for possible MachineDeployment adoption: %v", key, err)
		}
		return nil
	}

	// Check if the controller reference is already set and
	// return an empty result when one is found.
	for _, ref := range ms.ObjectMeta.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return result
		}
	}

	mds := r.getMachineDeploymentsForMachineSet(ctx, ms)
	if len(mds) == 0 {
		klog.V(4).Infof("Found no machine set for machine: %v", ms.Name)
		return nil
	}

	for _, md := range mds {
		name := client.ObjectKey{Namespace: md.Namespace, Name: md.Name}
		result = append(result, reconcile.Request{NamespacedName: name})
	}

	return result
}

func contains(list []string, strToSearch string) bool {
	for _, item := range list {
		if item == strToSearch {
			return true
		}
	}
	return false
}
