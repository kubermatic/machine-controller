package types

import (
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

type RawConfig struct {
	Token      providerconfigtypes.ConfigVarString   `json:"token,omitempty"`
	ServerType providerconfigtypes.ConfigVarString   `json:"serverType"`
	Datacenter providerconfigtypes.ConfigVarString   `json:"datacenter"`
	Location   providerconfigtypes.ConfigVarString   `json:"location"`
	Networks   []providerconfigtypes.ConfigVarString `json:"networks"`
	Labels     map[string]string                     `json:"labels,omitempty"`
}
