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

package provisioner

import (
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"

	v1 "k8s.io/api/core/v1"
)

const OutputFileName = "machines.json"

type output struct {
	Machines []machine `json:"machines"`
}

type machine struct {
	Name           string `json:"name"`
	ID             string `json:"id"`
	PublicAddress  string `json:"public_address,omitempty"`
	PrivateAddress string `json:"private_address,omitempty"`
	InternalDNS    string `json:"internal_dns,omitempty"`
	ExternalDNS    string `json:"external_dns,omitempty"`
	Hostname       string `json:"hostname,omitempty"`
	SSHUser        string `json:"ssh_user,omitempty"`
	Bastion        bool   `json:"bastion,omitempty"`
}

func getMachineProvisionerOutput(instances []instance.Instance) output {
	var out output

	for _, instance := range instances {
		machine := getMachineInfo(instance)
		out.Machines = append(out.Machines, machine)
	}
	return out
}

func getMachineInfo(inst instance.Instance) machine {
	var publicAddress, privateAddress, hostname, internalDNS, externalDNS string
	for address, addressType := range inst.Addresses() {
		if addressType == v1.NodeExternalIP {
			publicAddress = address
		} else if addressType == v1.NodeInternalIP {
			privateAddress = address
		} else if addressType == v1.NodeHostName {
			hostname = address
		} else if addressType == v1.NodeInternalDNS {
			internalDNS = address
		} else if addressType == v1.NodeExternalDNS {
			externalDNS = address
		}
	}

	return machine{
		Name:           inst.Name(),
		ID:             inst.ProviderID(),
		PublicAddress:  publicAddress,
		PrivateAddress: privateAddress,
		Hostname:       hostname,
		InternalDNS:    internalDNS,
		ExternalDNS:    externalDNS,
	}
}

func publicAndPrivateIPExist(addresses map[string]v1.NodeAddressType) bool {
	var publicIPExists, privateIPExists bool
	for _, addressType := range addresses {
		if addressType == v1.NodeExternalIP {
			publicIPExists = true
		} else if addressType == v1.NodeInternalIP {
			privateIPExists = true
		}
	}

	return publicIPExists && privateIPExists
}
