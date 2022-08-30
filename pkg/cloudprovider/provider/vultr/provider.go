/*
Copyright 2019 The Machine Controller Authors.

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
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	vultrtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/vultr/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/vultr/govultr/v2"
	"golang.org/x/oauth2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type Config struct {
	Token       string
	MachineType string
	Region      string
	Plan        string
	Tags        []string
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

	c.Token, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Token, "VULTR_TOKEN")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"token\" field, error = %w", err)
	}

	c.MachineType, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.MachineType)
	if err != nil {
		return nil, nil, err
	}

	c.Region, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Region)
	if err != nil {
		return nil, nil, err
	}

	c.Plan, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Plan)
	if err != nil {
		return nil, nil, err
	}

	for _, tag := range rawConfig.Tags {
		tagVal, err := p.configVarResolver.GetConfigVarStringValue(tag)
		if err != nil {
			return nil, nil, err
		}
		c.Tags = append(c.Tags, tagVal)
	}

	return &c, pconfig, err
}

func getClient(token string) *govultr.Client {
	config := &oauth2.Config{}
	ctx := context.Background()
	ts := config.TokenSource(ctx, &oauth2.Token{AccessToken: token})

	return govultr.NewClient(oauth2.NewClient(ctx, ts))
}

// getOdIs, vultr works with OS IDs (instead of names)
// The IDs hardcoded here come from https://api.vultr.com/v2/os
func getOsId(os providerconfigtypes.OperatingSystem) (int, error) {
	switch os {
	case providerconfigtypes.OperatingSystemUbuntu:
		return 387, nil
	}

	return 0, providerconfigtypes.ErrOSNotSupported
}

func (p *provider) Validate(ctx context.Context, spec clusterv1alpha1.MachineSpec) error {
	c, pc, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if c.Token == "" {
		return errors.New("token is missing")
	}

	if c.MachineType != string(vultrtypes.BareMetal) && c.MachineType != string(vultrtypes.CloudInstance) {
		return fmt.Errorf("invalid machineType %q, valid values are %q, %q", c.MachineType, vultrtypes.BareMetal, vultrtypes.CloudInstance)
	}

	osId, err := getOsId(pc.OperatingSystem)
	if err != nil {
		return fmt.Errorf("invalid/not supported operating system specified %q: %w", pc.OperatingSystem, err)
	}

	client := getClient(c.Token)

	var foundRegion bool
	regions, _, err := client.Region.List(ctx, nil)
	if err != nil {
		return err
	}
	for _, r := range regions {
		if r.ID == c.Region {
			foundRegion = true
			break
		}
	}
	if !foundRegion {
		return fmt.Errorf("region %q not found", c.Region)
	}

	var foundAvailablePlan bool
	if c.MachineType == string(vultrtypes.CloudInstance) {
		planAvailability, err := client.Region.Availability(ctx, c.Region, "all")
		if err != nil {
			return err
		}
		for _, p := range planAvailability.AvailablePlans {
			if p == c.Plan {
				foundAvailablePlan = true
				break
			}
		}
		if !foundAvailablePlan {
			return fmt.Errorf("plan %q not available on region %q", c.Plan, c.Region)
		}
	} else if c.MachineType == string(vultrtypes.BareMetal) {
		planAvailability, _, err := client.Plan.ListBareMetal(ctx, nil)
		if err != nil {
			return err
		}

		var foundPlanRegion bool
		for _, p := range planAvailability {
			if p.ID == c.Plan {
				for _, r := range p.Locations {
					if r == c.Region {
						foundPlanRegion = true
					}
				}
			}
		}
		if !foundPlanRegion {
			return fmt.Errorf("plan %q not available on region %q", c.Plan, c.Region)
		}
	}

	var foundOperatingSystem bool
	oss, _, err := client.OS.List(ctx, nil)
	if err != nil {
		return err
	}

	for _, os := range oss {
		if os.ID == osId {
			foundOperatingSystem = true
			break
		}
	}
	if !foundOperatingSystem {
		return fmt.Errorf("operating system with ID %q not found", osId)
	}

	return nil
}

func (p *provider) Create(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userData string) (instance.Instance, error) {
	c, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c.Token)

	osId, _ := getOsId(pc.OperatingSystem)

	c.Tags = append(c.Tags, fmt.Sprintf("machineUid=%s", machine.UID))

	if c.MachineType == string(vultrtypes.BareMetal) {
		instanceOpts := govultr.BareMetalCreate{
			Label:    machine.Spec.Name,
			Region:   c.Region,
			Plan:     c.Plan,
			OsID:     osId,
			Tags:     c.Tags,
			UserData: base64.StdEncoding.EncodeToString([]byte(userData)),
		}

		instance, err := client.BareMetalServer.Create(ctx, &instanceOpts)
		if err != nil {
			return nil, fmt.Errorf("could not create bare-metal instance: %q", err)
		}

		return &vultrBareMetalInstance{instance: instance}, nil
	} else if c.MachineType == string(vultrtypes.CloudInstance) {
		instanceOpts := govultr.InstanceCreateReq{
			Label:    machine.Spec.Name,
			Region:   c.Region,
			Plan:     c.Plan,
			OsID:     osId,
			Tags:     c.Tags,
			UserData: base64.StdEncoding.EncodeToString([]byte(userData)),
		}

		instance, err := client.Instance.Create(ctx, &instanceOpts)
		if err != nil {
			return nil, fmt.Errorf("could not create cloud-instance instance: %q", err)
		}

		return &vultrCloudInstance{instance: instance}, nil
	}

	return nil, fmt.Errorf("could not create instance: %q", err)
}

func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
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

	client := getClient(c.Token)

	if c.MachineType == string(vultrtypes.BareMetal) {
		err := client.BareMetalServer.Delete(ctx, instance.ID())
		if err != nil {
			return false, cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: err.Error(),
			}
		}

		return false, nil

	} else if c.MachineType == string(vultrtypes.CloudInstance) {
		err := client.Instance.Delete(ctx, instance.ID())
		if err != nil {
			return false, cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: err.Error(),
			}
		}

		return false, nil
	}

	return false, nil
}

func (p *provider) Get(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c.Token)

	if c.MachineType == string(vultrtypes.BareMetal) {
		instances, _, err := client.BareMetalServer.List(ctx, &govultr.ListOptions{Tag: fmt.Sprintf("machineUid=%s", machine.UID)})
		if err != nil {
			return nil, cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: err.Error(),
			}
		}

		for _, instance := range instances {
			for _, tag := range instance.Tags {
				if tag == fmt.Sprintf("machineUid=%s", machine.UID) {
					return &vultrBareMetalInstance{instance: &instance}, nil
				}
			}
		}
	} else if c.MachineType == string(vultrtypes.CloudInstance) {
		instances, _, err := client.Instance.List(ctx, nil)
		if err != nil {
			return nil, cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: err.Error(),
			}
		}

		for _, instance := range instances {
			for _, tag := range instance.Tags {
				if tag == fmt.Sprintf("machineUid=%s", machine.UID) {
					return &vultrCloudInstance{instance: &instance}, nil
				}
			}
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) MigrateUID(ctx context.Context, machine *clusterv1alpha1.Machine, newUID types.UID) error {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	instance, err := p.Get(ctx, machine, nil)
	if err != nil {
		if errors.Is(err, cloudprovidererrors.ErrInstanceNotFound) {
			return err
		}
	}

	client := getClient(c.Token)

	if c.MachineType == string(vultrtypes.BareMetal) {
		rawInstance, err := client.BareMetalServer.Get(ctx, instance.ID())
		if err != nil {
			return fmt.Errorf("could not get instance with id %q", instance.ID())
		}

		var tagFound bool
		for i, t := range rawInstance.Tags {
			if strings.HasPrefix(t, "machineUid") {
				tagFound = true
				rawInstance.Tags[i] = fmt.Sprintf("machineUid=%s", newUID)
			}
		}

		if !tagFound {
			return fmt.Errorf("could not find instance with old machineUid tag")
		}

		_, err = client.BareMetalServer.Update(ctx, instance.ID(), &govultr.BareMetalUpdate{Tags: rawInstance.Tags})
		if err != nil {
			return fmt.Errorf("failed to update instance with new UID: %w", err)
		}
	} else if c.MachineType == string(vultrtypes.CloudInstance) {
		rawInstance, err := client.Instance.Get(ctx, instance.ID())
		if err != nil {
			return fmt.Errorf("could not get instance with id %q", instance.ID())
		}

		var tagFound bool
		for i, t := range rawInstance.Tags {
			if strings.HasPrefix(t, "machineUid") {
				tagFound = true
				rawInstance.Tags[i] = fmt.Sprintf("machineUid=%s", newUID)
			}
		}

		if !tagFound {
			return fmt.Errorf("could not find instance with old machineUid tag")
		}

		_, err = client.Instance.Update(ctx, instance.ID(), &govultr.InstanceUpdateReq{Tags: rawInstance.Tags})
		if err != nil {
			return fmt.Errorf("failed to update instance with new UID: %w", err)
		}
	}

	return nil
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["region"] = c.Region
		labels["plan"] = c.Plan
	}

	return labels, err
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	return nil
}

type vultrBareMetalInstance struct {
	instance *govultr.BareMetalServer
}

func (v *vultrBareMetalInstance) Name() string {
	return string(v.instance.Label)
}

func (v *vultrBareMetalInstance) ID() string {
	return v.instance.ID
}

func (v *vultrBareMetalInstance) ProviderID() string {
	return fmt.Sprintf("vultr://%s", v.instance.ID)
}

func (v *vultrBareMetalInstance) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{}
	addresses[v.instance.MainIP] = v1.NodeExternalIP

	return addresses
}

func (v *vultrBareMetalInstance) Status() instance.Status {
	switch v.instance.Status {
	case "pending":
		return instance.StatusCreating
	case "active":
		return instance.StatusRunning
	default:
		return instance.StatusUnknown
	}
}

type vultrCloudInstance struct {
	instance *govultr.Instance
}

func (v *vultrCloudInstance) Name() string {
	return string(v.instance.Label)
}

func (v *vultrCloudInstance) ID() string {
	return v.instance.ID
}

func (v *vultrCloudInstance) ProviderID() string {
	return fmt.Sprintf("vultr://%s", v.instance.ID)
}

func (v *vultrCloudInstance) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{}
	addresses[v.instance.MainIP] = v1.NodeExternalIP

	return addresses
}

func (v *vultrCloudInstance) Status() instance.Status {
	switch v.instance.Status {
	case "pending":
		return instance.StatusCreating
	case "active":
		return instance.StatusRunning
	default:
		return instance.StatusUnknown
	}
}
