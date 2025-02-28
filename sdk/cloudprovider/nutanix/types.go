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

package nutanix

import (
	"k8c.io/machine-controller/sdk/jsonutil"
	"k8c.io/machine-controller/sdk/providerconfig"
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
	Endpoint      providerconfig.ConfigVarString `json:"endpoint"`
	Port          providerconfig.ConfigVarString `json:"port"`
	Username      providerconfig.ConfigVarString `json:"username"`
	Password      providerconfig.ConfigVarString `json:"password"`
	AllowInsecure providerconfig.ConfigVarBool   `json:"allowInsecure"`
	ProxyURL      providerconfig.ConfigVarString `json:"proxyURL,omitempty"`

	ClusterName           providerconfig.ConfigVarString  `json:"clusterName"`
	ProjectName           *providerconfig.ConfigVarString `json:"projectName,omitempty"`
	SubnetName            providerconfig.ConfigVarString  `json:"subnetName"`
	AdditionalSubnetNames []string                        `json:"additionalSubnetNames,omitempty"`
	ImageName             providerconfig.ConfigVarString  `json:"imageName"`

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

func GetConfig(pconfig providerconfig.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
