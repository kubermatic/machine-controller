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

package vsphere

import (
	"k8c.io/machine-controller/sdk/jsonutil"
	"k8c.io/machine-controller/sdk/providerconfig"
)

// RawConfig represents vsphere specific configuration.
type RawConfig struct {
	TemplateVMName providerconfig.ConfigVarString `json:"templateVMName"`
	// Deprecated: use networks instead.
	VMNetName  providerconfig.ConfigVarString   `json:"vmNetName"`
	Networks   []providerconfig.ConfigVarString `json:"networks"`
	Username   providerconfig.ConfigVarString   `json:"username"`
	Password   providerconfig.ConfigVarString   `json:"password"`
	VSphereURL providerconfig.ConfigVarString   `json:"vsphereURL"`
	Datacenter providerconfig.ConfigVarString   `json:"datacenter"`

	// Cluster defines the cluster to use in vcenter.
	// Only needed for vm anti affinity.
	Cluster providerconfig.ConfigVarString `json:"cluster"`

	Folder       providerconfig.ConfigVarString `json:"folder"`
	ResourcePool providerconfig.ConfigVarString `json:"resourcePool"`

	// Either Datastore or DatastoreCluster have to be provided.
	DatastoreCluster providerconfig.ConfigVarString `json:"datastoreCluster"`
	Datastore        providerconfig.ConfigVarString `json:"datastore"`

	CPUs          int32                        `json:"cpus"`
	MemoryMB      int64                        `json:"memoryMB"`
	DiskSizeGB    *int64                       `json:"diskSizeGB,omitempty"`
	Tags          []Tag                        `json:"tags,omitempty"`
	AllowInsecure providerconfig.ConfigVarBool `json:"allowInsecure"`

	// Placement rules
	VMAntiAffinity providerconfig.ConfigVarBool   `json:"vmAntiAffinity"`
	VMGroup        providerconfig.ConfigVarString `json:"vmGroup,omitempty"`
}

// Tag represents vsphere tag.
type Tag struct {
	Description string `json:"description,omitempty"`
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	CategoryID  string `json:"categoryID"`
}

func GetConfig(pconfig providerconfig.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}
