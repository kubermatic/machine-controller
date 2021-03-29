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

package ssh

import (
	"context"
	"strconv"
	"time"
)

type Server struct {
	Host HostConfig `json:"host"`
}

func (s Server) GetName() string {
	return s.Host.Hostname
}

func (s Server) GetID() string {
	return strconv.Itoa(s.Host.ID)
}

func (s Server) GetIPAddress() string {
	if s.Host.PublicAddress != "" {
		return s.Host.PublicAddress
	}

	if s.Host.PrivateAddress != "" {
		return s.Host.PrivateAddress
	}

	return ""
}

func (s Server) GetMACAddress() string {
	return s.Host.MacAddress
}

func (s Server) GetStatus() string {
	return s.Host.State
}

type MachineStatus string

const MachineProvisioned MachineStatus = "Provisioned"

// HostConfig describes a single control plane node.
type HostConfig struct {
	// ID automatically assigned at runtime.
	ID int `json:"-"`
	// PublicAddress is externally accessible IP address from public internet.
	PublicAddress string `json:"publicAddress"`
	// PrivateAddress is internal RFC-1918 IP address.
	PrivateAddress string `json:"privateAddress"`
	// SSHPort is port to connect ssh to.
	// Default value is 22.
	SSHPort int `json:"sshPort,omitempty"`
	// SSHUsername is system login name.
	// Default value is "root".
	SSHUsername string `json:"sshUsername,omitempty"`
	// SSHPrivateKeyFile is path to the file with PRIVATE AND CLEANTEXT ssh key.
	// Default value is "".
	SSHPrivateKeyFile string `json:"sshPrivateKeyFile,omitempty"`
	// Hostname is the hostname(1) of the host.
	// Default value is populated at the runtime via running `hostname -f` command over ssh.
	Hostname string `json:"hostname,omitempty"`
	// MacAddress is the mac address of the provisioned machine.
	MacAddress string `json:"macAddress"`
	// State represents the current state of the provisioned machine.
	State string `json:"state"`
	// CloudInitConfig is the cloud-init configuration which bootstrap and provision the server.
	CloudInitConfig string `json:"-"`
}

// Opts represents all the possible options for connecting to
// a remote server via SSH.
type Opts struct {
	Context    context.Context
	Username   string
	Password   string
	Hostname   string
	Port       int
	PrivateKey string
	KeyFile    string
	Timeout    time.Duration
}
