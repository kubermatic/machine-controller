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
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
)

type RawConfig struct {
	Auth           Auth                                `json:"auth,omitempty"`
	VirtualMachine VirtualMachine                      `json:"virtual_machine,omitempty"`
	DNSPolicy      providerconfigtypes.ConfigVarString `json:"dnsPolicy,omitempty"`
	DNSConfig      *corev1.PodDNSConfig                `json:"dnsConfig,omitempty"`
}

// Auth
type Auth struct {
	Kubeconfig providerconfigtypes.ConfigVarString `json:"kubeconfig,omitempty"`
}

// VirtualMachine
type VirtualMachine struct {
	Name     providerconfigtypes.ConfigVarString `json:"name,omitempty"`
	Flavor   Flavor                              `json:"flavor,omitempty"`
	Template Template                            `json:"template,omitempty"`
}

// Flavor
type Flavor struct {
	Name    providerconfigtypes.ConfigVarString `json:"name,omitempty"`
	Profile providerconfigtypes.ConfigVarString `json:"profile,omitempty"`
}

// Template
type Template struct {
	CPU            providerconfigtypes.ConfigVarString `json:"cpu,omitempty"`
	Memory         providerconfigtypes.ConfigVarString `json:"memory,omitempty"`
	PrimaryDisk    PrimaryDisk                         `json:"primary_disk,omitempty"`
	SecondaryDisks []SecondaryDisks                    `json:"secondary_disks,omitempty"`
}

// PrimaryDisk
type PrimaryDisk struct {
	Disk
	OsImageURL providerconfigtypes.ConfigVarString `json:"os_image_url,omitempty"`
}

// SecondaryDisks
type SecondaryDisks struct {
	Disk
}

// Disk
type Disk struct {
	Size             providerconfigtypes.ConfigVarString `json:"size,omitempty"`
	StorageClassName providerconfigtypes.ConfigVarString `json:"storageClassName,omitempty"`
}
