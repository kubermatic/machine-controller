package conversions

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

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

func Convert_ProviderConfig_To_ProviderSpec(in []byte) (*clusterv1alpha1.Machine, bool, error) {
	var wasConverted bool

	superMachine := &machineWithProviderSpecAndProviderConfig{}
	if err := json.Unmarshal(in, superMachine); err != nil {
		return nil, wasConverted, fmt.Errorf("error unmarshalling machine object: %v", err)
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
		return nil, wasConverted, fmt.Errorf("failed to marshal superMachine object for machine %s: %v", superMachine.Name, err)
	}
	if err := json.Unmarshal(superMachineBytes, machine); err != nil {
		return nil, wasConverted, fmt.Errorf("failed to unmarhsla superMachine object for machine %s back into machine object: %v", superMachine.Name, err)
	}
	return machine, wasConverted, nil
}
