package types

import (
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

// RawConfig is a direct representation of an Azure machine object's configuration
type RawConfig struct {
	SubscriptionID providerconfigtypes.ConfigVarString `json:"subscriptionID,omitempty"`
	TenantID       providerconfigtypes.ConfigVarString `json:"tenantID,omitempty"`
	ClientID       providerconfigtypes.ConfigVarString `json:"clientID,omitempty"`
	ClientSecret   providerconfigtypes.ConfigVarString `json:"clientSecret,omitempty"`

	Location          providerconfigtypes.ConfigVarString `json:"location"`
	ResourceGroup     providerconfigtypes.ConfigVarString `json:"resourceGroup"`
	VMSize            providerconfigtypes.ConfigVarString `json:"vmSize"`
	VNetName          providerconfigtypes.ConfigVarString `json:"vnetName"`
	SubnetName        providerconfigtypes.ConfigVarString `json:"subnetName"`
	RouteTableName    providerconfigtypes.ConfigVarString `json:"routeTableName"`
	AvailabilitySet   providerconfigtypes.ConfigVarString `json:"availabilitySet"`
	SecurityGroupName providerconfigtypes.ConfigVarString `json:"securityGroupName"`

	AssignPublicIP providerconfigtypes.ConfigVarBool `json:"assignPublicIP"`
	Tags           map[string]string                 `json:"tags,omitempty"`
}
