package types

import (
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

type RawConfig struct {
	Token             providerconfigtypes.ConfigVarString   `json:"token,omitempty"`
	Region            providerconfigtypes.ConfigVarString   `json:"region"`
	Size              providerconfigtypes.ConfigVarString   `json:"size"`
	Backups           providerconfigtypes.ConfigVarBool     `json:"backups"`
	IPv6              providerconfigtypes.ConfigVarBool     `json:"ipv6"`
	PrivateNetworking providerconfigtypes.ConfigVarBool     `json:"private_networking"`
	Monitoring        providerconfigtypes.ConfigVarBool     `json:"monitoring"`
	Tags              []providerconfigtypes.ConfigVarString `json:"tags,omitempty"`
}
