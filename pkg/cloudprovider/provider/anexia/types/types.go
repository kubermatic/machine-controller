/*
Copyright 2020 The Machine Controller Authors.

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
	"time"

	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

const (
	AnxTokenEnv = "ANEXIA_TOKEN"

	CreateRequestTimeout = 15 * time.Minute
	GetRequestTimeout    = 1 * time.Minute
	DeleteRequestTimeout = 1 * time.Minute

	VmxNet3NIC       = "vmxnet3"
	MachinePoweredOn = "poweredOn"
)

type RawConfig struct {
	Token      providerconfigtypes.ConfigVarString `json:"token,omitempty"`
	VlanID     providerconfigtypes.ConfigVarString `json:"vlanID"`
	LocationID providerconfigtypes.ConfigVarString `json:"locationID"`
	TemplateID providerconfigtypes.ConfigVarString `json:"templateID"`
	CPUs       int                                 `json:"cpus"`
	Memory     int                                 `json:"memory"`
	DiskSize   int                                 `json:"diskSize"`
}

type ProviderStatus struct {
	InstanceID     string `json:"instanceID"`
	ProvisioningID string `json:"provisioningID"`
	// TODO: add conditions to track progress on the provider side
}
