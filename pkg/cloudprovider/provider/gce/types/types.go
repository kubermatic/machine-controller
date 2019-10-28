package types

import (
	"encoding/json"
	"fmt"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	"k8s.io/apimachinery/pkg/runtime"
)

// CloudProviderSpec contains the specification of the cloud provider taken
// from the provider configuration.
type CloudProviderSpec struct {
	ServiceAccount        providerconfigtypes.ConfigVarString `json:"serviceAccount,omitempty"`
	Zone                  providerconfigtypes.ConfigVarString `json:"zone"`
	MachineType           providerconfigtypes.ConfigVarString `json:"machineType"`
	DiskSize              int64                               `json:"diskSize"`
	DiskType              providerconfigtypes.ConfigVarString `json:"diskType"`
	Network               providerconfigtypes.ConfigVarString `json:"network"`
	Subnetwork            providerconfigtypes.ConfigVarString `json:"subnetwork"`
	Preemptible           providerconfigtypes.ConfigVarBool   `json:"preemptible"`
	Labels                map[string]string                   `json:"labels,omitempty"`
	Tags                  []string                            `json:"tags,omitempty"`
	AssignPublicIPAddress *providerconfigtypes.ConfigVarBool  `json:"assignPublicIPAddress,omitempty"`
	MultiZone             providerconfigtypes.ConfigVarBool   `json:"multizone"`
	Regional              providerconfigtypes.ConfigVarBool   `json:"regional"`
}

// UpdateProviderSpec updates the given provider spec with changed
// configuration values.
func (cpSpec *CloudProviderSpec) UpdateProviderSpec(spec v1alpha1.ProviderSpec) (*runtime.RawExtension, error) {
	if spec.Value == nil {
		return nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	providerConfig := providerconfigtypes.Config{}
	err := json.Unmarshal(spec.Value.Raw, &providerConfig)
	if err != nil {
		return nil, err
	}
	rawCPSpec, err := json.Marshal(cpSpec)
	if err != nil {
		return nil, err
	}
	providerConfig.CloudProviderSpec = runtime.RawExtension{Raw: rawCPSpec}
	rawProviderConfig, err := json.Marshal(providerConfig)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: rawProviderConfig}, nil
}
