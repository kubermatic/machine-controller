/*
Copyright 2024 The Machine Controller Authors.

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
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
)

type Hardware struct {
	*tinkv1alpha1.Hardware `json:"hardware"`
}

var _ plugins.Server = &Hardware{}

func (h *Hardware) GetName() string {
	return h.Name
}

func (h *Hardware) GetID() string {
	return h.Spec.Metadata.Instance.ID
}

func (h *Hardware) GetIPAddress() string {
	interfaces := h.Spec.Interfaces
	if len(interfaces) > 0 && interfaces[0].DHCP.IP != nil {
		return interfaces[0].DHCP.IP.Address
	}

	return ""
}

func (h *Hardware) GetMACAddress() string {
	if len(h.Spec.Interfaces) > 0 {
		return h.Spec.Interfaces[0].DHCP.MAC
	}

	return ""
}

func (h *Hardware) GetStatus() string {
	if h.Status.State != "" {
		return string(h.Status.State)
	}

	return "Unknown"
}
