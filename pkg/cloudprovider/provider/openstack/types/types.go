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

package types

import (
	"github.com/kubermatic/machine-controller/pkg/jsonutil"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

type RawConfig struct {
	// Auth details
	IdentityEndpoint            providerconfigtypes.ConfigVarString `json:"identityEndpoint,omitempty"`
	Username                    providerconfigtypes.ConfigVarString `json:"username,omitempty"`
	Password                    providerconfigtypes.ConfigVarString `json:"password,omitempty"`
	ApplicationCredentialID     providerconfigtypes.ConfigVarString `json:"applicationCredentialID,omitempty"`
	ApplicationCredentialSecret providerconfigtypes.ConfigVarString `json:"applicationCredentialSecret,omitempty"`
	DomainName                  providerconfigtypes.ConfigVarString `json:"domainName,omitempty"`
	ProjectName                 providerconfigtypes.ConfigVarString `json:"projectName,omitempty"`
	ProjectID                   providerconfigtypes.ConfigVarString `json:"projectID,omitempty"`
	TenantName                  providerconfigtypes.ConfigVarString `json:"tenantName,omitempty"`
	TenantID                    providerconfigtypes.ConfigVarString `json:"tenantID,omitempty"`
	TokenID                     providerconfigtypes.ConfigVarString `json:"tokenId,omitempty"`
	Region                      providerconfigtypes.ConfigVarString `json:"region,omitempty"`
	InstanceReadyCheckPeriod    providerconfigtypes.ConfigVarString `json:"instanceReadyCheckPeriod,omitempty"`
	InstanceReadyCheckTimeout   providerconfigtypes.ConfigVarString `json:"instanceReadyCheckTimeout,omitempty"`
	ComputeAPIVersion           providerconfigtypes.ConfigVarString `json:"computeAPIVersion,omitempty"`

	// Machine details
	Image                 providerconfigtypes.ConfigVarString   `json:"image"`
	Flavor                providerconfigtypes.ConfigVarString   `json:"flavor"`
	SecurityGroups        []providerconfigtypes.ConfigVarString `json:"securityGroups,omitempty"`
	Network               providerconfigtypes.ConfigVarString   `json:"network,omitempty"`
	Subnet                providerconfigtypes.ConfigVarString   `json:"subnet,omitempty"`
	FloatingIPPool        providerconfigtypes.ConfigVarString   `json:"floatingIpPool,omitempty"`
	AvailabilityZone      providerconfigtypes.ConfigVarString   `json:"availabilityZone,omitempty"`
	TrustDevicePath       providerconfigtypes.ConfigVarBool     `json:"trustDevicePath"`
	RootDiskSizeGB        *int                                  `json:"rootDiskSizeGB"`
	RootDiskVolumeType    providerconfigtypes.ConfigVarString   `json:"rootDiskVolumeType,omitempty"`
	NodeVolumeAttachLimit *uint                                 `json:"nodeVolumeAttachLimit"`
	ServerGroup           providerconfigtypes.ConfigVarString   `json:"serverGroup"`
	// This tag is related to server metadata, not compute server's tag
	Tags map[string]string `json:"tags,omitempty"`
}

func GetConfig(pconfig providerconfigtypes.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
