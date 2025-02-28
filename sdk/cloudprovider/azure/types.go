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

package azure

import (
	"k8c.io/machine-controller/sdk/jsonutil"
	"k8c.io/machine-controller/sdk/providerconfig"
)

// RawConfig is a direct representation of an Azure machine object's configuration.
type RawConfig struct {
	SubscriptionID providerconfig.ConfigVarString `json:"subscriptionID,omitempty"`
	TenantID       providerconfig.ConfigVarString `json:"tenantID,omitempty"`
	ClientID       providerconfig.ConfigVarString `json:"clientID,omitempty"`
	ClientSecret   providerconfig.ConfigVarString `json:"clientSecret,omitempty"`

	Location                    providerconfig.ConfigVarString `json:"location"`
	ResourceGroup               providerconfig.ConfigVarString `json:"resourceGroup"`
	VNetResourceGroup           providerconfig.ConfigVarString `json:"vnetResourceGroup"`
	VMSize                      providerconfig.ConfigVarString `json:"vmSize"`
	VNetName                    providerconfig.ConfigVarString `json:"vnetName"`
	SubnetName                  providerconfig.ConfigVarString `json:"subnetName"`
	LoadBalancerSku             providerconfig.ConfigVarString `json:"loadBalancerSku"`
	RouteTableName              providerconfig.ConfigVarString `json:"routeTableName"`
	AvailabilitySet             providerconfig.ConfigVarString `json:"availabilitySet"`
	AssignAvailabilitySet       *bool                          `json:"assignAvailabilitySet"`
	SecurityGroupName           providerconfig.ConfigVarString `json:"securityGroupName"`
	Zones                       []string                       `json:"zones"`
	ImagePlan                   *ImagePlan                     `json:"imagePlan,omitempty"`
	ImageReference              *ImageReference                `json:"imageReference,omitempty"`
	EnableAcceleratedNetworking *bool                          `json:"enableAcceleratedNetworking"`
	EnableBootDiagnostics       *bool                          `json:"enableBootDiagnostics,omitempty"`

	ImageID        providerconfig.ConfigVarString `json:"imageID"`
	OSDiskSize     int32                          `json:"osDiskSize"`
	OSDiskSKU      *string                        `json:"osDiskSKU,omitempty"`
	DataDiskSize   int32                          `json:"dataDiskSize"`
	DataDiskSKU    *string                        `json:"dataDiskSKU,omitempty"`
	AssignPublicIP providerconfig.ConfigVarBool   `json:"assignPublicIP"`
	PublicIPSKU    *string                        `json:"publicIPSKU,omitempty"`
	Tags           map[string]string              `json:"tags,omitempty"`
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

func GetConfig(pconfig providerconfig.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
