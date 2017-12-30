package providerconfig

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime"
)

type Config struct {
	SSHPublicKeys []string `json:"sshPublicKeys"`

	CloudProvider     string               `json:"cloudProvider,omitempty"`
	CloudProviderSpec runtime.RawExtension `json:"cloudProviderSpec,omitempty"`

	OperatingSystem     string               `json:"operatingSystem"`
	OperatingSystemSpec runtime.RawExtension `json:"operatingSystemSpec"`
}

func GetConfig(r runtime.RawExtension) (*Config, error) {
	p := Config{}
	if err := json.Unmarshal(r.Raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
