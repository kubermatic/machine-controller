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

package alibaba

import (
	"k8c.io/machine-controller/sdk/jsonutil"
	"k8c.io/machine-controller/sdk/providerconfig"
)

type RawConfig struct {
	AccessKeyID             providerconfig.ConfigVarString `json:"accessKeyID,omitempty"`
	AccessKeySecret         providerconfig.ConfigVarString `json:"accessKeySecret,omitempty"`
	RegionID                providerconfig.ConfigVarString `json:"regionID,omitempty"`
	InstanceName            providerconfig.ConfigVarString `json:"instanceName,omitempty"`
	InstanceType            providerconfig.ConfigVarString `json:"instanceType,omitempty"`
	VSwitchID               providerconfig.ConfigVarString `json:"vSwitchID,omitempty"`
	InternetMaxBandwidthOut providerconfig.ConfigVarString `json:"internetMaxBandwidthOut,omitempty"`
	Labels                  map[string]string              `json:"labels,omitempty"`
	ZoneID                  providerconfig.ConfigVarString `json:"zoneID,omitempty"`
	DiskType                providerconfig.ConfigVarString `json:"diskType,omitempty"`
	DiskSize                providerconfig.ConfigVarString `json:"diskSize,omitempty"`
}

func GetConfig(pconfig providerconfig.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
