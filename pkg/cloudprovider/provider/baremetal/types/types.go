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

	"k8s.io/apimachinery/pkg/runtime"
)

type RawConfig struct {
	MetadataClient *MetadataClientConfig               `json:"metadataClientConfig"`
	Driver         providerconfigtypes.ConfigVarString `json:"driver"`
	DriverSpec     runtime.RawExtension                `json:"driverSpec"`
}

type MetadataClientConfig struct {
	Endpoint   providerconfigtypes.ConfigVarString `json:"endpoint,omitempty"`
	AuthMethod providerconfigtypes.ConfigVarString `json:"authMethod,omitempty"`
	Username   providerconfigtypes.ConfigVarString `json:"username,omitempty"`
	Password   providerconfigtypes.ConfigVarString `json:"password,omitempty"`
	Token      providerconfigtypes.ConfigVarString `json:"token,omitempty"`
}

func GetConfig(pconfig providerconfigtypes.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
