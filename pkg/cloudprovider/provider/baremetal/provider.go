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

package baremetal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/ssh"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins"
	baremetaltypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

type bareMetalServer struct {
	server plugins.Server
}

func (b bareMetalServer) Name() string {
	return b.server.GetName()
}

func (b bareMetalServer) ID() string {
	return b.server.GetID()
}

func (b bareMetalServer) Addresses() map[string]corev1.NodeAddressType {
	return map[string]corev1.NodeAddressType{
		b.server.GetIPAddress(): corev1.NodeInternalIP,
	}
}

func (b bareMetalServer) Status() instance.Status {
	return instance.StatusRunning
}

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a new BareMetal provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{
		configVarResolver: configVarResolver,
	}
}

type Config struct {
	driver     plugins.PluginDriver
	driverName plugins.Driver
	driverSpec runtime.RawExtension
}

func (p *provider) getConfig(s v1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, *baremetaltypes.RawConfig, error) {
	if s.Value == nil {
		return nil, nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	pconfig := providerconfigtypes.Config{}
	err := json.Unmarshal(s.Value.Raw, &pconfig)
	if err != nil {
		return nil, nil, nil, err
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig := baremetaltypes.RawConfig{}
	if err := json.Unmarshal(pconfig.CloudProviderSpec.Raw, &rawConfig); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to unmarshal: %v", err)
	}
	c := Config{}
	driverName, err := p.configVarResolver.GetConfigVarStringValue(rawConfig.Driver)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get baremetal provider's driver name: %v", err)
	}
	c.driverName = plugins.Driver(driverName)

	switch c.driverName {
	case plugins.SSHDriver:
		c.driver = ssh.NewSSHDriver(context.Background())
	}

	c.driverSpec = rawConfig.DriverSpec
	return &c, &pconfig, &rawConfig, err
}

func (p provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	_, _, _, err := p.getConfig(spec.ProviderSpec)
	return spec, err
}

func (p provider) Validate(spec v1alpha1.MachineSpec) error {
	c, _, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	if c.driverName == "" {
		return fmt.Errorf("baremetal provider's driver name cannot be empty")
	}

	if c.driverSpec.Raw == nil {
		return fmt.Errorf("baremetal provider's driver spec cannot be empty")
	}

	return nil
}

func (p provider) Get(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	server, err := c.driver.GetServer(context.TODO(), machine.UID, c.driverSpec)
	if err != nil {
		if cloudprovidererrors.IsNotFound(err) {
			return nil, cloudprovidererrors.ErrInstanceNotFound
		}

		return nil, fmt.Errorf("failed to fetch server: %v", err)
	}

	return &bareMetalServer{
		server: server,
	}, nil
}

func (p provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p provider) Create(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	server, err := c.driver.ProvisionServer(context.TODO(), machine.UID, c.driverSpec, userdata)
	if err != nil {
		return nil, fmt.Errorf("failed to provisioner server: %v", err)
	}

	return &bareMetalServer{
		server: server,
	}, nil
}

func (p provider) Cleanup(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	_, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	return false, nil
}

func (p provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	return nil, nil
}

func (p provider) MigrateUID(machine *v1alpha1.Machine, new types.UID) error {
	return nil
}

func (p provider) SetMetricsForMachines(machines v1alpha1.MachineList) error {
	return nil
}
