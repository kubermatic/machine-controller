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

package kubevirt

import (
	kubevirtcorev1 "kubevirt.io/api/core/v1"

	"k8c.io/machine-controller/sdk/jsonutil"
	"k8c.io/machine-controller/sdk/providerconfig"

	corev1 "k8s.io/api/core/v1"
)

var SupportedOS = map[providerconfig.OperatingSystem]*struct{}{
	providerconfig.OperatingSystemUbuntu:     nil,
	providerconfig.OperatingSystemRHEL:       nil,
	providerconfig.OperatingSystemFlatcar:    nil,
	providerconfig.OperatingSystemRockyLinux: nil,
}

type RawConfig struct {
	ClusterName               providerconfig.ConfigVarString `json:"clusterName"`
	ProjectID                 providerconfig.ConfigVarString `json:"projectID,omitempty"`
	Auth                      Auth                           `json:"auth,omitempty"`
	VirtualMachine            VirtualMachine                 `json:"virtualMachine,omitempty"`
	Affinity                  Affinity                       `json:"affinity,omitempty"`
	TopologySpreadConstraints []TopologySpreadConstraint     `json:"topologySpreadConstraints"`
}

// Auth.
type Auth struct {
	Kubeconfig providerconfig.ConfigVarString `json:"kubeconfig,omitempty"`
}

// VirtualMachine.
type VirtualMachine struct {
	// Deprecated: use Instancetype/Preference instead.
	Flavor Flavor `json:"flavor,omitempty"`
	// Instancetype is optional.
	Instancetype *kubevirtcorev1.InstancetypeMatcher `json:"instancetype,omitempty"`
	// Preference is optional.
	Preference              *kubevirtcorev1.PreferenceMatcher `json:"preference,omitempty"`
	Template                Template                          `json:"template,omitempty"`
	DNSPolicy               providerconfig.ConfigVarString    `json:"dnsPolicy,omitempty"`
	DNSConfig               *corev1.PodDNSConfig              `json:"dnsConfig,omitempty"`
	Location                *Location                         `json:"location,omitempty"`
	ProviderNetwork         *ProviderNetwork                  `json:"providerNetwork,omitempty"`
	EnableNetworkMultiQueue providerconfig.ConfigVarBool      `json:"enableNetworkMultiQueue,omitempty"`
	EvictionStrategy        string                            `json:"evictionStrategy,omitempty"`
}

// Flavor.
type Flavor struct {
	Name    providerconfig.ConfigVarString `json:"name,omitempty"`
	Profile providerconfig.ConfigVarString `json:"profile,omitempty"`
}

// Template.
type Template struct {
	// VCPUs is to configure vcpus used by a the virtual machine
	// when using kubevirts cpuAllocationRatio feature this leads to auto assignment of the
	// calculated ratio as resource cpu requests for the pod which launches the virtual machine
	VCPUs VCPUs `json:"vcpus,omitempty"`
	// CPUs is to configure cpu requests and limits directly for the pod which launches the virtual machine
	// and is related to the underlying hardware
	CPUs           providerconfig.ConfigVarString `json:"cpus,omitempty"`
	Memory         providerconfig.ConfigVarString `json:"memory,omitempty"`
	PrimaryDisk    PrimaryDisk                    `json:"primaryDisk,omitempty"`
	SecondaryDisks []SecondaryDisks               `json:"secondaryDisks,omitempty"`
}

// VCPUs.
type VCPUs struct {
	Cores int `json:"cores,omitempty"`
}

// PrimaryDisk.
type PrimaryDisk struct {
	Disk
	// DataVolumeSecretRef is the name of the secret that will be sent to the CDI data importer pod to read basic auth parameters.
	DataVolumeSecretRef providerconfig.ConfigVarString `json:"dataVolumeSecretRef,omitempty"`
	// ExtraHeaders is a list of strings containing extra headers to include with HTTP transfer requests
	// +optional
	ExtraHeaders []string `json:"extraHeaders,omitempty"`
	// ExtraHeadersSecretRef is a secret that contains a list of strings containing extra headers to include with HTTP transfer requests
	// +optional
	ExtraHeadersSecretRef providerconfig.ConfigVarString `json:"extraHeadersSecretRef,omitempty"`
	// StorageTarget describes which VirtualMachine storage target will be used in the DataVolumeTemplate.
	StorageTarget providerconfig.ConfigVarString `json:"storageTarget,omitempty"`
	// OsImage describes the OS that will be installed on the VirtualMachine.
	OsImage providerconfig.ConfigVarString `json:"osImage,omitempty"`
	// Source describes the VM Disk Image source.
	Source providerconfig.ConfigVarString `json:"source,omitempty"`
	// PullMethod describes the VM Disk Image source optional pull method for registry source. Defaults to 'node'.
	PullMethod providerconfig.ConfigVarString `json:"pullMethod,omitempty"`
}

// SecondaryDisks.
type SecondaryDisks struct {
	Disk
}

// Disk.
type Disk struct {
	Size              providerconfig.ConfigVarString `json:"size,omitempty"`
	StorageClassName  providerconfig.ConfigVarString `json:"storageClassName,omitempty"`
	StorageAccessType providerconfig.ConfigVarString `json:"storageAccessType,omitempty"`
}

// Affinity.
type Affinity struct {
	// Deprecated: Use TopologySpreadConstraint instead.
	PodAffinityPreset providerconfig.ConfigVarString `json:"podAffinityPreset,omitempty"`
	// Deprecated: Use TopologySpreadConstraint instead.
	PodAntiAffinityPreset providerconfig.ConfigVarString `json:"podAntiAffinityPreset,omitempty"`
	NodeAffinityPreset    NodeAffinityPreset             `json:"nodeAffinityPreset,omitempty"`
}

// NodeAffinityPreset.
type NodeAffinityPreset struct {
	Type   providerconfig.ConfigVarString   `json:"type,omitempty"`
	Key    providerconfig.ConfigVarString   `json:"key,omitempty"`
	Values []providerconfig.ConfigVarString `json:"values,omitempty"`
}

// TopologySpreadConstraint describes topology spread constraints for VMs.
type TopologySpreadConstraint struct {
	// MaxSkew describes the degree to which VMs may be unevenly distributed.
	MaxSkew providerconfig.ConfigVarString `json:"maxSkew,omitempty"`
	// TopologyKey is the key of infra-node labels.
	TopologyKey providerconfig.ConfigVarString `json:"topologyKey,omitempty"`
	// WhenUnsatisfiable indicates how to deal with a VM if it doesn't satisfy
	// the spread constraint.
	WhenUnsatisfiable providerconfig.ConfigVarString `json:"whenUnsatisfiable,omitempty"`
}

// Location describes the region and zone where the machines are created at and where the deployed resources will reside.
type Location struct {
	Region string `json:"region,omitempty"`
	Zone   string `json:"zone,omitempty"`
}

// ProviderNetwork describes the infra cluster network fabric that is being used.
type ProviderNetwork struct {
	Name string `json:"name"`
	VPC  VPC    `json:"vpc"`
}

// VPC  is a virtual network dedicated to a single tenant within a KubeVirt, where the resources in the VPC
// is isolated from any other resources within the KubeVirt infra cluster.
type VPC struct {
	Name   string  `json:"name"`
	Subnet *Subnet `json:"subnet,omitempty"`
}

// Subnet a smaller, segmented portion of a larger network, like a Virtual Private Cloud (VPC).
type Subnet struct {
	Name string `json:"name"`
}

func GetConfig(pconfig providerconfig.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
