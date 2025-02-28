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
	"k8c.io/machine-controller/pkg/cloudprovider/types"
	kuberneteshelper "k8c.io/machine-controller/pkg/kubernetes"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
)

const (
	RedhatSubscriptionFinalizer = "kubermatic.io/red-hat-subscription"
)

// AddRHELSubscriptionFinalizer adds finalizer RedhatSubscriptionFinalizer to the machine object on rhel machine creation.
func AddRHELSubscriptionFinalizer(machine *clusterv1alpha1.Machine, update types.MachineUpdater) error {
	if !kuberneteshelper.HasFinalizer(machine, RedhatSubscriptionFinalizer) {
		if err := update(machine, func(m *clusterv1alpha1.Machine) {
			m.Finalizers = append(m.Finalizers, RedhatSubscriptionFinalizer)
		}); err != nil {
			return err
		}
	}

	return nil
}

// RemoveRHELSubscriptionFinalizer removes finalizer RedhatSubscriptionFinalizer to the machine object on rhel machine deletion.
func RemoveRHELSubscriptionFinalizer(machine *clusterv1alpha1.Machine, update types.MachineUpdater) error {
	if kuberneteshelper.HasFinalizer(machine, RedhatSubscriptionFinalizer) {
		if err := update(machine, func(m *clusterv1alpha1.Machine) {
			m.Finalizers = kuberneteshelper.RemoveFinalizer(m.Finalizers, RedhatSubscriptionFinalizer)
		}); err != nil {
			return err
		}
	}

	return nil
}
