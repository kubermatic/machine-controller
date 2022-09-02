/*
Copyright 2022 The Machine Controller Authors.

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

type RawConfig struct {
	Endpoint      providerconfigtypes.ConfigVarString `json:"endpoint"`
	UserID        providerconfigtypes.ConfigVarString `json:"userID"`
	Token         providerconfigtypes.ConfigVarString `json:"token"`
	AllowInsecure providerconfigtypes.ConfigVarBool   `json:"allowInsecure"`
	ProxyURL      providerconfigtypes.ConfigVarString `json:"proxyURL,omitempty"`

	CIStorageSSHPrivateKey providerconfigtypes.ConfigVarString `json:"ciStorageSSHPrivateKey"`
	CIStorageName          *string                             `json:"ciStorageName,omitempty"`
	CIStoragePath          *string                             `json:"ciStoragePath,omitempty"`

	VMTemplateID int     `json:"vmTemplateID"`
	CPUSockets   *int    `json:"cpuSockets"`
	CPUCores     *int    `json:"cpuCores,omitempty"`
	MemoryMB     int     `json:"memoryMB"`
	DiskSizeGB   *int    `json:"diskSizeGB,omitempty"`
	DiskName     *string `json:"diskName,omitempty"`
}

func GetConfig(pconfig providerconfigtypes.Config) (*RawConfig, error) {
	rawConfig := &RawConfig{}

	return rawConfig, jsonutil.StrictUnmarshal(pconfig.CloudProviderSpec.Raw, rawConfig)
}

// NodeList represents the response body of GET /api2/json/nodes.
type NodeList struct {
	Data []Node `json:"data"`
}

// Node is one single node in the response of GET /api2/json/nodes.
type Node struct {
	CPUCount        int     `json:"maxcpu,omitempty"`
	CPUUtilization  float64 `json:"cpu,omitempty"`
	MemoryAvailable int     `json:"maxmem,omitempty"`
	MemoryUsed      int     `json:"mem,omitempty"`
	Name            string  `json:"node"`
	SSLFingerprint  string  `json:"ssl_fingerprint,omitempty"`
	Status          string  `json:"status"`
	SupportLevel    string  `json:"level,omitempty"`
	Uptime          int     `json:"uptime"`
}

// NodeNetworkDeviceList represents the response body of GET /api2/json/nodes/<node>/network.
type NodeNetworkDeviceList struct {
	Data []NodeNetworkDevice `json:"data"`
}

// NodeNetworkDevice is one single network device of a node in the response of GET /api2/json/nodes/<node>/network.
type NodeNetworkDevice struct {
	Active      *int     `json:"active"`
	Address     *string  `json:"address"`
	Autostart   *int     `json:"autostart"`
	BridgeFD    *string  `json:"bridge_fd"`
	BridgePorts *string  `json:"bridge_ports"`
	BridgeSTP   *string  `json:"bridge_stp"`
	CIDR        *string  `json:"cidr"`
	Exists      *int     `json:"exists"`
	Families    []string `json:"families"`
	Gateway     *string  `json:"gateway"`
	Iface       string   `json:"iface"`
	MethodIPv4  *string  `json:"method"`
	MethodIPv6  *string  `json:"method6"`
	Netmask     *string  `json:"netmask"`
	Priority    int      `json:"priority"`
	Type        string   `json:"type"`
}
