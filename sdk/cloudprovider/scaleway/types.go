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

package scaleway

import (
	"k8c.io/machine-controller/sdk/jsonutil"
	"k8c.io/machine-controller/sdk/providerconfig"
)

type RawConfig struct {
	AccessKey      providerconfig.ConfigVarString `json:"accessKey,omitempty"`
	SecretKey      providerconfig.ConfigVarString `json:"secretKey,omitempty"`
	ProjectID      providerconfig.ConfigVarString `json:"projectId,omitempty"`
	Zone           providerconfig.ConfigVarString `json:"zone,omitempty"`
	CommercialType providerconfig.ConfigVarString `json:"commercialType"`
	IPv6           providerconfig.ConfigVarBool   `json:"ipv6"`
	Tags           []string                       `json:"tags,omitempty"`
}

func GetConfig(pconfig providerconfig.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
