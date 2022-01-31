/*
Copyright 2021 The Machine Controller Authors.

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

package tinkerbell

import (
	"encoding/json"

	"github.com/tinkerbell/tink/pkg"

	"k8s.io/klog"
)

type HardwareSpec struct {
	Hardware pkg.HardwareWrapper `json:"hardware"`
}

func (h *HardwareSpec) GetName() string {
	return ""
}

func (h *HardwareSpec) GetID() string {
	return h.Hardware.Id
}

func (h *HardwareSpec) GetIPAddress() string {
	interfaces := h.Hardware.Network.Interfaces
	if len(interfaces) > 0 && interfaces[0].Dhcp.Ip != nil {
		return h.Hardware.Network.Interfaces[0].Dhcp.Ip.Address
	}

	return ""
}

func (h *HardwareSpec) GetMACAddress() string {
	if len(h.Hardware.Network.Interfaces) > 0 {
		return h.Hardware.Network.Interfaces[0].Dhcp.Mac
	}

	return ""
}

func (h *HardwareSpec) GetStatus() string {
	metadata := struct {
		State string `json:"state"`
	}{}

	if err := json.Unmarshal([]byte(h.Hardware.Metadata), &metadata); err != nil {
		klog.Errorf("failed to unmarshal hardware metadata: %v", err)
		return ""
	}

	return metadata.State
}
