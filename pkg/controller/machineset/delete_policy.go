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
	"math"
	"sort"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type (
	deletePriority     float64
	deletePriorityFunc func(machine *v1alpha1.Machine) deletePriority
)

const (

	// DeleteNodeAnnotation marks nodes that will be given priority for deletion
	// when a machineset scales down. This annotation is given top priority on all delete policies.
	DeleteNodeAnnotation = "cluster.k8s.io/delete-machine"

	mustDelete    deletePriority = 100.0
	betterDelete  deletePriority = 50.0
	couldDelete   deletePriority = 20.0
	mustNotDelete deletePriority = 0.0

	secondsPerTenDays float64 = 864000
)

// maps the creation timestamp onto the 0-100 priority range.
func oldestDeletePriority(machine *v1alpha1.Machine) deletePriority {
	// DeletionTimestamp is RFC 3339 date and time at which this resource will be deleted. This
	// field is set by the server when a graceful deletion is requested by the user, and is not
	// directly settable by a client.
	// If machine deletion was already set, machine OK to delete
	if machine.DeletionTimestamp != nil && !machine.DeletionTimestamp.IsZero() {
		return mustDelete
	}
	// If machine annotations is not empty and DeleteNodeAnnotation is set, machine OK to delete
	if machine.ObjectMeta.Annotations != nil && machine.ObjectMeta.Annotations[DeleteNodeAnnotation] != "" {
		return mustDelete
	}
	// If there are machine errors, delete the machine
	if machine.Status.ErrorReason != nil || machine.Status.ErrorMessage != nil {
		return mustDelete
	}
	// If machine is new, don't delete it "CreationTimestamp is a timestamp representing the server time when this object was created"
	if machine.ObjectMeta.CreationTimestamp.Time.IsZero() {
		return mustNotDelete
	}
	d := metav1.Now().Sub(machine.ObjectMeta.CreationTimestamp.Time)
	if d.Seconds() < 0 {
		return mustNotDelete
	}
	return deletePriority(float64(mustDelete) * (1.0 - math.Exp(-d.Seconds()/secondsPerTenDays)))
}


func customDeletePolicy(machine *v1alpha1.Machine) deletePriority {
	// Iterate through conditions to get if machine is not "Ready"
	// Check types.go
	for _, c := range machine.Status.Conditions {
		if c.Type != corev1.NodeReady {
			return mustDelete
		}
	}
	// Rest of conditions are extracted from the "Newest" policy, that is the one that
	// matches the idea of deleting the newest machines (more possibilities to be WIP in the attachment to cluster)
	// than the old ones.
	if machine.DeletionTimestamp != nil && !machine.DeletionTimestamp.IsZero() {
		return mustDelete
	}
	// If annotations are set + DeleteAnnotation is present
	if machine.ObjectMeta.Annotations != nil && machine.ObjectMeta.Annotations[DeleteNodeAnnotation] != "" {
		return mustDelete
	}
	// If there are errors on machine, delete it
	if machine.Status.ErrorReason != nil || machine.Status.ErrorMessage != nil {
		return mustDelete
	}
	// Thinking about this, maybe it's a good idea to delegate in oldestDeletePriority, like
	// newestDeletePriority is doing if no match is done, or just return couldDelete
	return mustDelete - oldestDeletePriority(machine)
}

func newestDeletePriority(machine *v1alpha1.Machine) deletePriority {
	if machine.DeletionTimestamp != nil && !machine.DeletionTimestamp.IsZero() {
		return mustDelete
	}
	if machine.ObjectMeta.Annotations != nil && machine.ObjectMeta.Annotations[DeleteNodeAnnotation] != "" {
		return mustDelete
	}
	if machine.Status.ErrorReason != nil || machine.Status.ErrorMessage != nil {
		return mustDelete
	}
	return mustDelete - oldestDeletePriority(machine)
}

func randomDeletePolicy(machine *v1alpha1.Machine) deletePriority {
	if machine.DeletionTimestamp != nil && !machine.DeletionTimestamp.IsZero() {
		return mustDelete
	}
	if machine.ObjectMeta.Annotations != nil && machine.ObjectMeta.Annotations[DeleteNodeAnnotation] != "" {
		return betterDelete
	}
	if machine.Status.ErrorReason != nil || machine.Status.ErrorMessage != nil {
		return betterDelete
	}
	return couldDelete
}

type sortableMachines struct {
	machines []*v1alpha1.Machine
	priority deletePriorityFunc
}

func (m sortableMachines) Len() int { return len(m.machines) }

func (m sortableMachines) Swap(i, j int) { m.machines[i], m.machines[j] = m.machines[j], m.machines[i] }

func (m sortableMachines) Less(i, j int) bool {
	return m.priority(m.machines[j]) < m.priority(m.machines[i]) // high to low
}

func getMachinesToDeletePrioritized(filteredMachines []*v1alpha1.Machine, diff int, fun deletePriorityFunc) []*v1alpha1.Machine {
	if diff >= len(filteredMachines) {
		return filteredMachines
	} else if diff <= 0 {
		return []*v1alpha1.Machine{}
	}

	sortable := sortableMachines{
		machines: filteredMachines,
		priority: fun,
	}
	sort.Sort(sortable)

	return sortable.machines[:diff]
}

func getDeletePriorityFunc(ms *v1alpha1.MachineSet) (deletePriorityFunc, error) {
	// Map the Spec.DeletePolicy value to the appropriate delete priority function
	// Defaults to customDeletePolicy if not specified
	switch msdp := v1alpha1.MachineSetDeletePolicy(ms.Spec.DeletePolicy); msdp {
	case v1alpha1.RandomMachineSetDeletePolicy:
		return randomDeletePolicy, nil
	case v1alpha1.NewestMachineSetDeletePolicy:
		return newestDeletePriority, nil
	case v1alpha1.OldestMachineSetDeletePolicy:
		return oldestDeletePriority, nil
	case v1alpha1.CustomMachineSetDeletePolicy:
		return customDeletePolicy, nil
	case "":
		return customDeletePolicy, nil
	default:
		return nil, errors.Errorf("Unsupported delete policy %q. Must be one of 'Random', 'Newest', or 'Oldest'", msdp)
	}
}
