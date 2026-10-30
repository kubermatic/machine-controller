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
	"encoding/json"
	"time"

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

	VirtioNIC = "virtio"
)

// RawDisk specifies a single disk, with some values maybe being fetched from secrets.
type RawDisk struct {
	// Size (in Gibibyte) specifies the size of the disk.
	Size int `json:"size"`
	// PerformanceType specifies the performance of the disk.
	PerformanceType providerconfig.ConfigVarString `json:"performanceType"`
	// Name as human readable identifier for the disk
	Name string `json:"name"`
	// Identifier of the disk to maintain engine sync
	Identifier string `json:"identifier,omitempty"`
}

// RawNetwork specifies a single network interface.
type RawNetwork struct {
	// Identifier of the VLAN to attach this network interface to.
	VlanID providerconfig.ConfigVarString `json:"vlan"`

	// IDs of prefixes to reserve IP addresses from for each Machine on network interface.
	//
	// Empty list means that no IPs will be reserved, but the interface will still be added.
	PrefixIDs []providerconfig.ConfigVarString `json:"prefixes"`

	// Bandwidth limit in MBit/s on the network interface. Allowed values are: 100, 1000, 10000.
	//
	// If unset, the default value from the Anexia Engine is used, which is usually 1000.
	BandwidthLimit int `json:"bandwidthLimit,omitempty"`

	// Identifier of the Network to maintain engine sync
	Identifier string `json:"identifier,omitempty"`
}

// RawConfig contains all the configuration values for VMs to create, with some values maybe being fetched from secrets.
type RawConfig struct {
	// LocationID specifies the datacenter location.
	LocationID providerconfig.ConfigVarString `json:"locationID"`

	// TemplateID specifies the template exactly, however most cases should use Template and TemplateBuild instead.
	TemplateID providerconfig.ConfigVarString `json:"templateID"`
	// Template specifies the template name.
	Template providerconfig.ConfigVarString `json:"template"`
	// TemplateBuild specifies the template build.
	TemplateBuild providerconfig.ConfigVarString `json:"templateBuild"`

	// CPUs specify the number of CPUs of the machine
	CPUs int `json:"cpus"`
	// CPUPerformanceType specifies the performance of the used CPU.
	CPUPerformanceType string `json:"cpuPerformanceType"`
	// Memory (in Mebibyte) specifies the main memory size.
	Memory int `json:"memory"`

	// DiskSize (in Gibibytes) for the main disk.
	DiskSize int `json:"diskSize"`
	// DiskPerformanceType specifies the performance of the main disk.
	DiskPerformanceType string `json:"diskPerformanceType"`

	// Disks specifies the additional disks.
	Disks []RawDisk `json:"disks"`

	// Configuration of the network interfaces. At least one entry with at
	// least one Prefix is required.
	Networks []RawNetwork `json:"networks"`

	AvailabilityZone string `json:"availability_zone,omitempty"`
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

	return rawConfig, json.Unmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
