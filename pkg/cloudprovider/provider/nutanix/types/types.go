/*
Copyright 2021 The Machine Controller Authors.

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

const (
	VMKind      = "vm"
	ProjectKind = "project"
	ClusterKind = "cluster"
	SubnetKind  = "subnet"
	DiskKind    = "disk"
	ImageKind   = "image"
)

type RawConfig struct {
	Endpoint      providerconfigtypes.ConfigVarString `json:"endpoint"`
	Port          providerconfigtypes.ConfigVarString `json:"port"`
	Username      providerconfigtypes.ConfigVarString `json:"username"`
	Password      providerconfigtypes.ConfigVarString `json:"password"`
	AllowInsecure providerconfigtypes.ConfigVarBool   `json:"allowInsecure"`
	ProxyURL      providerconfigtypes.ConfigVarString `json:"proxyURL,omitempty"`

	ClusterName           providerconfigtypes.ConfigVarString  `json:"clusterName"`
	ProjectName           *providerconfigtypes.ConfigVarString `json:"projectName,omitempty"`
	SubnetName            providerconfigtypes.ConfigVarString  `json:"subnetName"`
	AdditionalSubnetNames []string                             `json:"additionalSubnetNames,omitempty"`
	ImageName             providerconfigtypes.ConfigVarString  `json:"imageName"`

	// VM sizing configuration
	CPUs           int64  `json:"cpus"`
	CPUCores       *int64 `json:"cpuCores,omitempty"`
	CPUPassthrough *bool  `json:"cpuPassthrough,omitempty"`
	MemoryMB       int64  `json:"memoryMB"`
	DiskSize       *int64 `json:"diskSize,omitempty"`

	// Metadata related configuration
	Categories map[string]string `json:"categories,omitempty"`
}

type ErrorResponse struct {
	APIVersion  string             `json:"api_version"`
	Kind        string             `json:"kind"`
	State       string             `json:"state"`
	MessageList []ErrorResponseMsg `json:"message_list"`
	Code        int32              `json:"code"`
}

type ErrorResponseMsg struct {
	Message string `json:"message"`
	Reason  string `json:"reason"`
}

func GetConfig(pconfig providerconfigtypes.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
