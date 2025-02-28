/*
Copyright 2019 The Machine Controller Authors.

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

package linode

import (
	"k8c.io/machine-controller/sdk/jsonutil"
	"k8c.io/machine-controller/sdk/providerconfig"
)

type RawConfig struct {
	Token             providerconfig.ConfigVarString   `json:"token,omitempty"`
	Region            providerconfig.ConfigVarString   `json:"region"`
	Type              providerconfig.ConfigVarString   `json:"type"`
	Backups           providerconfig.ConfigVarBool     `json:"backups"`
	PrivateNetworking providerconfig.ConfigVarBool     `json:"private_networking"`
	Tags              []providerconfig.ConfigVarString `json:"tags,omitempty"`
}

func GetConfig(pconfig providerconfig.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
