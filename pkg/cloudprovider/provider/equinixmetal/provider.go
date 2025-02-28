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
	"net/http"
	"slices"
	"strings"

	"github.com/equinix/equinix-sdk-go/services/metalv1"
	"go.uber.org/zap"

	cloudprovidererrors "k8c.io/machine-controller/pkg/cloudprovider/errors"
	"k8c.io/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "k8c.io/machine-controller/pkg/cloudprovider/types"
	"k8c.io/machine-controller/sdk/apis/cluster/common"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
	equinixmetaltypes "k8c.io/machine-controller/sdk/cloudprovider/equinixmetal"
	"k8c.io/machine-controller/sdk/providerconfig"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
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
	Metro        string
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

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *equinixmetaltypes.RawConfig, *providerconfig.Config, error) {
	pconfig, err := providerconfig.GetConfig(provSpec)
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
	c.Metro, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Metro)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"metro\" field, error = %w", err)
	}

	// ensure we have defaults
	c.populateDefaults()

	return &c, rawConfig, pconfig, err
}

func (p *provider) getMetalDevice(ctx context.Context, machine *clusterv1alpha1.Machine) (*metalv1.Device, *metalv1.APIClient, error) {
	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c.Token)
	device, err := getDeviceByTag(ctx, client, c.ProjectID, generateTag(string(machine.UID)))
	if err != nil {
		return nil, nil, err
	}
	return device, client, nil
}

func (p *provider) Validate(ctx context.Context, _ *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) error {
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

	if c.Metro == "" && (len(c.Facilities) == 0 || c.Facilities[0] == "") {
		return fmt.Errorf("must have at least one non-blank facility or a metro")
	}

	if c.Facilities != nil && (len(c.Facilities) > 0 || c.Facilities[0] != "") {
		// get all valid facilities
		request := client.FacilitiesApi.FindFacilitiesByProject(ctx, c.ProjectID)
		facilities, resp, err := client.FacilitiesApi.FindFacilitiesByProjectExecute(request)
		if err != nil {
			return fmt.Errorf("failed to list facilities: %w", err)
		}
		resp.Body.Close()

		expectedFacilities := sets.New(c.Facilities...)
		availableFacilities := sets.New[string]()
		for _, facility := range facilities.Facilities {
			availableFacilities.Insert(*facility.Code)
		}

		// ensure our requested facilities are in those facilities
		if diff := expectedFacilities.Difference(availableFacilities); diff.Len() > 0 {
			return fmt.Errorf("unknown facilities: %v", sets.List(diff))
		}
	}

	if c.Metro != "" {
		request := client.MetrosApi.FindMetros(ctx)
		metros, resp, err := client.MetrosApi.FindMetrosExecute(request)
		if err != nil {
			return fmt.Errorf("failed to list metros: %w", err)
		}
		resp.Body.Close()

		metroExists := slices.ContainsFunc(metros.Metros, func(m metalv1.Metro) bool {
			return strings.EqualFold(*m.Code, c.Metro)
		})

		if !metroExists {
			return fmt.Errorf("unknown metro %q", c.Metro)
		}
	}

	// get all valid plans a.k.a. instance types
	request := client.PlansApi.FindPlansByProject(ctx, c.ProjectID)
	plans, resp, err := client.PlansApi.FindPlansByProjectExecute(request)
	if err != nil {
		return fmt.Errorf("failed to list instance types / plans: %w", err)
	}
	resp.Body.Close()

	// ensure our requested plan is in those plans
	expectedPlans := sets.New(c.InstanceType)
	availablePlans := sets.New[string]()
	for _, plan := range plans.Plans {
		availablePlans.Insert(*plan.Name)
	}

	if diff := expectedPlans.Difference(availablePlans); diff.Len() > 0 {
		return fmt.Errorf("unknown instance type / plan: %s, acceptable plans: %v", c.InstanceType, sets.List(availablePlans))
	}

	return nil
}

func (p *provider) Create(ctx context.Context, _ *zap.SugaredLogger, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, _, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c.Token)
	request := client.DevicesApi.CreateDevice(ctx, c.ProjectID)

	imageName, err := getNameForOS(pc.OperatingSystem)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Invalid operating system specified %q, details = %v", pc.OperatingSystem, err),
		}
	}

	billingCycle := metalv1.DeviceCreateInputBillingCycle(c.BillingCycle)

	if c.Metro != "" {
		request = request.CreateDeviceRequest(metalv1.CreateDeviceRequest{
			DeviceCreateInMetroInput: &metalv1.DeviceCreateInMetroInput{
				Hostname:        &machine.Spec.Name,
				Userdata:        &userdata,
				Metro:           c.Metro,
				BillingCycle:    &billingCycle,
				Plan:            c.InstanceType,
				OperatingSystem: imageName,
				Tags: []string{
					generateTag(string(machine.UID)),
				},
			},
		})
	} else {
		request = request.CreateDeviceRequest(metalv1.CreateDeviceRequest{
			DeviceCreateInFacilityInput: &metalv1.DeviceCreateInFacilityInput{
				Hostname:        &machine.Spec.Name,
				Userdata:        &userdata,
				Facility:        c.Facilities,
				BillingCycle:    &billingCycle,
				Plan:            c.InstanceType,
				OperatingSystem: imageName,
				Tags: []string{
					generateTag(string(machine.UID)),
				},
			},
		})
	}

	device, resp, err := client.DevicesApi.CreateDeviceExecute(request)
	if err != nil {
		return nil, metalErrorToTerminalError(err, resp, "failed to create server")
	}
	resp.Body.Close()

	return &metalDevice{device: device}, nil
}

func (p *provider) Cleanup(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	instance, err := p.Get(ctx, log, machine, data)
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
	request := client.DevicesApi.DeleteDevice(ctx, *instance.(*metalDevice).device.Id)

	resp, err := client.DevicesApi.DeleteDeviceExecute(request)
	if err != nil {
		return false, metalErrorToTerminalError(err, resp, "failed to delete the server")
	}
	resp.Body.Close()

	return false, nil
}

func (p *provider) AddDefaults(_ *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
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

func (p *provider) Get(ctx context.Context, _ *zap.SugaredLogger, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	device, _, err := p.getMetalDevice(ctx, machine)
	if err != nil {
		return nil, err
	}
	if device != nil {
		return &metalDevice{device: device}, nil
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) MigrateUID(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, newID types.UID) error {
	device, client, err := p.getMetalDevice(ctx, machine)
	if err != nil {
		return err
	}
	if device == nil {
		log.Info("No instance exists for machine")
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

	log.Info("Setting UID label for machine")

	dur := client.DevicesApi.
		UpdateDevice(ctx, *device.Id).
		DeviceUpdateInput(metalv1.DeviceUpdateInput{
			Tags: tags,
		})

	_, response, err := client.DevicesApi.UpdateDeviceExecute(dur)
	if err != nil {
		return metalErrorToTerminalError(err, response, "failed to update UID label")
	}
	response.Body.Close()

	log.Info("Successfully set UID label for machine")

	return nil
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

func (p *provider) SetMetricsForMachines(_ clusterv1alpha1.MachineList) error {
	return nil
}

type metalDevice struct {
	device *metalv1.Device
}

func (s *metalDevice) Name() string {
	return *s.device.Hostname
}

func (s *metalDevice) ID() string {
	return *s.device.Id
}

func (s *metalDevice) ProviderID() string {
	if s.device == nil || *s.device.Id == "" {
		return ""
	}
	return "equinixmetal://" + *s.device.Id
}

// Addresses returns addresses in CIDR format.
func (s *metalDevice) Addresses() map[string]corev1.NodeAddressType {
	addresses := map[string]corev1.NodeAddressType{}
	for _, ip := range s.device.IpAddresses {
		kind := corev1.NodeInternalIP
		if *ip.Public {
			kind = corev1.NodeExternalIP
		}

		addresses[*ip.Address] = kind
	}

	return addresses
}

func (s *metalDevice) Status() instance.Status {
	if s.device.State == nil {
		return instance.StatusUnknown
	}

	switch *s.device.State {
	case metalv1.DEVICESTATE_PROVISIONING:
		return instance.StatusCreating
	case metalv1.DEVICESTATE_ACTIVE:
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

	pconfig, err := providerconfig.GetConfig(s)
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

func getDeviceByTag(ctx context.Context, client *metalv1.APIClient, projectID, tag string) (*metalv1.Device, error) {
	request := client.DevicesApi.
		FindProjectDevices(ctx, projectID).
		Tag(tag)

	devices, response, err := client.DevicesApi.FindProjectDevicesExecute(request)
	if err != nil {
		return nil, metalErrorToTerminalError(err, response, "failed to list devices")
	}
	response.Body.Close()

	for _, device := range devices.Devices {
		if slices.Contains(device.Tags, tag) {
			return &device, nil
		}
	}

	return nil, nil
}

// given a defined Kubermatic constant for an operating system, return the canonical slug for Equinix Metal.
func getNameForOS(os providerconfig.OperatingSystem) (string, error) {
	switch os {
	case providerconfig.OperatingSystemUbuntu:
		return "ubuntu_24_04", nil
	case providerconfig.OperatingSystemFlatcar:
		return "flatcar_stable", nil
	case providerconfig.OperatingSystemRockyLinux:
		return "rocky_8", nil
	}
	return "", providerconfig.ErrOSNotSupported
}

func getClient(apiKey string) *metalv1.APIClient {
	configuration := metalv1.NewConfiguration()
	configuration.UserAgent = fmt.Sprintf("kubermatic/machine-controller %s", configuration.UserAgent)
	configuration.AddDefaultHeader("X-Auth-Token", apiKey)

	return metalv1.NewAPIClient(configuration)
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
func metalErrorToTerminalError(err error, response *http.Response, msg string) error {
	prepareAndReturnError := func() error {
		return fmt.Errorf("%s: %w", msg, err)
	}

	if err != nil {
		if response != nil && response.StatusCode == http.StatusForbidden {
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
