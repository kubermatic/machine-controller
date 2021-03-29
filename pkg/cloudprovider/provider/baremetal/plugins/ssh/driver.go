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
	"encoding/base64"
	"encoding/json"
	"fmt"

	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const machineStatusPath = "/var/lib/machine"

type sshDriver struct {
	connector *Connector
}

func NewSSHDriver(ctx context.Context) plugins.PluginDriver {
	return &sshDriver{
		connector: NewConnector(ctx),
	}
}

func (s *sshDriver) Validate(extension runtime.RawExtension) error {
	hostConfig := HostConfig{}
	if err := json.Unmarshal(extension.Raw, &hostConfig); err != nil {
		return fmt.Errorf("failed to unmarshal server config: %v", err)
	}

	if _, err := validateOptions(sshOpts(hostConfig)); err != nil {
		return fmt.Errorf("invalied host configs: %v", err)
	}

	return nil
}

func (s *sshDriver) GetServer(ctx context.Context, _ types.UID, extension runtime.RawExtension) (plugins.Server, error) {
	hostConfig := HostConfig{}
	if err := json.Unmarshal(extension.Raw, &hostConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal server config: %v", err)
	}

	conn, err := s.connector.Connect(hostConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection to server: %v", err)
	}

	out, _, _, err := conn.Exec(fmt.Sprintf("cat %s/status", machineStatusPath))
	if err != nil {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	if out == string(MachineProvisioned) {
		return &Server{
			Host: hostConfig,
		}, nil
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (s *sshDriver) ProvisionServer(ctx context.Context, _ types.UID, extension runtime.RawExtension, userdata string) (plugins.Server, error) {
	hostConfig := HostConfig{}
	if err := json.Unmarshal(extension.Raw, &hostConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal server config: %v", err)
	}

	conn, err := s.connector.Connect(hostConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection to server: %v", err)
	}

	cloudInit := base64.StdEncoding.EncodeToString([]byte(userdata))
	cloudInitPath := "/etc/cloud/cloud.cfg.d"

	cmds := []string{
		fmt.Sprintf("echo %v > %s/encoded-cloud-config", cloudInit, cloudInitPath),
		fmt.Sprintf("base64 --decode %s/encoded-cloud-config > %s/99-cloud-config.cfg", cloudInitPath, cloudInitPath),
		fmt.Sprintf("mkdir -p %s && touch %s/status", machineStatusPath, machineStatusPath),
		fmt.Sprintf("echo Provisioned > %s/status", machineStatusPath),
		"cloud-init clean --reboot",
	}

	if err := executeCMDs(conn, cmds); err != nil {
		return nil, fmt.Errorf("failed to execute provisioning commands: %v", err)
	}

	return &Server{
		Host: hostConfig,
	}, nil
}

func (s *sshDriver) DeprovisionServer(uid types.UID, extension runtime.RawExtension) (string, error) {
	// TODO: find a good way to de-provision machines
	return "", nil
}

func executeCMDs(conn Connection, cmds []string) error {
	for _, cmd := range cmds {
		_, _, osCode, err := conn.Exec(cmd)
		if err != nil {
			return fmt.Errorf("failed to execute command: %v", err)
		}

		if osCode != 0 {
			return fmt.Errorf("failed to execute command, osCode=%v", osCode)
		}
	}

	return nil
}
