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

// RawConfig is a direct representation of an Azure machine object's configuration.
type RawConfig struct {
	SubscriptionID providerconfigtypes.ConfigVarString `json:"subscriptionID,omitempty"`
	TenantID       providerconfigtypes.ConfigVarString `json:"tenantID,omitempty"`
	ClientID       providerconfigtypes.ConfigVarString `json:"clientID,omitempty"`
	ClientSecret   providerconfigtypes.ConfigVarString `json:"clientSecret,omitempty"`

	Location                    providerconfigtypes.ConfigVarString `json:"location"`
	ResourceGroup               providerconfigtypes.ConfigVarString `json:"resourceGroup"`
	VNetResourceGroup           providerconfigtypes.ConfigVarString `json:"vnetResourceGroup"`
	VMSize                      providerconfigtypes.ConfigVarString `json:"vmSize"`
	VNetName                    providerconfigtypes.ConfigVarString `json:"vnetName"`
	SubnetName                  providerconfigtypes.ConfigVarString `json:"subnetName"`
	LoadBalancerSku             providerconfigtypes.ConfigVarString `json:"loadBalancerSku"`
	RouteTableName              providerconfigtypes.ConfigVarString `json:"routeTableName"`
	AvailabilitySet             providerconfigtypes.ConfigVarString `json:"availabilitySet"`
	AssignAvailabilitySet       *bool                               `json:"assignAvailabilitySet"`
	SecurityGroupName           providerconfigtypes.ConfigVarString `json:"securityGroupName"`
	Zones                       []string                            `json:"zones"`
	ImagePlan                   *ImagePlan                          `json:"imagePlan,omitempty"`
	ImageReference              *ImageReference                     `json:"imageReference,omitempty"`
	EnableAcceleratedNetworking *bool                               `json:"enableAcceleratedNetworking"`
	EnableBootDiagnostics       *bool                               `json:"enableBootDiagnostics,omitempty"`

	ImageID        providerconfigtypes.ConfigVarString `json:"imageID"`
	OSDiskSize     int32                               `json:"osDiskSize"`
	OSDiskSKU      *string                             `json:"osDiskSKU,omitempty"`
	DataDiskSize   int32                               `json:"dataDiskSize"`
	DataDiskSKU    *string                             `json:"dataDiskSKU,omitempty"`
	AssignPublicIP providerconfigtypes.ConfigVarBool   `json:"assignPublicIP"`
	PublicIPSKU    *string                             `json:"publicIPSKU,omitempty"`
	Tags           map[string]string                   `json:"tags,omitempty"`
}

// ImagePlan contains azure OS Plan fields for the marketplace images.
type ImagePlan struct {
	Name      string `json:"name,omitempty"`
	Publisher string `json:"publisher,omitempty"`
	Product   string `json:"product,omitempty"`
}

// ImageReference specifies information about the image to use.
type ImageReference struct {
	Publisher string `json:"publisher,omitempty"`
	Offer     string `json:"offer,omitempty"`
	Sku       string `json:"sku,omitempty"`
	Version   string `json:"version,omitempty"`
}

func GetConfig(pconfig providerconfigtypes.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
