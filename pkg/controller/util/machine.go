/*
Copyright 2022 The Machine Controller Authors.

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
	"context"
	"fmt"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetMachineDeploymentNameForMachine(ctx context.Context, machine *clusterv1alpha1.Machine, c client.Client) (string, error) {
	var (
		machineSetName        string
		machineDeploymentName string
	)
	for _, ownerRef := range machine.OwnerReferences {
		if ownerRef.Kind == "MachineSet" {
			machineSetName = ownerRef.Name
		}
	}

	if machineSetName != "" {
		machineSet := &clusterv1alpha1.MachineSet{}
		if err := c.Get(ctx, types.NamespacedName{Name: machineSetName, Namespace: "kube-system"}, machineSet); err != nil {
			return "", err
		}

		for _, ownerRef := range machineSet.OwnerReferences {
			if ownerRef.Kind == "MachineDeployment" {
				machineDeploymentName = ownerRef.Name
			}
		}

		if machineDeploymentName != "" {
			return machineDeploymentName, nil
		}
	}

	return "", fmt.Errorf("failed to find machine deployment reference for the machine %s", machine.Name)
}
