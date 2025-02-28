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

package util

import (
	"fmt"

	cloudprovidertypes "k8c.io/machine-controller/pkg/cloudprovider/types"
	kuberneteshelper "k8c.io/machine-controller/pkg/kubernetes"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
)

// RemoveFinalizerOnInstanceNotFound checks whether a finalizer exists and removes it on demand.
func RemoveFinalizerOnInstanceNotFound(finalizer string,
	machine *clusterv1alpha1.Machine,
	provider *cloudprovidertypes.ProviderData) (bool, error) {
	if !kuberneteshelper.HasFinalizer(machine, finalizer) {
		return true, nil
	}

	if err := provider.Update(machine, func(updatedMachine *clusterv1alpha1.Machine) {
		updatedMachine.Finalizers = kuberneteshelper.RemoveFinalizer(updatedMachine.Finalizers, finalizer)
	}); err != nil {
		return false, fmt.Errorf("failed updating machine %v finzaliers: %w", machine.Name, err)
	}
	return true, nil
}
