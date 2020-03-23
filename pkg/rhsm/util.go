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

package rhsm

import (
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	kuberneteshelper "github.com/kubermatic/machine-controller/pkg/kubernetes"
)

const (
	redhatSubscriptionFinalizer = "kubermatic.io/red-hat-subscription"
)

// AddRHELSubscriptionFinalizer adds finalizer redhatSubscriptionFinalizer to the machine object on rhel machine creation.
func AddRHELSubscriptionFinalizer(machine *v1alpha1.Machine, update types.MachineUpdater) error {
	if !kuberneteshelper.HasFinalizer(machine, redhatSubscriptionFinalizer) {
		if err := update(machine, func(m *v1alpha1.Machine) {
			machine.Finalizers = append(m.Finalizers, redhatSubscriptionFinalizer)
		}); err != nil {
			return err
		}
	}

	return nil
}

// RemoveRHELSubscriptionFinalizer removes finalizer redhatSubscriptionFinalizer to the machine object on rhel machine deletion.
func RemoveRHELSubscriptionFinalizer(machine *v1alpha1.Machine, update types.MachineUpdater) error {
	if kuberneteshelper.HasFinalizer(machine, redhatSubscriptionFinalizer) {
		if err := update(machine, func(m *v1alpha1.Machine) {
			machine.Finalizers = kuberneteshelper.RemoveFinalizer(machine.Finalizers, redhatSubscriptionFinalizer)
		}); err != nil {
			return err
		}
	}

	return nil
}
