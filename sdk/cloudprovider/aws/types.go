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

package aws

import (
	"k8c.io/machine-controller/sdk/jsonutil"
	"k8c.io/machine-controller/sdk/providerconfig"
)

type RawConfig struct {
	AccessKeyID     providerconfig.ConfigVarString `json:"accessKeyId,omitempty"`
	SecretAccessKey providerconfig.ConfigVarString `json:"secretAccessKey,omitempty"`

	AssumeRoleARN        providerconfig.ConfigVarString `json:"assumeRoleARN,omitempty"`
	AssumeRoleExternalID providerconfig.ConfigVarString `json:"assumeRoleExternalID,omitempty"`

	Region             providerconfig.ConfigVarString   `json:"region"`
	AvailabilityZone   providerconfig.ConfigVarString   `json:"availabilityZone,omitempty"`
	VpcID              providerconfig.ConfigVarString   `json:"vpcId"`
	SubnetID           providerconfig.ConfigVarString   `json:"subnetId"`
	SecurityGroupIDs   []providerconfig.ConfigVarString `json:"securityGroupIDs,omitempty"`
	InstanceProfile    providerconfig.ConfigVarString   `json:"instanceProfile,omitempty"`
	InstanceType       providerconfig.ConfigVarString   `json:"instanceType,omitempty"`
	AMI                providerconfig.ConfigVarString   `json:"ami,omitempty"`
	DiskSize           int32                            `json:"diskSize"`
	DiskType           providerconfig.ConfigVarString   `json:"diskType,omitempty"`
	DiskIops           *int32                           `json:"diskIops,omitempty"`
	EBSVolumeEncrypted providerconfig.ConfigVarBool     `json:"ebsVolumeEncrypted"`
	Tags               map[string]string                `json:"tags,omitempty"`
	AssignPublicIP     *bool                            `json:"assignPublicIP,omitempty"`

	IsSpotInstance     *bool               `json:"isSpotInstance,omitempty"`
	SpotInstanceConfig *SpotInstanceConfig `json:"spotInstanceConfig,omitempty"`
}

type SpotInstanceConfig struct {
	MaxPrice             providerconfig.ConfigVarString `json:"maxPrice,omitempty"`
	PersistentRequest    providerconfig.ConfigVarBool   `json:"persistentRequest,omitempty"`
	InterruptionBehavior providerconfig.ConfigVarString `json:"interruptionBehavior,omitempty"`
}

// CPUArchitecture defines processor architectures returned by the AWS API.
type CPUArchitecture string

const (
	CPUArchitectureARM64  CPUArchitecture = "arm64"
	CPUArchitectureX86_64 CPUArchitecture = "x86_64"
	CPUArchitectureI386   CPUArchitecture = "i386"
)

func GetConfig(pconfig providerconfig.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
