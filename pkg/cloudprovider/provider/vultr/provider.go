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
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/vultr/govultr/v3"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	vultrtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/vultr/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	createCheckPeriod           = 10 * time.Second
	createCheckTimeout          = 5 * time.Minute
	createCheckFailedWaitPeriod = 10 * time.Second
)

type ValidVPC struct {
	IsAllValid  bool
	InvalidVpcs []string
}

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a new vultr provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type Config struct {
	PhysicalMachine bool
	APIKey          string
	Region          string
	Plan            string
	OsID            string
	Tags            []string
	VpcID           []string
	EnableVPC       bool
	EnableIPv6      bool
	EnableVPC2      bool
	Vpc2ID          []string
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
	c.PhysicalMachine = rawConfig.PhysicalMachine
	c.EnableIPv6 = rawConfig.EnableIPv6
	c.VpcID = rawConfig.VpcID
	c.EnableVPC = rawConfig.EnableVPC
	c.EnableVPC2 = rawConfig.EnableVPC2
	c.Vpc2ID = rawConfig.Vpc2ID

	return &c, pconfig, err
}

func (p *provider) AddDefaults(_ *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) validateVpc(ctx context.Context, client *govultr.Client, c *Config, legacyVPC bool) (ValidVPC, error) {
	validVpc := ValidVPC{IsAllValid: true}
	accountvpcs := []string{}
	var requestedvpcs []string

	if legacyVPC {
		for {
			vpcs, meta, err := func(ctx context.Context, client *govultr.Client) ([]govultr.VPC, *govultr.Meta, error) {
				vpcs, meta, resp, err := client.VPC.List(ctx, &govultr.ListOptions{})
				if err != nil {
					return nil, nil, vltErrorToTerminalError(resp.StatusCode, err)
				}
				defer resp.Body.Close()

				return vpcs, meta, nil
			}(ctx, client)
			if err != nil {
				return validVpc, err
			}
			for _, v := range vpcs {
				accountvpcs = append(accountvpcs, v.ID)
			}
			if meta.Links.Next == "" {
				break
			}
		}
		requestedvpcs = c.VpcID
	} else {
		for {
			vpcs, meta, err := func(ctx context.Context, client *govultr.Client) ([]govultr.VPC2, *govultr.Meta, error) {
				vpcs, meta, resp, err := client.VPC2.List(ctx, &govultr.ListOptions{})
				if err != nil {
					return nil, nil, vltErrorToTerminalError(resp.StatusCode, err)
				}
				defer resp.Body.Close()

				return vpcs, meta, nil
			}(ctx, client)
			if err != nil {
				return validVpc, err
			}
			for _, v := range vpcs {
				accountvpcs = append(accountvpcs, v.ID)
			}
			if meta.Links.Next == "" {
				break
			}
		}
		requestedvpcs = c.Vpc2ID
	}
	accountvpcsset := sets.New[string](accountvpcs...)
	// Iterator to provide user the exact mismatches
	for _, v := range requestedvpcs {
		if !accountvpcsset.Has(v) {
			validVpc.IsAllValid = false
			validVpc.InvalidVpcs = append(validVpc.InvalidVpcs, v)
		}
	}

	return validVpc, nil
}

func (p *provider) Validate(ctx context.Context, _ *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) error {
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

	plans, resp, err := client.Region.Availability(ctx, c.Region, "")

	// TODO: Validate region separately
	if err != nil {
		return err
	}
	resp.Body.Close()

	planFound := false

	// Check if given plan present in the returned list
	for _, plan := range plans.AvailablePlans {
		if plan == c.Plan {
			planFound = true
			break
		}
	}
	if !planFound {
		return fmt.Errorf("invalid/not supported plan specified %q, available plans are: %q, %w", c.Plan, plans.AvailablePlans, err)
	}

	validvpc, err := p.validateVpc(ctx, client, c, false)
	if err != nil {
		return err
	}
	if !validvpc.IsAllValid {
		return fmt.Errorf("invalid/not supported vpc id specified %v", validvpc.InvalidVpcs)
	}

	if c.PhysicalMachine {
		// Don't check for validity of legacy VPC as BareMetal doesn't support VPC v1
		return nil
	}

	// Verify legacy VPCs
	validvpc, err = p.validateVpc(ctx, client, c, true)
	if err != nil {
		return err
	}

	if !validvpc.IsAllValid {
		return fmt.Errorf("invalid/not supported vpc id specified %v", validvpc.InvalidVpcs)
	}

	return nil
}

func (p *provider) getPhysicalMachine(ctx context.Context, c *Config, machine *clusterv1alpha1.Machine) (*vultrPhysicalMachine, error) {
	client := getClient(ctx, c.APIKey)
	// Not looping on metadata assuming that tagged machines won;t cross
	// pagination boundary
	instances, _, resp, err := client.BareMetalServer.List(ctx, &govultr.ListOptions{
		Tag: string(machine.UID),
	})
	if err != nil {
		return nil, vltErrorToTerminalError(resp.StatusCode, err)
	}
	resp.Body.Close()
	for _, instance := range instances {
		if sets.NewString(instance.Tags...).Has(string(machine.UID)) {
			return &vultrPhysicalMachine{instance: &instance}, nil
		}
	}
	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) getVirtualMachine(ctx context.Context, c *Config, machine *clusterv1alpha1.Machine) (*vultrVirtualMachine, error) {
	client := getClient(ctx, c.APIKey)

	instances, _, resp, err := client.Instance.List(ctx, &govultr.ListOptions{
		Tag: string(machine.UID),
	})
	if err != nil {
		return nil, vltErrorToTerminalError(resp.StatusCode, err)
	}
	resp.Body.Close()

	for _, instance := range instances {
		if sets.NewString(instance.Tags...).Has(string(machine.UID)) &&
			instance.Label == machine.Name {
			return &vultrVirtualMachine{instance: &instance}, nil
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) Get(ctx context.Context, _ *zap.SugaredLogger, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}
	if !c.PhysicalMachine {
		return p.getVirtualMachine(ctx, c, machine)
	}

	return p.getPhysicalMachine(ctx, c, machine)
}

func (p *provider) GetCloudConfig(_ clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) waitForInstanceCreation(ctx context.Context, c *Config, instance instance.Instance, machine *clusterv1alpha1.Machine) error {
	return wait.PollUntilContextTimeout(ctx, createCheckPeriod, createCheckTimeout, false, func(ctx context.Context) (bool, error) {
		var err error
		if !c.PhysicalMachine {
			_, err = p.getVirtualMachine(ctx, c, machine)
		} else {
			_, err = p.getPhysicalMachine(ctx, c, machine)
		}

		if err != nil {
			if cloudprovidererrors.IsNotFound(err) {
				// Continue the loop as the instances was successfully fetched
				// just that our instance was not found
				return false, nil
			}
			if isTerminalErr, _, _ := cloudprovidererrors.IsTerminalError(err); isTerminalErr {
				return true, err
			}
			// Wait for some time as instance creation is successful
			// just that we are not able to fetch it
			time.Sleep(createCheckFailedWaitPeriod)
			return false, fmt.Errorf("instance %q created but controller failed to fetch instance details", instance.Name())
		}
		return true, nil
	})
}

func (p *provider) createVirtualMachine(ctx context.Context, client *govultr.Client, c *Config, machine *clusterv1alpha1.Machine, osid int, userdata string) (*vultrVirtualMachine, error) {
	tags := sets.List[string](sets.New(c.Tags...).Insert(string(machine.UID)))

	instanceCreateRequest := govultr.InstanceCreateReq{
		Region: c.Region,
		Plan:   c.Plan,
		OsID:   osid,

		Label:    machine.Spec.Name,
		UserData: base64.StdEncoding.EncodeToString([]byte(userdata)),
		Tags:     tags,

		EnableIPv6: &c.EnableIPv6,
		EnableVPC:  &c.EnableVPC,
		AttachVPC:  c.VpcID,
		EnableVPC2: &c.EnableVPC2,
		AttachVPC2: c.Vpc2ID,
	}
	instance, resp, err := client.Instance.Create(ctx, &instanceCreateRequest)
	if err != nil {
		return nil, vltErrorToTerminalError(resp.StatusCode, err)
	}
	resp.Body.Close()

	return &vultrVirtualMachine{instance: instance}, nil
}

func (p *provider) createPhysicalMachine(ctx context.Context, client *govultr.Client, c *Config, machine *clusterv1alpha1.Machine, osid int, userdata string) (*vultrPhysicalMachine, error) {
	tags := sets.NewString(c.Tags...).Insert(string(machine.UID)).List()

	bareMetalCreateRequest := govultr.BareMetalCreate{
		Region:     c.Region,
		Plan:       c.Plan,
		Label:      machine.Spec.Name,
		UserData:   base64.StdEncoding.EncodeToString([]byte(userdata)),
		EnableIPv6: &c.EnableIPv6,
		Tags:       tags,
		OsID:       osid,
		AttachVPC2: c.Vpc2ID,
		EnableVPC2: &c.EnableVPC2,
	}
	instance, resp, err := client.BareMetalServer.Create(ctx, &bareMetalCreateRequest)
	if err != nil {
		return nil, vltErrorToTerminalError(resp.StatusCode, err)
	}
	resp.Body.Close()
	return &vultrPhysicalMachine{instance: instance}, nil
}

func (p *provider) Create(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

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
	strOsID, err := strconv.Atoi(c.OsID)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Cannot parse operating system id %q, details = %v", pc.OperatingSystem, err),
		}
	}
	client := getClient(ctx, c.APIKey)

	var instance instance.Instance
	if !c.PhysicalMachine {
		instance, err = p.createVirtualMachine(ctx, client, c, machine, strOsID, userdata)
		if err != nil {
			return nil, err
		}
	} else {
		instance, err = p.createPhysicalMachine(ctx, client, c, machine, strOsID, userdata)
		if err != nil {
			return nil, err
		}
	}

	err = p.waitForInstanceCreation(ctx, c, instance, machine)
	if err != nil {
		if !c.PhysicalMachine {
			if err := client.Instance.Delete(ctx, instance.ID()); err != nil {
				log.Error("Failed to cleanup instance after failed creation: %v", err)
			}
		} else {
			if err := client.BareMetalServer.Delete(ctx, instance.ID()); err != nil {
				log.Error("Failed to cleanup bare metal instance after failed creation: %v", err)
			}
		}
		return nil, err
	}
	return instance, nil
}

func (p *provider) Cleanup(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	instance, err := p.Get(ctx, log, machine, data)
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

	if !c.PhysicalMachine {
		if err := client.Instance.Delete(ctx, instance.ID()); err != nil {
			return false, fmt.Errorf("failed to delete instance: %w", err)
		}
	} else {
		if err := client.BareMetalServer.Delete(ctx, instance.ID()); err != nil {
			return false, fmt.Errorf("failed to delete bare metal instance: %w", err)
		}
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

func (p *provider) MigrateUID(ctx context.Context, _ *zap.SugaredLogger, machine *clusterv1alpha1.Machine, newUID types.UID) error {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to decode providerconfig: %w", err)
	}
	client := getClient(ctx, c.APIKey)

	if !c.PhysicalMachine {
		instance, err := p.getVirtualMachine(ctx, c, machine)
		if err != nil {
			return err
		}
		_, resp, err := client.Instance.Update(ctx, instance.instance.ID, &govultr.InstanceUpdateReq{
			Tags: sets.NewString(instance.instance.Tags...).Delete(string(machine.UID)).Insert(string(newUID)).List(),
		})
		if err != nil {
			return vltErrorToTerminalError(resp.StatusCode, err)
		}
		resp.Body.Close()
		return nil
	}
	instance, err := p.getPhysicalMachine(ctx, c, machine)
	if err != nil {
		return fmt.Errorf("failed to get instance with UID tag: %w", err)
	}
	_, resp, err := client.BareMetalServer.Update(ctx, instance.instance.ID, &govultr.BareMetalUpdate{
		Tags: sets.NewString(instance.instance.Tags...).Delete(string(machine.UID)).Insert(string(newUID)).List(),
	})
	if err != nil {
		return vltErrorToTerminalError(resp.StatusCode, err)
	}
	resp.Body.Close()
	return nil
}

type vultrVirtualMachine struct {
	instance *govultr.Instance
}
type vultrPhysicalMachine struct {
	instance *govultr.BareMetalServer
}

func (v *vultrVirtualMachine) Name() string {
	return v.instance.Label
}
func (v *vultrPhysicalMachine) Name() string {
	return v.instance.Label
}

func (v *vultrVirtualMachine) ID() string {
	return v.instance.ID
}
func (v *vultrPhysicalMachine) ID() string {
	return v.instance.ID
}

func (v *vultrVirtualMachine) ProviderID() string {
	return "vultr://" + v.instance.ID
}
func (v *vultrPhysicalMachine) ProviderID() string {
	return "vultr://" + v.instance.ID
}

func (v *vultrVirtualMachine) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{}
	addresses[v.instance.MainIP] = v1.NodeExternalIP
	addresses[v.instance.InternalIP] = v1.NodeInternalIP
	return addresses
}
func (v *vultrPhysicalMachine) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{}
	addresses[v.instance.MainIP] = v1.NodeExternalIP
	return addresses
}

func (v *vultrVirtualMachine) Status() instance.Status {
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
func (v *vultrPhysicalMachine) Status() instance.Status {
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

func vltErrorToTerminalError(status int, err error) error {
	switch status {
	case http.StatusUnauthorized:
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: "A request has been rejected due to invalid credentials which were taken from the MachineSpec",
		}
	default:
		return err
	}
}

func (p *provider) SetMetricsForMachines(_ clusterv1alpha1.MachineList) error {
	return nil
}
