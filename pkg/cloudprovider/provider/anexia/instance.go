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
	"net"

	"go.anx.io/go-anxcloud/pkg/vsphere/info"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	anxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"

	v1 "k8s.io/api/core/v1"
)

type anexiaInstance struct {
	isCreating        bool
	info              *info.Info
	reservedAddresses []string
}

func (ai *anexiaInstance) Name() string {
	if ai.info == nil {
		return ""
	}

	return ai.info.Name
}

func (ai *anexiaInstance) ID() string {
	if ai.info == nil {
		return ""
	}

	return ai.info.Identifier
}

func (ai *anexiaInstance) ProviderID() string {
	return ai.ID()
}

func (ai *anexiaInstance) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{}

	if ai.reservedAddresses != nil {
		for _, reservedIP := range ai.reservedAddresses {
			addresses[reservedIP] = v1.NodeExternalIP
		}
	}

	if ai.info != nil {
		for _, network := range ai.info.Network {
			for _, ip := range network.IPv4 {
				addresses[ip] = v1.NodeExternalIP
			}
			for _, ip := range network.IPv6 {
				addresses[ip] = v1.NodeExternalIP
			}
		}
	}

	for ip := range addresses {
		parsed := net.ParseIP(ip)
		if parsed.IsPrivate() {
			addresses[ip] = v1.NodeInternalIP
		} else {
			addresses[ip] = v1.NodeExternalIP
		}
	}

	return addresses
}

func (ai *anexiaInstance) Status() instance.Status {
	if ai.isCreating {
		return instance.StatusCreating
	}

	if ai.info != nil {
		if ai.info.Status == anxtypes.MachinePoweredOn {
			return instance.StatusRunning
		}
	}
	return instance.StatusUnknown
}
