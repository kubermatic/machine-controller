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

package gce

import (
	"encoding/json"
	"fmt"

	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
	"k8c.io/machine-controller/sdk/jsonutil"
	"k8c.io/machine-controller/sdk/providerconfig"

	"k8s.io/apimachinery/pkg/runtime"
)

// CloudProviderSpec contains the specification of the cloud provider taken
// from the provider configuration.
type CloudProviderSpec struct {
	// ServiceAccount must be base64-encoded.
	ServiceAccount               providerconfig.ConfigVarString  `json:"serviceAccount,omitempty"`
	Zone                         providerconfig.ConfigVarString  `json:"zone"`
	MachineType                  providerconfig.ConfigVarString  `json:"machineType"`
	DiskSize                     int64                           `json:"diskSize"`
	DiskType                     providerconfig.ConfigVarString  `json:"diskType"`
	Network                      providerconfig.ConfigVarString  `json:"network"`
	Subnetwork                   providerconfig.ConfigVarString  `json:"subnetwork"`
	Preemptible                  providerconfig.ConfigVarBool    `json:"preemptible"`
	AutomaticRestart             *providerconfig.ConfigVarBool   `json:"automaticRestart,omitempty"`
	ProvisioningModel            *providerconfig.ConfigVarString `json:"provisioningModel,omitempty"`
	Labels                       map[string]string               `json:"labels,omitempty"`
	Tags                         []string                        `json:"tags,omitempty"`
	AssignPublicIPAddress        *providerconfig.ConfigVarBool   `json:"assignPublicIPAddress,omitempty"`
	MultiZone                    providerconfig.ConfigVarBool    `json:"multizone"`
	Regional                     providerconfig.ConfigVarBool    `json:"regional"`
	CustomImage                  providerconfig.ConfigVarString  `json:"customImage,omitempty"`
	DisableMachineServiceAccount providerconfig.ConfigVarBool    `json:"disableMachineServiceAccount,omitempty"`
	EnableNestedVirtualization   providerconfig.ConfigVarBool    `json:"enableNestedVirtualization,omitempty"`
	MinCPUPlatform               providerconfig.ConfigVarString  `json:"minCPUPlatform,omitempty"`
	GuestOSFeatures              []string                        `json:"guestOSFeatures,omitempty"`
	ProjectID                    providerconfig.ConfigVarString  `json:"projectID,omitempty"`
}

// UpdateProviderSpec updates the given provider spec with changed
// configuration values.
func (cpSpec *CloudProviderSpec) UpdateProviderSpec(spec clusterv1alpha1.ProviderSpec) (*runtime.RawExtension, error) {
	if spec.Value == nil {
		return nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	providerConfig := providerconfig.Config{}
	err := json.Unmarshal(spec.Value.Raw, &providerConfig)
	if err != nil {
		return nil, err
	}
	rawCPSpec, err := json.Marshal(cpSpec)
	if err != nil {
		return nil, err
	}
	providerConfig.CloudProviderSpec = runtime.RawExtension{Raw: rawCPSpec}
	rawProviderConfig, err := json.Marshal(providerConfig)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: rawProviderConfig}, nil
}

type RawConfig = CloudProviderSpec

func GetConfig(pconfig providerconfig.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
