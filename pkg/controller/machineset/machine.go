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

	"go.uber.org/zap"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *ReconcileMachineSet) getMachineSetsForMachine(ctx context.Context, machineLog *zap.SugaredLogger, m *v1alpha1.Machine) []*v1alpha1.MachineSet {
	if len(m.Labels) == 0 {
		machineLog.Infow("No MachineSets found for Machine because it has no labels")
		return nil
	}

	msList := &v1alpha1.MachineSetList{}
	listOptions := &client.ListOptions{
		Namespace: m.Namespace,
	}

	err := c.Client.List(ctx, msList, listOptions)
	if err != nil {
		machineLog.Errorw("Failed to list MachineSets", zap.Error(err))
		return nil
	}

	var mss []*v1alpha1.MachineSet
	for idx := range msList.Items {
		ms := &msList.Items[idx]
		if hasMatchingLabels(machineLog, ms, m) {
			mss = append(mss, ms)
		}
	}

	return mss
}

func hasMatchingLabels(machineLog *zap.SugaredLogger, machineSet *v1alpha1.MachineSet, machine *v1alpha1.Machine) bool {
	selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		machineLog.Errorw("Failed to convert selector", zap.Error(err))
		return false
	}

	// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
	if selector.Empty() {
		machineLog.Info("MachineSet has empty selector")
		return false
	}

	if !selector.Matches(labels.Set(machine.Labels)) {
		machineLog.Debug("Machine has mismatch labels")
		return false
	}

	return true
}
