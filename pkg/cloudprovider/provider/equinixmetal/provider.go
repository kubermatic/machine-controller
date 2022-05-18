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

package equinixmetal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/packethost/packngo"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	equinixmetaltypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/equinixmetal/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
)

const (
	machineUIDTag       = "kubermatic-machine-controller:machine-uid"
	defaultBillingCycle = "hourly"
)

// New returns a Equinix Metal provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type Config struct {
	Token        string
	ProjectID    string
	BillingCycle string
	InstanceType string
	Facilities   []string
	Tags         []string
}

// because we have both Config and RawConfig, we need to have func for each
// ideally, these would be merged into one.
func (c *Config) populateDefaults() {
	if c.BillingCycle == "" {
		c.BillingCycle = defaultBillingCycle
	}
}

func populateDefaults(c *equinixmetaltypes.RawConfig) {
	if c.BillingCycle.Value == "" {
		c.BillingCycle.Value = defaultBillingCycle
	}
}

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *equinixmetaltypes.RawConfig, *providerconfigtypes.Config, error) {
	if provSpec.Value == nil {
		return nil, nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, nil, err
	}

	rawConfig, err := equinixmetaltypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, nil, err
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	c := Config{}
	c.Token, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Token, "METAL_AUTH_TOKEN")
	if err != nil || len(c.Token) == 0 {
		// This retry is temporary and is only required to facilitate migration from Packet to Equinix Metal
		// We look for env variable PACKET_API_KEY associated with Packet to ensure that nothing breaks during automated migration for the Machines
		// TODO(@ahmedwaleedmalik) Remove this after a release period
		c.Token, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Token, "PACKET_API_KEY")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get the value of \"apiKey\" field, error = %w", err)
		}
	}
	c.ProjectID, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.ProjectID, "METAL_PROJECT_ID")
	if err != nil || len(c.ProjectID) == 0 {
		// This retry is temporary and is only required to facilitate migration from Packet to Equinix Metal
		// We look for env variable PACKET_PROJECT_ID associated with Packet to ensure that nothing breaks during automated migration for the Machines
		// TODO(@ahmedwaleedmalik) Remove this after a release period
		c.ProjectID, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.ProjectID, "PACKET_PROJECT_ID")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get the value of \"apiKey\" field, error = %w", err)
		}
	}
	c.InstanceType, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.InstanceType)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"instanceType\" field, error = %w", err)
	}
	c.BillingCycle, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.BillingCycle)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"billingCycle\" field, error = %w", err)
	}
	for i, tag := range rawConfig.Tags {
		tagValue, err := p.configVarResolver.GetConfigVarStringValue(tag)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to read the value for the Tag at index %d of the \"tags\" field, error = %w", i, err)
		}
		c.Tags = append(c.Tags, tagValue)
	}
	for i, facility := range rawConfig.Facilities {
		facilityValue, err := p.configVarResolver.GetConfigVarStringValue(facility)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to read the value for the Tag at index %d of the \"facilities\" field, error = %w", i, err)
		}
		c.Facilities = append(c.Facilities, facilityValue)
	}

	// ensure we have defaults
	c.populateDefaults()

	return &c, rawConfig, pconfig, err
}

func (p *provider) getMetalDevice(machine *clusterv1alpha1.Machine) (*packngo.Device, *packngo.Client, error) {
	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c.Token)
	device, err := getDeviceByTag(client, c.ProjectID, generateTag(string(machine.UID)))
	if err != nil {
		return nil, nil, err
	}
	return device, client, nil
}

func (p *provider) Validate(_ context.Context, spec clusterv1alpha1.MachineSpec) error {
	c, _, pc, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if c.Token == "" {
		return errors.New("apiKey is missing")
	}
	if c.InstanceType == "" {
		return errors.New("instanceType is missing")
	}
	if c.ProjectID == "" {
		return errors.New("projectID is missing")
	}

	_, err = getNameForOS(pc.OperatingSystem)
	if err != nil {
		return fmt.Errorf("invalid/not supported operating system specified %q: %w", pc.OperatingSystem, err)
	}

	client := getClient(c.Token)

	if len(c.Facilities) == 0 || c.Facilities[0] == "" {
		return fmt.Errorf("must have at least one non-blank facility")
	}

	// get all valid facilities
	facilities, _, err := client.Facilities.List(nil)
	if err != nil {
		return fmt.Errorf("failed to list facilities: %w", err)
	}
	// ensure our requested facilities are in those facilities
	if missingFacilities := itemsNotInList(facilityProp(facilities, "Code"), c.Facilities); len(missingFacilities) > 0 {
		return fmt.Errorf("unknown facilities: %s", strings.Join(missingFacilities, ","))
	}

	// get all valid plans a.k.a. instance types
	plans, _, err := client.Plans.List(nil)
	if err != nil {
		return fmt.Errorf("failed to list instance types / plans: %w", err)
	}
	// ensure our requested plan is in those plans
	validPlanNames := planProp(plans, "Name")
	if missingPlans := itemsNotInList(validPlanNames, []string{c.InstanceType}); len(missingPlans) > 0 {
		return fmt.Errorf("unknown instance type / plan: %s, acceptable plans: %s", strings.Join(missingPlans, ","), strings.Join(validPlanNames, ","))
	}

	return nil
}

func (p *provider) Create(_ context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, _, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c.Token)

	imageName, err := getNameForOS(pc.OperatingSystem)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Invalid operating system specified %q, details = %v", pc.OperatingSystem, err),
		}
	}

	serverCreateOpts := &packngo.DeviceCreateRequest{
		Hostname:     machine.Spec.Name,
		UserData:     userdata,
		ProjectID:    c.ProjectID,
		Facility:     c.Facilities,
		BillingCycle: c.BillingCycle,
		Plan:         c.InstanceType,
		OS:           imageName,
		Tags: []string{
			generateTag(string(machine.UID)),
		},
	}

	device, res, err := client.Devices.Create(serverCreateOpts)
	if err != nil {
		return nil, metalErrorToTerminalError(err, res, "failed to create server")
	}

	return &metalDevice{device: device}, nil
}

func (p *provider) Cleanup(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	instance, err := p.Get(ctx, machine, data)
	if err != nil {
		if errors.Is(err, cloudprovidererrors.ErrInstanceNotFound) {
			return true, nil
		}
		return false, err
	}

	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c.Token)
	res, err := client.Devices.Delete(instance.(*metalDevice).device.ID)
	if err != nil {
		return false, metalErrorToTerminalError(err, res, "failed to delete the server")
	}

	return false, nil
}

func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	_, rawConfig, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return spec, err
	}
	populateDefaults(rawConfig)
	spec.ProviderSpec.Value, err = setProviderSpec(*rawConfig, spec.ProviderSpec)
	if err != nil {
		return spec, err
	}
	return spec, nil
}

func (p *provider) Get(_ context.Context, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	device, _, err := p.getMetalDevice(machine)
	if err != nil {
		return nil, err
	}
	if device != nil {
		return &metalDevice{device: device}, nil
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) MigrateUID(_ context.Context, machine *clusterv1alpha1.Machine, newID types.UID) error {
	device, client, err := p.getMetalDevice(machine)
	if err != nil {
		return err
	}
	if device == nil {
		klog.Infof("No instance exists for machine %s", machine.Name)
		return nil
	}

	// go through existing labels, make sure that no other UID label exists
	tags := make([]string, 0)
	for _, t := range device.Tags {
		// filter out old UID tag(s)
		if _, err := getTagUID(t); err != nil {
			tags = append(tags, t)
		}
	}

	// create a new UID label
	tags = append(tags, generateTag(string(newID)))

	klog.Infof("Setting UID label for machine %s", machine.Name)
	dur := &packngo.DeviceUpdateRequest{
		Tags: &tags,
	}
	_, response, err := client.Devices.Update(device.ID, dur)
	if err != nil {
		return metalErrorToTerminalError(err, response, "failed to update UID label")
	}
	klog.Infof("Successfully set UID label for machine %s", machine.Name)

	return nil
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["size"] = c.InstanceType
		labels["facilities"] = strings.Join(c.Facilities, ",")
	}

	return labels, err
}

func (p *provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	return nil
}

type metalDevice struct {
	device *packngo.Device
}

func (s *metalDevice) Name() string {
	return s.device.Hostname
}

func (s *metalDevice) ID() string {
	return s.device.ID
}

func (s *metalDevice) Addresses() map[string]v1.NodeAddressType {
	// returns addresses in CIDR format
	addresses := map[string]v1.NodeAddressType{}
	for _, ip := range s.device.Network {
		if ip.Public {
			addresses[ip.Address] = v1.NodeExternalIP
			continue
		}
		addresses[ip.Address] = v1.NodeInternalIP
	}

	return addresses
}

func (s *metalDevice) Status() instance.Status {
	switch s.device.State {
	case "provisioning":
		return instance.StatusCreating
	case "active":
		return instance.StatusRunning
	default:
		return instance.StatusUnknown
	}
}

// CONVENIENCE INTERNAL FUNCTIONS.
func setProviderSpec(rawConfig equinixmetaltypes.RawConfig, s clusterv1alpha1.ProviderSpec) (*runtime.RawExtension, error) {
	if s.Value == nil {
		return nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(s)
	if err != nil {
		return nil, err
	}

	rawCloudProviderSpec, err := json.Marshal(rawConfig)
	if err != nil {
		return nil, err
	}

	pconfig.CloudProviderSpec = runtime.RawExtension{Raw: rawCloudProviderSpec}
	rawPconfig, err := json.Marshal(pconfig)
	if err != nil {
		return nil, err
	}

	return &runtime.RawExtension{Raw: rawPconfig}, nil
}

func getDeviceByTag(client *packngo.Client, projectID, tag string) (*packngo.Device, error) {
	devices, response, err := client.Devices.List(projectID, nil)
	if err != nil {
		return nil, metalErrorToTerminalError(err, response, "failed to list devices")
	}

	for _, device := range devices {
		if itemInList(device.Tags, tag) {
			return &device, nil
		}
	}
	return nil, nil
}

// given a defined Kubermatic constant for an operating system, return the canonical slug for Equinix Metal.
func getNameForOS(os providerconfigtypes.OperatingSystem) (string, error) {
	switch os {
	case providerconfigtypes.OperatingSystemUbuntu:
		return "ubuntu_20_04", nil
	case providerconfigtypes.OperatingSystemCentOS:
		return "centos_7", nil
	case providerconfigtypes.OperatingSystemFlatcar:
		return "flatcar_stable", nil
	}
	return "", providerconfigtypes.ErrOSNotSupported
}

func getClient(apiKey string) *packngo.Client {
	return packngo.NewClientWithAuth("kubermatic", apiKey, nil)
}

func generateTag(ID string) string {
	return fmt.Sprintf("%s:%s", machineUIDTag, ID)
}

func getTagUID(tag string) (string, error) {
	parts := strings.Split(tag, ":")
	if len(parts) < 2 || parts[0] != machineUIDTag {
		return "", fmt.Errorf("not a machine UID tag")
	}
	return parts[1], nil
}

// metalErrorToTerminalError judges if the given error
// can be qualified as a "terminal" error, for more info see v1alpha1.MachineStatus
//
// if the given error doesn't qualify the error passed as an argument will be returned.
func metalErrorToTerminalError(err error, response *packngo.Response, msg string) error {
	prepareAndReturnError := func() error {
		return fmt.Errorf("%s, due to %w", msg, err)
	}

	if err != nil {
		if response != nil && response.Response != nil && response.Response.StatusCode == 403 {
			// authorization primitives come from MachineSpec
			// thus we are setting InvalidConfigurationMachineError
			return cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: "A request has been rejected due to invalid credentials which were taken from the MachineSpec",
			}
		}

		return prepareAndReturnError()
	}

	return err
}

func itemInList(list []string, item string) bool {
	for _, elm := range list {
		if elm == item {
			return true
		}
	}
	return false
}

func itemsNotInList(list, items []string) []string {
	listMap := make(map[string]bool)
	missing := make([]string, 0)
	for _, item := range list {
		listMap[item] = true
	}
	for _, item := range items {
		if _, ok := listMap[item]; !ok {
			missing = append(missing, item)
		}
	}
	return missing
}

func facilityProp(vs []packngo.Facility, field string) []string {
	vsm := make([]string, len(vs))
	for i, v := range vs {
		val := reflect.ValueOf(v)
		vsm[i] = val.FieldByName(field).String()
	}
	return vsm
}

func planProp(vs []packngo.Plan, field string) []string {
	vsm := make([]string, len(vs))
	for i, v := range vs {
		val := reflect.ValueOf(v)
		vsm[i] = val.FieldByName(field).String()
	}
	return vsm
}
