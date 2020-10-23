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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	anexiatypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	anxcloud "github.com/anexia-it/go-anxcloud/pkg/client"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
)

const (
	machineUIDLabelKey = "machine-uid"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns an Anexia provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	klog.Infoln("anexia provider loaded")
	return &provider{configVarResolver: configVarResolver}
}

type Config struct {
	Token      string
	VlanID     string
	LocationID string
	TemplateID string
	Cpus       int
	Memory     int
	DiskSize   int
	SSHKey     string
}

func getClient() (anxcloud.Client, error) {
	client, err := anxcloud.NewAnyClientFromEnvs(true, nil)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (p *provider) getConfig(s v1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, error) {
	if s.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerSpec.value is nil")
	}
	pconfig := providerconfigtypes.Config{}
	err := json.Unmarshal(s.Value.Raw, &pconfig)
	if err != nil {
		return nil, nil, err
	}

	rawConfig := anexiatypes.RawConfig{}
	if err = json.Unmarshal(pconfig.CloudProviderSpec.Raw, &rawConfig); err != nil {
		return nil, nil, err
	}

	c := Config{}
	c.Token, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Token, "ANX_TOKEN")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"token\" field, error = %v", err)
	}

	c.Cpus = rawConfig.Cpus
	c.Memory = rawConfig.Memory
	c.DiskSize = rawConfig.DiskSize

	c.LocationID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.LocationID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"locationID\" field, error = %v", err)
	}

	c.TemplateID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.TemplateID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"templateID\" field, error = %v", err)
	}

	c.VlanID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VlanID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"vlanID\" field, error = %v", err)
	}

	fmt.Printf("Parsed machine config:\npconfig: %+v\nc: %+v\n", pconfig, c)

	return &c, &pconfig, nil
}

// AddDefaults adds omitted optional values to the given MachineSpec
func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	klog.Infoln("anexia provider.AddDefaults(%+v)", spec)
	return spec, nil
}

// Validate returns success or failure based according to its FakeCloudProviderSpec
func (p *provider) Validate(machinespec v1alpha1.MachineSpec) error {
	klog.Infoln("anexia provider.Validate(%+v)", machinespec)
	config, _, err := p.getConfig(machinespec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if config.Token == "" {
		return errors.New("token is missing")
	}

	if config.Cpus == 0 {
		return errors.New("cpu count is missing")
	}

	if config.DiskSize == 0 {
		return errors.New("disk size is missing")
	}

	if config.Memory == 0 {
		return errors.New("memory size is missing")
	}

	if config.LocationID == "" {
		return errors.New("location id is missing")
	}

	if config.TemplateID == "" {
		return errors.New("template id is missing")
	}

	if config.VlanID == "" {
		return errors.New("vlan id is missing")
	}

	return nil
}

func (p *provider) Get(machine *v1alpha1.Machine, provider *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	klog.Infoln("anexia provider.Get(machine, provider)")

	client, err := getClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	return GetFromAnexia(
		ctx,
		machine.ObjectMeta.Name,
		client,
	)
}

func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (string, string, error) {
	klog.Infoln("anexia provider.GetCloudConfig(spec)")
	return "", "", nil
}

// Create creates a cloud instance according to the given machine
func (p *provider) Create(machine *v1alpha1.Machine, providerData *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	klog.Infoln("anexia provider.Create(machine, providerData, userdata)")
	klog.Infoln(userdata)

	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := getClient()
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to create anexia api-client, due to %v", err),
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return CreateAnexiaVM(
		ctx,
		config,
		machine.ObjectMeta.Name,
		userdata,
		client,
	)
}

func (p *provider) Cleanup(machine *v1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (bool, error) {
	klog.Infoln("anexia provider.Cleanup(machine)")

	client, err := getClient()
	if err != nil {
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	err = Remove(ctx, machine.ObjectMeta.Name, client)
	if err != nil {
		return false, fmt.Errorf("could not deprovision machine: %w", err)
	}

	return true, nil
}

func (p *provider) MigrateUID(_ *v1alpha1.Machine, _ types.UID) error {
	klog.Infoln("anexia provider.MigrateUID")
	return nil
}

func (p *provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	klog.Infoln("anexia provider.MachineMetricsLabels(machine)")
	return map[string]string{}, nil
}

func (p *provider) SetMetricsForMachines(machine v1alpha1.MachineList) error {
	klog.Infoln("anexia provider.SetMetricsForMachines(machine)")
	return nil
}
