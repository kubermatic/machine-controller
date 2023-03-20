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

type RawConfig struct {
	// Auth details
	Username providerconfigtypes.ConfigVarString `json:"username,omitempty"`
	Password providerconfigtypes.ConfigVarString `json:"password,omitempty"`
	Endpoint providerconfigtypes.ConfigVarString `json:"endpoint,omitempty"`

	// Machine details
	Cpu             *float64                            `json:"cpu"`
	Vcpu            *int                                `json:"vcpu"`
	Memory          *int                                `json:"memory"`
	Image           providerconfigtypes.ConfigVarString `json:"image"`
	Datastore       providerconfigtypes.ConfigVarString `json:"datastore"`
	DiskSize        *int                                `json:"diskSize"`
	Network         providerconfigtypes.ConfigVarString `json:"network"`
	EnableVNC       providerconfigtypes.ConfigVarBool   `json:"enableVNC"`
	VMTemplateExtra map[string]string                   `json:"vmTemplateExtra,omitempty"`
}

func GetConfig(pconfig providerconfigtypes.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
