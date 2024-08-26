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

	"go.uber.org/zap"

	"k8c.io/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "k8c.io/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "k8c.io/machine-controller/pkg/cloudprovider/errors"
	"k8c.io/machine-controller/pkg/cloudprovider/instance"
	"k8c.io/machine-controller/pkg/cloudprovider/provider/baremetal/plugins"
	tink "k8c.io/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell"
	tinktypes "k8c.io/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/types"
	baremetaltypes "k8c.io/machine-controller/pkg/cloudprovider/provider/baremetal/types"
	cloudprovidertypes "k8c.io/machine-controller/pkg/cloudprovider/types"
	"k8c.io/machine-controller/pkg/cloudprovider/util"
	"k8c.io/machine-controller/pkg/providerconfig"
	providerconfigtypes "k8c.io/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
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

// TODO: Tinkerbell doesn't have a CCM.
func (b bareMetalServer) ProviderID() string {
	return ""
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

// New returns a new BareMetal provider.
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

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, error) {
	if provSpec.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, err
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig, err := baremetaltypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	c := Config{}

	driverName, err := p.configVarResolver.GetConfigVarStringValue(rawConfig.Driver)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get baremetal provider's driver name: %w", err)
	}
	c.driverName = plugins.Driver(driverName)

	c.driverSpec = rawConfig.DriverSpec

	switch c.driverName {
	case plugins.Tinkerbell:
		driverConfig := &tinktypes.TinkerbellPluginSpec{}

		if err := json.Unmarshal(c.driverSpec.Raw, &driverConfig); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal tinkerbell driver spec: %w", err)
		}

		tinkConfig, err := tink.GetConfig(*driverConfig, p.configVarResolver.GetConfigVarStringValueOrEnv)

		if err != nil {
			return nil, nil, err
		}

		c.driver, err = tink.NewTinkerbellDriver(*tinkConfig, driverConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create a tinkerbell driver: %w", err)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported baremetal driver: %s", pconfig.CloudProvider)
	}

	return &c, pconfig, err
}

func (p provider) AddDefaults(_ *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	_, _, err := p.getConfig(spec.ProviderSpec)
	return spec, err
}

func (p provider) Validate(_ context.Context, _ *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) error {
	c, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if c.driverName == "" {
		return fmt.Errorf("baremetal provider's driver name cannot be empty")
	}

	if c.driverSpec.Raw == nil {
		return fmt.Errorf("baremetal provider's driver spec cannot be empty")
	}

	return nil
}

func (p provider) Get(ctx context.Context, _ *zap.SugaredLogger, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	server, err := c.driver.GetServer(ctx)
	if err != nil {
		if errors.Is(err, cloudprovidererrors.ErrInstanceNotFound) {
			return nil, cloudprovidererrors.ErrInstanceNotFound
		}

		return nil, fmt.Errorf("failed to fetch server with the id %s: %w", machine.Name, err)
	}

	return &bareMetalServer{
		server: server,
	}, nil
}

func (p provider) GetCloudConfig(_ clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p provider) Create(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	server, err := c.driver.ProvisionServer(ctx, log, machine.ObjectMeta, c.driverSpec, userdata)
	if err != nil {
		return nil, fmt.Errorf("failed to provision server: %w", err)
	}

	return &bareMetalServer{
		server: server,
	}, nil
}

func (p provider) Cleanup(ctx context.Context, _ *zap.SugaredLogger, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	if err := c.driver.DeprovisionServer(ctx); err != nil {
		return false, fmt.Errorf("failed to de-provision server: %w", err)
	}

	secret := &corev1.Secret{}
	if err := data.Client.Get(ctx, types.NamespacedName{Namespace: util.CloudInitNamespace, Name: machine.Name}, secret); err != nil {
		if !kerrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to fetching secret for userdata: %w", err)
		}

		return true, nil
	}

	if err := data.Client.Delete(ctx, secret); err != nil {
		return false, fmt.Errorf("failed to cleanup secret for userdata: %w", err)
	}

	return true, nil
}

func (p provider) MachineMetricsLabels(_ *clusterv1alpha1.Machine) (map[string]string, error) {
	return nil, nil
}

func (p provider) MigrateUID(_ context.Context, _ *zap.SugaredLogger, _ *clusterv1alpha1.Machine, _ types.UID) error {
	return nil
}

func (p provider) SetMetricsForMachines(_ clusterv1alpha1.MachineList) error {
	return nil
}
