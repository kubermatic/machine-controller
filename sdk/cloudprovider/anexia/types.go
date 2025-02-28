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

package anexia

import (
	"time"

	"k8c.io/machine-controller/sdk/jsonutil"
	"k8c.io/machine-controller/sdk/providerconfig"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	AnxTokenEnv = "ANEXIA_TOKEN"

	CreateRequestTimeout = 15 * time.Minute
	GetRequestTimeout    = 1 * time.Minute
	DeleteRequestTimeout = 1 * time.Minute

	IPStateBound          = "Bound"
	IPStateUnbound        = "Unbound"
	IPProvisioningExpires = 1800 * time.Second

	VmxNet3NIC       = "vmxnet3"
	MachinePoweredOn = "poweredOn"
)

// RawDisk specifies a single disk, with some values maybe being fetched from secrets.
type RawDisk struct {
	Size            int                            `json:"size"`
	PerformanceType providerconfig.ConfigVarString `json:"performanceType"`
}

// RawNetwork specifies a single network interface.
type RawNetwork struct {
	// Identifier of the VLAN to attach this network interface to.
	VlanID providerconfig.ConfigVarString `json:"vlan"`

	// IDs of prefixes to reserve IP addresses from for each Machine on network interface.
	//
	// Empty list means that no IPs will be reserved, but the interface will still be added.
	PrefixIDs []providerconfig.ConfigVarString `json:"prefixes"`
}

// RawConfig contains all the configuration values for VMs to create, with some values maybe being fetched from secrets.
type RawConfig struct {
	Token      providerconfig.ConfigVarString `json:"token,omitempty"`
	LocationID providerconfig.ConfigVarString `json:"locationID"`

	TemplateID    providerconfig.ConfigVarString `json:"templateID"`
	Template      providerconfig.ConfigVarString `json:"template"`
	TemplateBuild providerconfig.ConfigVarString `json:"templateBuild"`

	CPUs               int    `json:"cpus"`
	CPUPerformanceType string `json:"cpuPerformanceType"`
	Memory             int    `json:"memory"`

	// Deprecated, use Disks instead.
	DiskSize int `json:"diskSize"`

	Disks []RawDisk `json:"disks"`

	// Deprecated, use Networks instead.
	VlanID providerconfig.ConfigVarString `json:"vlanID"`

	// Configuration of the network interfaces. At least one entry with at
	// least one Prefix is required.
	Networks []RawNetwork `json:"networks"`
}

type NetworkAddressStatus struct {
	ReservedIP            string    `json:"reservedIP"`
	IPState               string    `json:"ipState"`
	IPProvisioningExpires time.Time `json:"ipProvisioningExpires"`
}

type NetworkStatus struct {
	// each entry belongs to a config.Networks.Prefix entry at the same index
	Addresses []NetworkAddressStatus `json:"addresses"`
}

type ProviderStatus struct {
	InstanceID       string             `json:"instanceID"`
	ProvisioningID   string             `json:"provisioningID"`
	DeprovisioningID string             `json:"deprovisioningID"`
	Conditions       []metav1.Condition `json:"conditions,omitempty"`

	// each entry belongs to the config.Networks entry at the same index
	Networks []NetworkStatus `json:"networkStatus,omitempty"`
}

func GetConfig(pconfig providerconfig.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
