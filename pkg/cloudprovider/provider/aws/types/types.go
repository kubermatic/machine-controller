package types

import (
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

type Config struct {
	AccessKeyID     providerconfigtypes.ConfigVarString `json:"accessKeyId,omitempty"`
	SecretAccessKey providerconfigtypes.ConfigVarString `json:"secretAccessKey,omitempty"`

	Region           providerconfigtypes.ConfigVarString   `json:"region"`
	AvailabilityZone providerconfigtypes.ConfigVarString   `json:"availabilityZone,omitempty"`
	VpcID            providerconfigtypes.ConfigVarString   `json:"vpcId"`
	SubnetID         providerconfigtypes.ConfigVarString   `json:"subnetId"`
	SecurityGroupIDs []providerconfigtypes.ConfigVarString `json:"securityGroupIDs,omitempty"`
	InstanceProfile  providerconfigtypes.ConfigVarString   `json:"instanceProfile,omitempty"`
	IsSpotInstance   *bool                                 `json:"isSpotInstance,omitempty"`
	InstanceType     providerconfigtypes.ConfigVarString   `json:"instanceType,omitempty"`
	AMI              providerconfigtypes.ConfigVarString   `json:"ami,omitempty"`
	DiskSize         int64                                 `json:"diskSize"`
	DiskType         providerconfigtypes.ConfigVarString   `json:"diskType,omitempty"`
	DiskIops         *int64                                `json:"diskIops,omitempty"`
	Tags             map[string]string                     `json:"tags,omitempty"`
	AssignPublicIP   *bool                                 `json:"assignPublicIP,omitempty"`
}
