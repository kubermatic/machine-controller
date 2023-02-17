/*
Copyright 2023 The Machine Controller Authors.

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

package vultr

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/vultr/govultr/v2"
	"golang.org/x/oauth2"

	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	vultrtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/vultr/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a new vultr provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type Config struct {
	APIKey string
	Region string
	Plan   string
	OsID   string
	Tags   []string
}

func getIDForOS(os providerconfigtypes.OperatingSystem) (int, error) {
	switch os {
	case providerconfigtypes.OperatingSystemUbuntu:
		return 1743, nil
		// name: CentOS 7 x64
	case providerconfigtypes.OperatingSystemCentOS:
		return 167, nil
		// name: Rocky Linux 9 x64
	case providerconfigtypes.OperatingSystemRockyLinux:
		return 1869, nil
	}
	return 0, providerconfigtypes.ErrOSNotSupported
}

func getClient(ctx context.Context, apiKey string) *govultr.Client {
	config := &oauth2.Config{}
	ts := config.TokenSource(ctx, &oauth2.Token{AccessToken: apiKey})
	return govultr.NewClient(oauth2.NewClient(ctx, ts))
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

	rawConfig, err := vultrtypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, err
	}

	c := Config{}
	c.APIKey, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.APIKey, "VULTR_API_KEY")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"apiKey\" field, error = %w", err)
	}

	c.Plan, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Plan)
	if err != nil {
		return nil, nil, err
	}

	c.Region, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Region)
	if err != nil {
		return nil, nil, err
	}

	c.OsID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.OsID)
	if err != nil {
		return nil, nil, err
	}

	c.Tags = rawConfig.Tags

	return &c, pconfig, err
}

func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) Validate(ctx context.Context, spec clusterv1alpha1.MachineSpec) error {
	c, pc, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if c.APIKey == "" {
		return errors.New("apiKey is missing")
	}

	if c.Region == "" {
		return errors.New("region is missing")
	}

	if c.Plan == "" {
		return errors.New("plan is missing")
	}

	if c.OsID == "" {
		return errors.New("osID is missing")
	}

	_, err = getIDForOS(pc.OperatingSystem)
	if err != nil {
		return fmt.Errorf("invalid/not supported operating system specified %q: %w", pc.OperatingSystem, err)
	}

	client := getClient(ctx, c.APIKey)

	plans, err := client.Region.Availability(ctx, c.Region, "")

	// TODO: Validate region separately
	if err != nil {
		return fmt.Errorf("invalid/not supported region specified %q: %w", c.Region, err)
	}

	planFound := false

	// Check if given plan present in the returned list
	for _, plan := range plans.AvailablePlans {
		if plan == c.Plan {
			planFound = true
		}
	}
	if !planFound {
		return fmt.Errorf("invalid/not supported plan specified %q: %w", c.Plan, err)
	}
	return nil
}

func (p *provider) get(ctx context.Context, machine *clusterv1alpha1.Machine) (*vultrInstance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(ctx, c.APIKey)

	instances, _, err := client.Instance.List(ctx, &govultr.ListOptions{
		Tag: string(machine.UID),
	})
	if err != nil {
		return nil, vltErrorToTerminalError(err, "failed to list servers")
	}

	for _, instance := range instances {
		for _, tag := range instance.Tags {
			if tag == string(machine.UID) {
				return &vultrInstance{instance: &instance}, nil
			}
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) Get(ctx context.Context, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	return p.get(ctx, machine)
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) Create(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(ctx, c.APIKey)

	if c.OsID == "" {
		osID, err := getIDForOS(pc.OperatingSystem)
		if err != nil {
			return nil, cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: fmt.Sprintf("Invalid operating system specified %q, details = %v", pc.OperatingSystem, err),
			}
		}
		c.OsID = strconv.Itoa(osID)
	}

	if c.Tags == nil {
		c.Tags = []string{}
	}

	c.Tags = append(c.Tags, string(machine.UID))

	strOsID, err := strconv.Atoi(c.OsID)
	if err != nil {
		return nil, err
	}

	instanceCreateRequest := govultr.InstanceCreateReq{
		Region:   c.Region,
		Plan:     c.Plan,
		Label:    machine.Spec.Name,
		UserData: userdata,
		Tags:     c.Tags,
		OsID:     strOsID,
	}

	res, err := client.Instance.Create(ctx, &instanceCreateRequest)
	if err != nil {
		return nil, vltErrorToTerminalError(err, "failed to create server")
	}

	return &vultrInstance{instance: res}, nil
}

func (p *provider) Cleanup(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	instance, err := p.Get(ctx, machine, data)
	if err != nil {
		if errors.Is(err, cloudprovidererrors.ErrInstanceNotFound) {
			return true, nil
		}
		return false, err
	}

	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}
	client := getClient(ctx, c.APIKey)

	if err = client.Instance.Delete(ctx, instance.ID()); err != nil {
		return false, vltErrorToTerminalError(err, "failed to delete server")
	}

	return false, nil
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["plan"] = c.Plan
		labels["region"] = c.Region
	}

	return labels, err
}

func (p *provider) MigrateUID(ctx context.Context, machine *clusterv1alpha1.Machine, newUID types.UID) error {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to decode providerconfig: %w", err)
	}
	client := getClient(ctx, c.APIKey)
	instances, _, err := client.Instance.List(ctx, &govultr.ListOptions{PerPage: 1000})
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	for _, instance := range instances {
		if instance.Label == machine.Spec.Name && sets.NewString(instance.Tags...).Has(string(machine.UID)) {
			_, err = client.Instance.Update(ctx, instance.ID, &govultr.InstanceUpdateReq{
				Tags: sets.NewString(instance.Tags...).Delete(string(machine.UID)).Insert(string(newUID)).List(),
			})
			if err != nil {
				return fmt.Errorf("failed to tag instance with new UID tag: %w", err)
			}
		}
	}

	return nil
}

type vultrInstance struct {
	instance *govultr.Instance
}

func (v *vultrInstance) Name() string {
	return v.instance.Label
}

func (v *vultrInstance) ID() string {
	return v.instance.ID
}

func (v *vultrInstance) ProviderID() string {
	return "vultr://" + v.instance.ID
}

func (v *vultrInstance) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{}
	addresses[v.instance.MainIP] = v1.NodeExternalIP
	addresses[v.instance.InternalIP] = v1.NodeInternalIP
	return addresses
}

func (v *vultrInstance) Status() instance.Status {
	switch v.instance.Status {
	case "active":
		return instance.StatusRunning
	case "pending":
		return instance.StatusCreating
		// "suspending" or "resizing"
	default:
		return instance.StatusUnknown
	}
}

func vltErrorToTerminalError(err error, msg string) error {
	prepareAndReturnError := func() error {
		return fmt.Errorf("%s, due to %w", msg, err)
	}
	if err != nil {
		return prepareAndReturnError()
	}
	return err
}

func (p *provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	return nil
}
