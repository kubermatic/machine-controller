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

package conversions

import (
	"encoding/json"
	"fmt"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type machineDeploymentWithProviderSpecAndProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   machineDeploymentSpecWithProviderSpecAndProviderConfig `json:"spec,omitempty"`
	Status clusterv1alpha1.MachineDeploymentStatus                `json:"status,omitempty"`
}

type machineDeploymentSpecWithProviderSpecAndProviderConfig struct {
	Replicas                *int32                                               `json:"replicas,omitempty"`
	Selector                metav1.LabelSelector                                 `json:"selector"`
	Template                machineTemplateSpecWithProviderSpecAndProviderConfig `json:"template"`
	Strategy                *clusterv1alpha1.MachineDeploymentStrategy           `json:"strategy,omitempty"`
	MinReadySeconds         *int32                                               `json:"minReadySeconds,omitempty"`
	RevisionHistoryLimit    *int32                                               `json:"revisionHistoryLimit,omitempty"`
	Paused                  bool                                                 `json:"paused,omitempty"`
	ProgressDeadlineSeconds *int32                                               `json:"progressDeadlineSeconds,omitempty"`
}

type machineSetWithProviderSpecAndProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   machineSetSpecWithProviderSpecAndProviderConfig `json:"spec,omitempty"`
	Status clusterv1alpha1.MachineSetStatus                `json:"status,omitempty"`
}

type machineSetSpecWithProviderSpecAndProviderConfig struct {
	Replicas        *int32                                               `json:"replicas,omitempty"`
	MinReadySeconds int32                                                `json:"minReadySeconds,omitempty"`
	Selector        metav1.LabelSelector                                 `json:"selector"`
	Template        machineTemplateSpecWithProviderSpecAndProviderConfig `json:"template,omitempty"`
}

type machineTemplateSpecWithProviderSpecAndProviderConfig struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              machineSpecWithProviderSpecAndProviderConfig `json:"spec,omitempty"`
}

type machineWithProviderSpecAndProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   machineSpecWithProviderSpecAndProviderConfig `json:"spec,omitempty"`
	Status clusterv1alpha1.MachineStatus                `json:"status,omitempty"`
}

type machineSpecWithProviderSpecAndProviderConfig struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Taints            []corev1.Taint                     `json:"taints,omitempty"`
	ProviderConfig    json.RawMessage                    `json:"providerConfig"`
	ProviderSpec      json.RawMessage                    `json:"providerSpec"`
	Versions          clusterv1alpha1.MachineVersionInfo `json:"versions,omitempty"`
	ConfigSource      *corev1.NodeConfigSource           `json:"configSource,omitempty"`
}

func Convert_MachineDeployment_ProviderConfig_To_ProviderSpec(in []byte) (*clusterv1alpha1.MachineDeployment, bool, error) {
	var wasConverted bool
	superMachineDeployment := &machineDeploymentWithProviderSpecAndProviderConfig{}
	if err := json.Unmarshal(in, superMachineDeployment); err != nil {
		return nil, wasConverted, fmt.Errorf("error unmarshalling machineDeployment object: %w", err)
	}
	if superMachineDeployment.Spec.Template.Spec.ProviderConfig != nil && superMachineDeployment.Spec.Template.Spec.ProviderSpec != nil {
		return nil, wasConverted, fmt.Errorf("both .spec.template.spec.providerConfig and .spec.template.spec.providerSpec were non-nil for machineDeployment %s", superMachineDeployment.Name)
	}
	if superMachineDeployment.Spec.Template.Spec.ProviderConfig != nil {
		superMachineDeployment.Spec.Template.Spec.ProviderSpec = superMachineDeployment.Spec.Template.Spec.ProviderConfig
		superMachineDeployment.Spec.Template.Spec.ProviderConfig = nil
		wasConverted = true
	}

	machineDeployment := &clusterv1alpha1.MachineDeployment{}
	superMachineDeploymentBytes, err := json.Marshal(superMachineDeployment)
	if err != nil {
		return nil, wasConverted, fmt.Errorf("failed to marshal superMachineDeployment object for machineDeployment %s: %w", superMachineDeployment.Name, err)
	}
	if err := json.Unmarshal(superMachineDeploymentBytes, machineDeployment); err != nil {
		return nil, wasConverted, fmt.Errorf("failed to unmarshal superMachineDeployment object for machineDeployment %s back into machineDeployment object: %w", superMachineDeployment.Name, err)
	}
	return machineDeployment, wasConverted, nil
}

func Convert_MachineSet_ProviderConfig_To_ProviderSpec(in []byte) (*clusterv1alpha1.MachineSet, bool, error) {
	var wasConverted bool
	superMachineSet := &machineSetWithProviderSpecAndProviderConfig{}
	if err := json.Unmarshal(in, superMachineSet); err != nil {
		return nil, wasConverted, fmt.Errorf("error unmarshalling machineSet object: %w", err)
	}
	if superMachineSet.Spec.Template.Spec.ProviderConfig != nil && superMachineSet.Spec.Template.Spec.ProviderSpec != nil {
		return nil, wasConverted, fmt.Errorf("both .spec.template.spec.providerConfig and .spec.template.spec.providerSpec were non-nil for machineSet %s", superMachineSet.Name)
	}
	if superMachineSet.Spec.Template.Spec.ProviderConfig != nil {
		superMachineSet.Spec.Template.Spec.ProviderSpec = superMachineSet.Spec.Template.Spec.ProviderConfig
		superMachineSet.Spec.Template.Spec.ProviderConfig = nil
		wasConverted = true
	}

	machineSet := &clusterv1alpha1.MachineSet{}
	superMachineSetBytes, err := json.Marshal(superMachineSet)
	if err != nil {
		return nil, wasConverted, fmt.Errorf("failed to marshal superMachineSet object for machineSet %s: %w", superMachineSet.Name, err)
	}
	if err := json.Unmarshal(superMachineSetBytes, machineSet); err != nil {
		return nil, wasConverted, fmt.Errorf("failed to unmarshal superMachineSet object for machineSet %s back into machineSet object: %w", superMachineSet.Name, err)
	}
	return machineSet, wasConverted, nil
}

func Convert_Machine_ProviderConfig_To_ProviderSpec(in []byte) (*clusterv1alpha1.Machine, bool, error) {
	var wasConverted bool

	superMachine := &machineWithProviderSpecAndProviderConfig{}
	if err := json.Unmarshal(in, superMachine); err != nil {
		return nil, wasConverted, fmt.Errorf("error unmarshalling machine object: %w", err)
	}
	if superMachine.Spec.ProviderConfig != nil && superMachine.Spec.ProviderSpec != nil {
		return nil, wasConverted, fmt.Errorf("both .spec.providerConfig and .spec.ProviderSpec were non-nil for machine %s", superMachine.Name)
	}
	if superMachine.Spec.ProviderConfig != nil {
		superMachine.Spec.ProviderSpec = superMachine.Spec.ProviderConfig
		superMachine.Spec.ProviderConfig = nil
		wasConverted = true
	}

	machine := &clusterv1alpha1.Machine{}
	superMachineBytes, err := json.Marshal(superMachine)
	if err != nil {
		return nil, wasConverted, fmt.Errorf("failed to marshal superMachine object for machine %s: %w", superMachine.Name, err)
	}
	if err := json.Unmarshal(superMachineBytes, machine); err != nil {
		return nil, wasConverted, fmt.Errorf("failed to unmarshal superMachine object for machine %s back into machine object: %w", superMachine.Name, err)
	}
	return machine, wasConverted, nil
}
