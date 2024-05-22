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

package machineset

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// controllerName is the name of this controller.
const controllerName = "machineset-controller"

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = clusterv1alpha1.SchemeGroupVersion.WithKind("MachineSet")

	// stateConfirmationTimeout is the amount of time allowed to wait for desired state.
	stateConfirmationTimeout = 10 * time.Second

	// stateConfirmationInterval is the amount of time between polling for the desired state.
	// The polling is against a local memory cache.
	stateConfirmationInterval = 100 * time.Millisecond
)

// Add creates a new MachineSet Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, log *zap.SugaredLogger) error {
	r := newReconciler(mgr, log)
	return add(mgr, r, r.MachineToMachineSets())
}

// newReconciler returns a new reconcile.Reconciler.
func newReconciler(mgr manager.Manager, log *zap.SugaredLogger) *ReconcileMachineSet {
	return &ReconcileMachineSet{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		log:      log.Named(controllerName),
		recorder: mgr.GetEventRecorderFor(controllerName),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler.
func add(mgr manager.Manager, r reconcile.Reconciler, mapFn handler.MapFunc) error {
	_, err := builder.ControllerManagedBy(mgr).
		Named(controllerName).
		WithOptions(controller.Options{
			LogConstructor: func(*reconcile.Request) logr.Logger {
				// we log ourselves
				return zapr.NewLogger(zap.NewNop())
			},
		}).
		// Watch for changes to MachineSet.
		For(&clusterv1alpha1.MachineSet{}).
		// Watch for changes to Machines and reconcile the owner MachineSet.
		Owns(&clusterv1alpha1.Machine{}).
		// Watch for changes to Machines using a mapping function to MachineSets.
		// This watcher is required for use cases like adoption. In case a Machine doesn't have
		// a controller reference, it'll look for potential matching MachineSet to reconcile.
		Watches(&clusterv1alpha1.Machine{}, handler.EnqueueRequestsFromMapFunc(mapFn)).
		Build(r)

	return err
}

// ReconcileMachineSet reconciles a MachineSet object.
type ReconcileMachineSet struct {
	client.Client
	log      *zap.SugaredLogger
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a MachineSet object and makes changes based on the state read
// and what is in the MachineSet.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=cluster.k8s.io,resources=machinesets;machinesets/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.k8s.io,resources=machines,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileMachineSet) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := r.log.With("machineset", request.NamespacedName)
	log.Debug("Reconciling")

	// Fetch the MachineSet instance
	machineSet := &clusterv1alpha1.MachineSet{}
	if err := r.Get(ctx, request.NamespacedName, machineSet); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Errorw("Failed to get MachineSet", zap.Error(err))
		return reconcile.Result{}, err
	}

	// Ignore deleted MachineSets, this can happen when foregroundDeletion
	// is enabled
	if machineSet.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	result, err := r.reconcile(ctx, log, machineSet)
	if err != nil {
		log.Errorw("Reconciling failed", zap.Error(err))
		r.recorder.Eventf(machineSet, corev1.EventTypeWarning, "ReconcileError", "%v", err)
	}
	return result, err
}

func (r *ReconcileMachineSet) reconcile(ctx context.Context, log *zap.SugaredLogger, machineSet *clusterv1alpha1.MachineSet) (reconcile.Result, error) {
	log.Debug("Reconcile MachineSet")
	allMachines := &clusterv1alpha1.MachineList{}

	if err := r.Client.List(ctx, allMachines, client.InNamespace(machineSet.Namespace)); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to list machines")
	}

	// Make sure that label selector can match template's labels.
	// TODO(vincepri): Move to a validation (admission) webhook when supported.
	selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to parse MachineSet %q label selector", machineSet.Name)
	}

	if !selector.Matches(labels.Set(machineSet.Spec.Template.Labels)) {
		return reconcile.Result{}, errors.Errorf("failed validation on MachineSet %q label selector, cannot match any machines ", machineSet.Name)
	}

	// Add foregroundDeletion finalizer
	if !contains(machineSet.Finalizers, metav1.FinalizerDeleteDependents) {
		machineSet.Finalizers = append(machineSet.ObjectMeta.Finalizers, metav1.FinalizerDeleteDependents)

		if err := r.Client.Update(ctx, machineSet); err != nil {
			return reconcile.Result{}, err
		}

		// Since adding the finalizer updates the object return to avoid later update issues
		return reconcile.Result{Requeue: true}, nil
	}

	// Return early if the MachineSet is deleted.
	if !machineSet.ObjectMeta.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	// Filter out irrelevant machines (deleting/mismatch labels) and claim orphaned machines.
	filteredMachines := make([]*clusterv1alpha1.Machine, 0, len(allMachines.Items))
	for idx := range allMachines.Items {
		machine := &allMachines.Items[idx]
		machineLog := log.With("machine", client.ObjectKeyFromObject(machine))

		if shouldExcludeMachine(machineLog, machineSet, machine) {
			continue
		}

		// Attempt to adopt machine if it meets previous conditions and it has no controller references.
		if metav1.GetControllerOf(machine) == nil {
			if err := r.adoptOrphan(ctx, machineSet, machine); err != nil {
				machineLog.Errorw("Failed to adopt Machine into MachineSet", zap.Error(err))
				continue
			}
		}

		filteredMachines = append(filteredMachines, machine)
	}

	syncErr := r.syncReplicas(ctx, log, machineSet, filteredMachines)

	ms := machineSet.DeepCopy()
	newStatus := r.calculateStatus(ctx, log, ms, filteredMachines)

	// Always updates status as machines come up or die.
	updatedMS, err := updateMachineSetStatus(ctx, log, r.Client, machineSet, newStatus)
	if err != nil {
		if syncErr != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to sync machines: %v. failed to update machine set status", syncErr)
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to update machine set status")
	}

	if syncErr != nil {
		return reconcile.Result{}, errors.Wrapf(syncErr, "failed to sync Machineset replicas")
	}

	var replicas int32
	if updatedMS.Spec.Replicas != nil {
		replicas = *updatedMS.Spec.Replicas
	}

	// Resync the MachineSet after MinReadySeconds as a last line of defense to guard against clock-skew.
	// Clock-skew is an issue as it may impact whether an available replica is counted as a ready replica.
	// A replica is available if the amount of time since last transition exceeds MinReadySeconds.
	// If there was a clock skew, checking whether the amount of time since last transition to ready state
	// exceeds MinReadySeconds could be incorrect.
	// To avoid an available replica stuck in the ready state, we force a reconcile after MinReadySeconds,
	// at which point it should confirm any available replica to be available.
	if updatedMS.Spec.MinReadySeconds > 0 &&
		updatedMS.Status.ReadyReplicas == replicas &&
		updatedMS.Status.AvailableReplicas != replicas {
		return reconcile.Result{RequeueAfter: time.Duration(updatedMS.Spec.MinReadySeconds) * time.Second}, nil
	}
	return reconcile.Result{}, nil
}

// syncReplicas scales Machine resources up or down.
func (r *ReconcileMachineSet) syncReplicas(ctx context.Context, log *zap.SugaredLogger, ms *clusterv1alpha1.MachineSet, machines []*clusterv1alpha1.Machine) error {
	if ms.Spec.Replicas == nil {
		return errors.Errorf("the Replicas field in Spec for machineset %v is nil, this should not be allowed", ms.Name)
	}

	diff := len(machines) - int(*(ms.Spec.Replicas))
	replicasLog := log.With("spec", *(ms.Spec.Replicas), "current", len(machines))

	if diff < 0 {
		diff *= -1
		replicasLog.Infow("Too few replicas, creating more", "diff", diff)

		var machineList []*clusterv1alpha1.Machine
		var errstrings []string
		for i := 0; i < diff; i++ {
			replicasLog.Infow("Creating new machine", "index", i+1)

			machine := r.createMachine(ms)
			if err := r.Client.Create(ctx, machine); err != nil {
				log.Errorw("Failed to create Machine", "machine", client.ObjectKeyFromObject(machine), zap.Error(err))
				errstrings = append(errstrings, err.Error())
				continue
			}

			machineList = append(machineList, machine)
		}

		if len(errstrings) > 0 {
			return errors.New(strings.Join(errstrings, "; "))
		}

		return r.waitForMachineCreation(ctx, log, machineList)
	} else if diff > 0 {
		replicasLog.Infow("Too many replicas, deleting extras", "diff", diff, "deletepolicy", ms.Spec.DeletePolicy)

		deletePriorityFunc, err := getDeletePriorityFunc(ms)
		if err != nil {
			return err
		}

		// Choose which Machines to delete.
		machinesToDelete := getMachinesToDeletePrioritized(machines, diff, deletePriorityFunc)

		// TODO: Add cap to limit concurrent delete calls.
		errCh := make(chan error, diff)
		var wg sync.WaitGroup
		wg.Add(diff)
		for _, machine := range machinesToDelete {
			go func(targetMachine *clusterv1alpha1.Machine) {
				defer wg.Done()
				err := r.Client.Delete(ctx, targetMachine)
				if err != nil {
					log.Errorw("Failed to delete Machine", "machine", client.ObjectKeyFromObject(targetMachine), zap.Error(err))
					errCh <- err
				}
			}(machine)
		}
		wg.Wait()

		select {
		case err := <-errCh:
			// all errors have been reported before and they're likely to be the same, so we'll only return the first one we hit.
			if err != nil {
				return err
			}
		default:
		}

		return r.waitForMachineDeletion(ctx, machinesToDelete)
	}

	return nil
}

// createMachine creates a Machine resource. The name of the newly created resource is going
// to be created by the API server, we set the generateName field.
func (r *ReconcileMachineSet) createMachine(machineSet *clusterv1alpha1.MachineSet) *clusterv1alpha1.Machine {
	gv := clusterv1alpha1.SchemeGroupVersion
	machine := &clusterv1alpha1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       gv.WithKind("Machine").Kind,
			APIVersion: gv.String(),
		},
		ObjectMeta: machineSet.Spec.Template.ObjectMeta,
		Spec:       machineSet.Spec.Template.Spec,
	}
	machine.ObjectMeta.GenerateName = fmt.Sprintf("%s-", machineSet.Name)
	machine.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(machineSet, controllerKind)}
	machine.Namespace = machineSet.Namespace
	return machine
}

// shouldExcludeMachine returns true if the machine should be filtered out, false otherwise.
func shouldExcludeMachine(machineLog *zap.SugaredLogger, machineSet *clusterv1alpha1.MachineSet, machine *clusterv1alpha1.Machine) bool {
	// Ignore inactive machines.
	if metav1.GetControllerOf(machine) != nil && !metav1.IsControlledBy(machine, machineSet) {
		machineLog.Debug("Machine not controlled by MachineSet")
		return true
	}

	if machine.ObjectMeta.DeletionTimestamp != nil {
		return true
	}

	if !hasMatchingLabels(machineLog, machineSet, machine) {
		return true
	}

	return false
}

// adoptOrphan sets the MachineSet as a controller OwnerReference to the Machine.
func (r *ReconcileMachineSet) adoptOrphan(ctx context.Context, machineSet *clusterv1alpha1.MachineSet, machine *clusterv1alpha1.Machine) error {
	newRef := *metav1.NewControllerRef(machineSet, controllerKind)
	machine.OwnerReferences = append(machine.OwnerReferences, newRef)
	return r.Client.Update(ctx, machine)
}

func (r *ReconcileMachineSet) waitForMachineCreation(ctx context.Context, log *zap.SugaredLogger, machineList []*clusterv1alpha1.Machine) error {
	for _, machine := range machineList {
		pollErr := wait.PollUntilContextTimeout(ctx, stateConfirmationInterval, stateConfirmationTimeout, false, func(ctx context.Context) (bool, error) {
			key := client.ObjectKey{Namespace: machine.Namespace, Name: machine.Name}

			if err := r.Client.Get(ctx, key, &clusterv1alpha1.Machine{}); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				log.Error(err)
				return false, err
			}

			return true, nil
		})

		if pollErr != nil {
			return errors.Wrap(pollErr, "failed waiting for machine object to be created")
		}
	}

	return nil
}

func (r *ReconcileMachineSet) waitForMachineDeletion(ctx context.Context, machineList []*clusterv1alpha1.Machine) error {
	for _, machine := range machineList {
		pollErr := wait.PollUntilContextTimeout(ctx, stateConfirmationInterval, stateConfirmationTimeout, false, func(ctx context.Context) (bool, error) {
			m := &clusterv1alpha1.Machine{}
			key := client.ObjectKey{Namespace: machine.Namespace, Name: machine.Name}

			err := r.Client.Get(ctx, key, m)
			if apierrors.IsNotFound(err) || !m.DeletionTimestamp.IsZero() {
				return true, nil
			}

			return false, err
		})

		if pollErr != nil {
			return errors.Wrap(pollErr, "failed waiting for machine object to be deleted")
		}
	}
	return nil
}

// MachineToMachineSets is a handler.ToRequestsFunc to be used to enqeue requests for reconciliation
// for MachineSets that might adopt an orphaned Machine.
func (r *ReconcileMachineSet) MachineToMachineSets() handler.MapFunc {
	return func(ctx context.Context, o client.Object) []ctrlruntime.Request {
		result := []reconcile.Request{}

		m := &clusterv1alpha1.Machine{}
		key := client.ObjectKey{Namespace: o.GetNamespace(), Name: o.GetName()}
		machineLog := r.log.With("machine", key)

		if err := r.Client.Get(ctx, key, m); err != nil {
			if !apierrors.IsNotFound(err) {
				machineLog.Errorw("Failed to retrieve Machine for possible MachineSet adoption", zap.Error(err))
			}
			return nil
		}

		// Check if the controller reference is already set and
		// return an empty result when one is found.
		for _, ref := range m.ObjectMeta.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				return result
			}
		}

		mss := r.getMachineSetsForMachine(ctx, machineLog, m)
		if len(mss) == 0 {
			machineLog.Debug("Found no MachineSet for Machine")
			return nil
		}

		for _, ms := range mss {
			name := client.ObjectKey{Namespace: ms.Namespace, Name: ms.Name}
			result = append(result, reconcile.Request{NamespacedName: name})
		}

		return result
	}
}

func contains(list []string, strToSearch string) bool {
	for _, item := range list {
		if item == strToSearch {
			return true
		}
	}
	return false
}
