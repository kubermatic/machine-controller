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

package types

import (
	"github.com/kubermatic/machine-controller/pkg/jsonutil"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

type IPAllocationMode string

const (
	PoolIPAllocationMode IPAllocationMode = "POOL"
	DHCPIPAllocationMode IPAllocationMode = "DHCP"
)

// RawConfig represents VMware Cloud Director specific configuration.
type RawConfig struct {
	// Provider configuration.
	Username      providerconfigtypes.ConfigVarString `json:"username"`
	Password      providerconfigtypes.ConfigVarString `json:"password"`
	Organization  providerconfigtypes.ConfigVarString `json:"organization"`
	URL           providerconfigtypes.ConfigVarString `json:"url"`
	VDC           providerconfigtypes.ConfigVarString `json:"vdc"`
	AllowInsecure providerconfigtypes.ConfigVarBool   `json:"allowInsecure"`

	// VM configuration.
	VApp            providerconfigtypes.ConfigVarString `json:"vapp"`
	Template        providerconfigtypes.ConfigVarString `json:"template"`
	Catalog         providerconfigtypes.ConfigVarString `json:"catalog"`
	PlacementPolicy *string                             `json:"placementPolicy,omitempty"`

	// Network configuration.
	Network          providerconfigtypes.ConfigVarString `json:"network"`
	IPAllocationMode IPAllocationMode                    `json:"ipAllocationMode"`

	// Compute configuration.
	CPUs         int64   `json:"cpus"`
	CPUCores     int64   `json:"cpuCores"`
	MemoryMB     int64   `json:"memoryMB"`
	SizingPolicy *string `json:"sizingPolicy,omitempty"`

	// Storage configuration.
	DiskSizeGB     *int64  `json:"diskSizeGB,omitempty"`
	DiskBusType    *string `json:"diskBusType,omitempty"`
	DiskIOPS       *int64  `json:"diskIOPS,omitempty"`
	StorageProfile *string `json:"storageProfile,omitempty"`

	// Metadata configuration.
	Metadata *map[string]string `json:"metadata,omitempty"`
}

func GetConfig(pconfig providerconfigtypes.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
