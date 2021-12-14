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
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

type RawConfig struct {
	Endpoint      providerconfigtypes.ConfigVarString `json:"endpoint"`
	Username      providerconfigtypes.ConfigVarString `json:"username"`
	Password      providerconfigtypes.ConfigVarString `json:"password"`
	AllowInsecure providerconfigtypes.ConfigVarBool   `json:"allowInsecure"`

	ClusterName        providerconfigtypes.ConfigVarString `json:"clusterName"`
	ProjectName        providerconfigtypes.ConfigVarString `json:"projectName"`
	SubnetName         providerconfigtypes.ConfigVarString `json:"subnetName"`
	ImageName          providerconfigtypes.ConfigVarString `json:"imageName"`
	StorageContainerID providerconfigtypes.ConfigVarString `json:"storageContainerID"`

	// VM sizing configuration
	CPUs     int64  `json:"cpus"`
	CPUCores *int64 `json:"cpuCores,omitempty"`
	MemoryMB int64  `json:"memoryMB"`
	DiskSize *int64 `json:"diskSize,omitempty"`

	// Metadata related configuration
	Categories map[string]string `json:"categories,omitempty"`
}
