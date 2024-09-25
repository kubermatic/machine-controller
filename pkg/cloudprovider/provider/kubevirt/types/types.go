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
	kubevirtv1 "kubevirt.io/api/core/v1"

	"k8c.io/machine-controller/pkg/jsonutil"
	providerconfigtypes "k8c.io/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
)

var SupportedOS = map[providerconfigtypes.OperatingSystem]*struct{}{
	providerconfigtypes.OperatingSystemCentOS:     nil,
	providerconfigtypes.OperatingSystemUbuntu:     nil,
	providerconfigtypes.OperatingSystemRHEL:       nil,
	providerconfigtypes.OperatingSystemFlatcar:    nil,
	providerconfigtypes.OperatingSystemRockyLinux: nil,
}

type RawConfig struct {
	ClusterName               providerconfigtypes.ConfigVarString `json:"clusterName"`
	Auth                      Auth                                `json:"auth,omitempty"`
	VirtualMachine            VirtualMachine                      `json:"virtualMachine,omitempty"`
	Affinity                  Affinity                            `json:"affinity,omitempty"`
	TopologySpreadConstraints []TopologySpreadConstraint          `json:"topologySpreadConstraints"`
}

// Auth.
type Auth struct {
	Kubeconfig providerconfigtypes.ConfigVarString `json:"kubeconfig,omitempty"`
}

// VirtualMachine.
type VirtualMachine struct {
	// Deprecated: use Instancetype/Preference instead.
	Flavor Flavor `json:"flavor,omitempty"`
	// Instancetype is optional.
	Instancetype *kubevirtv1.InstancetypeMatcher `json:"instancetype,omitempty"`
	// Preference is optional.
	Preference *kubevirtv1.PreferenceMatcher       `json:"preference,omitempty"`
	Template   Template                            `json:"template,omitempty"`
	DNSPolicy  providerconfigtypes.ConfigVarString `json:"dnsPolicy,omitempty"`
	DNSConfig  *corev1.PodDNSConfig                `json:"dnsConfig,omitempty"`
	Location   *Location                           `json:"location,omitempty"`
}

// Flavor.
type Flavor struct {
	Name    providerconfigtypes.ConfigVarString `json:"name,omitempty"`
	Profile providerconfigtypes.ConfigVarString `json:"profile,omitempty"`
}

// Template.
type Template struct {
	CPUs           providerconfigtypes.ConfigVarString `json:"cpus,omitempty"`
	Memory         providerconfigtypes.ConfigVarString `json:"memory,omitempty"`
	PrimaryDisk    PrimaryDisk                         `json:"primaryDisk,omitempty"`
	SecondaryDisks []SecondaryDisks                    `json:"secondaryDisks,omitempty"`
}

// PrimaryDisk.
type PrimaryDisk struct {
	Disk
	// StorageTarget describes which VirtualMachine storage target will be used in the DataVolumeTemplate.
	StorageTarget providerconfigtypes.ConfigVarString `json:"storageTarget,omitempty"`
	// OsImage describes the OS that will be installed on the VirtualMachine.
	OsImage providerconfigtypes.ConfigVarString `json:"osImage,omitempty"`
	// Source describes the VM Disk Image source.
	Source providerconfigtypes.ConfigVarString `json:"source,omitempty"`
	// PullMethod describes the VM Disk Image source optional pull method for registry source. Defaults to 'node'.
	PullMethod providerconfigtypes.ConfigVarString `json:"pullMethod,omitempty"`
}

// SecondaryDisks.
type SecondaryDisks struct {
	Disk
}

// Disk.
type Disk struct {
	Size              providerconfigtypes.ConfigVarString `json:"size,omitempty"`
	StorageClassName  providerconfigtypes.ConfigVarString `json:"storageClassName,omitempty"`
	StorageAccessType providerconfigtypes.ConfigVarString `json:"storageAccessType,omitempty"`
}

// Affinity.
type Affinity struct {
	// Deprecated: Use TopologySpreadConstraint instead.
	PodAffinityPreset providerconfigtypes.ConfigVarString `json:"podAffinityPreset,omitempty"`
	// Deprecated: Use TopologySpreadConstraint instead.
	PodAntiAffinityPreset providerconfigtypes.ConfigVarString `json:"podAntiAffinityPreset,omitempty"`
	NodeAffinityPreset    NodeAffinityPreset                  `json:"nodeAffinityPreset,omitempty"`
}

// NodeAffinityPreset.
type NodeAffinityPreset struct {
	Type   providerconfigtypes.ConfigVarString   `json:"type,omitempty"`
	Key    providerconfigtypes.ConfigVarString   `json:"key,omitempty"`
	Values []providerconfigtypes.ConfigVarString `json:"values,omitempty"`
}

// TopologySpreadConstraint describes topology spread constraints for VMs.
type TopologySpreadConstraint struct {
	// MaxSkew describes the degree to which VMs may be unevenly distributed.
	MaxSkew providerconfigtypes.ConfigVarString `json:"maxSkew,omitempty"`
	// TopologyKey is the key of infra-node labels.
	TopologyKey providerconfigtypes.ConfigVarString `json:"topologyKey,omitempty"`
	// WhenUnsatisfiable indicates how to deal with a VM if it doesn't satisfy
	// the spread constraint.
	WhenUnsatisfiable providerconfigtypes.ConfigVarString `json:"whenUnsatisfiable,omitempty"`
}

// Location describes the region and zone where the machines are created at and where the deployed resources will reside.
type Location struct {
	Region string `json:"region,omitempty"`
	Zone   string `json:"zone,omitempty"`
}

func GetConfig(pconfig providerconfigtypes.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
