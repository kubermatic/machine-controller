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
	v1 "k8s.io/api/core/v1"
)

const OutputFileName = "machines.json"

type output struct {
	Machines []machine `json:"machines"`
}

type machine struct {
	PublicAddress  string `json:"public_address,omitempty"`
	PrivateAddress string `json:"private_address,omitempty"`
	Hostname       string `json:"hostname,omitempty"`
	SSHUser        string `json:"ssh_user,omitempty"`
	Bastion        bool   `json:"bastion,omitempty"`
}

func getMachineProvisionerOutput(instances []MachineInstance) output {
	var out output

	for _, instance := range instances {
		machine := getMachineInfo(instance)
		out.Machines = append(out.Machines, machine)
	}
	return out
}

func getMachineInfo(instance MachineInstance) machine {
	var publicAddress, privateAddress, hostname string
	for address, addressType := range instance.inst.Addresses() {
		if addressType == v1.NodeExternalIP {
			publicAddress = address
		} else if addressType == v1.NodeInternalIP {
			privateAddress = address
		} else if addressType == v1.NodeHostName {
			hostname = address
		} else if addressType == v1.NodeInternalDNS {
			hostname = address
		}
	}

	return machine{
		PublicAddress:  publicAddress,
		PrivateAddress: privateAddress,
		Hostname:       hostname,
		SSHUser:        instance.sshUser,
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
