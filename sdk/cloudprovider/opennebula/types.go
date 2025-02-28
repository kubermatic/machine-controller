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

package opennebula

import (
	"k8c.io/machine-controller/sdk/jsonutil"
	"k8c.io/machine-controller/sdk/providerconfig"
)

type RawConfig struct {
	// Auth details
	Username providerconfig.ConfigVarString `json:"username,omitempty"`
	Password providerconfig.ConfigVarString `json:"password,omitempty"`
	Endpoint providerconfig.ConfigVarString `json:"endpoint,omitempty"`

	// Machine details
	CPU             *float64                       `json:"cpu"`
	VCPU            *int                           `json:"vcpu"`
	Memory          *int                           `json:"memory"`
	Image           providerconfig.ConfigVarString `json:"image"`
	Datastore       providerconfig.ConfigVarString `json:"datastore"`
	DiskSize        *int                           `json:"diskSize"`
	Network         providerconfig.ConfigVarString `json:"network"`
	EnableVNC       providerconfig.ConfigVarBool   `json:"enableVNC"`
	VMTemplateExtra map[string]string              `json:"vmTemplateExtra,omitempty"`
}

func GetConfig(pconfig providerconfig.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
