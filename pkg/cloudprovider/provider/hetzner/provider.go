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

package hetzner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/hetznercloud/hcloud-go/hcloud"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/common/ssh"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	hetznertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/hetzner/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog"
)

const (
	machineUIDLabelKey = "machine-uid"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a Hetzner provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type Config struct {
	Token                string
	ServerType           string
	Datacenter           string
	Image                string
	Location             string
	PlacementGroupPrefix string
	Networks             []string
	Firewalls            []string
	Labels               map[string]string
}

func getNameForOS(os providerconfigtypes.OperatingSystem) (string, error) {
	switch os {
	case providerconfigtypes.OperatingSystemUbuntu:
		return "ubuntu-20.04", nil
	case providerconfigtypes.OperatingSystemCentOS:
		return "centos-7", nil
	}
	return "", providerconfigtypes.ErrOSNotSupported
}

func getClient(token string) *hcloud.Client {
	return hcloud.NewClient(hcloud.WithToken(token))
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

	rawConfig, err := hetznertypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, err
	}

	c := Config{}
	c.Token, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Token, "HZ_TOKEN")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"token\" field, error = %v", err)
	}

	c.ServerType, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ServerType)
	if err != nil {
		return nil, nil, err
	}

	c.Datacenter, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Datacenter)
	if err != nil {
		return nil, nil, err
	}

	c.Image, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Image)
	if err != nil {
		return nil, nil, err
	}

	c.Location, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Location)
	if err != nil {
		return nil, nil, err
	}

	c.PlacementGroupPrefix, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.PlacementGroupPrefix)
	if err != nil {
		return nil, nil, err
	}

	for _, network := range rawConfig.Networks {
		networkValue, err := p.configVarResolver.GetConfigVarStringValue(network)
		if err != nil {
			return nil, nil, err
		}
		c.Networks = append(c.Networks, networkValue)
	}

	for _, firewall := range rawConfig.Firewalls {
		firewallValue, err := p.configVarResolver.GetConfigVarStringValue(firewall)
		if err != nil {
			return nil, nil, err
		}
		c.Firewalls = append(c.Firewalls, firewallValue)
	}

	c.Labels = rawConfig.Labels

	return &c, pconfig, err
}

func (p *provider) getServerPlacementGroup(ctx context.Context, client *hcloud.Client, c *Config) (*hcloud.PlacementGroup, error) {
	placementGroups, _, err := client.PlacementGroup.List(ctx, hcloud.PlacementGroupListOpts{Type: hcloud.PlacementGroupTypeSpread})
	if err != nil {
		return nil, hzErrorToTerminalError(err, "failed to get placement groups of type spread")
	}
	for _, pg := range placementGroups {
		if !strings.HasPrefix(pg.Name, c.PlacementGroupPrefix) {
			continue
		}
		if len(pg.Servers) < 10 {
			return pg, nil
		}
	}
	pgLabels := map[string]string{}
	for k, v := range c.Labels {
		if k != machineUIDLabelKey {
			pgLabels[k] = v
		}
	}
	createdPg, _, err := client.PlacementGroup.Create(ctx, hcloud.PlacementGroupCreateOpts{
		Name:   fmt.Sprintf("%s-%s", c.PlacementGroupPrefix, rand.SafeEncodeString(rand.String(5))),
		Labels: pgLabels,
		Type:   hcloud.PlacementGroupTypeSpread,
	})
	if err != nil {
		return nil, hzErrorToTerminalError(err, "failed to create placement group")
	}
	return createdPg.PlacementGroup, nil
}

func (p *provider) Validate(spec clusterv1alpha1.MachineSpec) error {
	c, pc, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	if c.Token == "" {
		return errors.New("token is missing")
	}

	_, err = getNameForOS(pc.OperatingSystem)
	if err != nil {
		return fmt.Errorf("invalid/not supported operating system specified %q: %v", pc.OperatingSystem, err)
	}

	ctx := context.TODO()
	client := getClient(c.Token)

	if c.Location != "" && c.Datacenter != "" {
		return fmt.Errorf("location and datacenter must not be set at the same time")
	}

	if c.Location != "" {
		if _, _, err = client.Location.Get(ctx, c.Location); err != nil {
			return fmt.Errorf("failed to get location: %v", err)
		}
	}

	if c.Datacenter != "" {
		if _, _, err = client.Datacenter.Get(ctx, c.Datacenter); err != nil {
			return fmt.Errorf("failed to get datacenter: %v", err)
		}
	}

	if c.Image != "" {
		if _, _, err = client.Image.Get(ctx, c.Image); err != nil {
			return fmt.Errorf("failed to get image: %v", err)
		}
	}

	for _, network := range c.Networks {
		if _, _, err = client.Network.Get(ctx, network); err != nil {
			return fmt.Errorf("failed to get network %q: %v", network, err)
		}
	}

	for _, firewall := range c.Firewalls {
		f, _, err := client.Firewall.Get(ctx, firewall)
		if err != nil {
			return fmt.Errorf("failed to get firewall %q: %v", firewall, err)
		}
		if f == nil {
			return fmt.Errorf("firewall %q does not exist", firewall)
		}
	}

	if _, _, err = client.ServerType.Get(ctx, c.ServerType); err != nil {
		return fmt.Errorf("failed to get server type: %v", err)
	}

	return nil
}

func (p *provider) Create(machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	ctx := context.TODO()
	client := getClient(c.Token)

	if c.Image == "" {
		imageName, err := getNameForOS(pc.OperatingSystem)
		if err != nil {
			return nil, cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: fmt.Sprintf("Invalid operating system specified %q, details = %v", pc.OperatingSystem, err),
			}
		}
		c.Image = imageName
	}

	if c.Labels == nil {
		c.Labels = map[string]string{}
	}

	c.Labels[machineUIDLabelKey] = string(machine.UID)
	serverCreateOpts := hcloud.ServerCreateOpts{
		Name:     machine.Spec.Name,
		UserData: userdata,
		Labels:   c.Labels,
	}

	if c.Datacenter != "" {
		dc, _, err := client.Datacenter.Get(ctx, c.Datacenter)
		if err != nil {
			return nil, hzErrorToTerminalError(err, "failed to get datacenter")
		}
		if dc == nil {
			return nil, fmt.Errorf("datacenter %q does not exist", c.Datacenter)
		}
		serverCreateOpts.Datacenter = dc
	}

	if c.Location != "" {
		location, _, err := client.Location.Get(ctx, c.Location)
		if err != nil {
			return nil, hzErrorToTerminalError(err, "failed to get location")
		}
		if location == nil {
			return nil, fmt.Errorf("location %q does not exist", c.Location)
		}
		serverCreateOpts.Location = location
	}

	if c.PlacementGroupPrefix != "" {
		selectedPg, err := p.getServerPlacementGroup(ctx, client, c)
		if err != nil {
			return nil, err
		}
		serverCreateOpts.PlacementGroup = selectedPg
	}

	for _, network := range c.Networks {
		n, _, err := client.Network.Get(ctx, network)
		if err != nil {
			return nil, hzErrorToTerminalError(err, "failed to get network")
		}
		if n == nil {
			return nil, fmt.Errorf("network %q does not exist", network)
		}
		serverCreateOpts.Networks = append(serverCreateOpts.Networks, n)
	}

	for _, firewall := range c.Firewalls {
		n, _, err := client.Firewall.Get(ctx, firewall)
		if err != nil {
			return nil, hzErrorToTerminalError(err, "failed to get firewall")
		}
		if n == nil {
			return nil, fmt.Errorf("firewall %q does not exist", firewall)
		}
		serverCreateOpts.Firewalls = append(serverCreateOpts.Firewalls, &hcloud.ServerCreateFirewall{Firewall: *n})
	}

	image, _, err := client.Image.Get(ctx, c.Image)
	if err != nil {
		return nil, hzErrorToTerminalError(err, "failed to get image")
	}
	if image == nil {
		return nil, fmt.Errorf("image %q does not exist", c.Image)
	}
	serverCreateOpts.Image = image

	serverType, _, err := client.ServerType.Get(ctx, c.ServerType)
	if err != nil {
		return nil, hzErrorToTerminalError(err, "failed to get server type")
	}
	if serverType == nil {
		return nil, fmt.Errorf("server type %q does not exist", c.ServerType)
	}
	serverCreateOpts.ServerType = serverType

	// We generate a temporary SSH key here, because otherwise Hetzner creates
	// a password and sends it via E-Mail to the account owner, which can be quite
	// spammy. No one will ever get access to the private key.
	sshkey, err := ssh.NewKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ssh key: %v", err)
	}

	hkey, res, err := client.SSHKey.Create(ctx, hcloud.SSHKeyCreateOpts{
		Name:      sshkey.Name,
		PublicKey: sshkey.PublicKey,
	})
	if err != nil {
		return nil, fmt.Errorf("creating temporary ssh key failed with error %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("got invalid http status code when creating ssh key: expected=%d, god=%d", http.StatusCreated, res.StatusCode)
	}
	defer func() {
		_, err := client.SSHKey.Delete(ctx, hkey)
		if err != nil {
			klog.Errorf("Failed to delete temporary ssh key: %v", err)
		}
	}()
	serverCreateOpts.SSHKeys = []*hcloud.SSHKey{hkey}

	serverCreateRes, res, err := client.Server.Create(ctx, serverCreateOpts)
	if err != nil {
		return nil, hzErrorToTerminalError(err, "failed to create server")
	}
	if res.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create server invalid status code returned. expected=%d got %d", http.StatusCreated, res.StatusCode)
	}

	return &hetznerServer{server: serverCreateRes.Server}, nil
}

func (p *provider) Cleanup(machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	instance, err := p.Get(machine, data)
	if err != nil {
		if err == cloudprovidererrors.ErrInstanceNotFound {
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

	ctx := context.TODO()
	client := getClient(c.Token)
	hzServer := instance.(*hetznerServer).server

	res, err := client.Server.Delete(ctx, hzServer)
	if err != nil {
		return false, hzErrorToTerminalError(err, "failed to delete the server")
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNotFound {
		return false, fmt.Errorf("invalid status code returned. expected=%d got=%d", http.StatusOK, res.StatusCode)
	}

	if hzServer.PlacementGroup != nil {
		pgHzServer, _, err := client.PlacementGroup.Get(ctx, hzServer.PlacementGroup.Name)
		if err != nil {
			return false, hzErrorToTerminalError(err, "failed to get placement group")
		}
		count := 0
		for _, s := range pgHzServer.Servers {
			if s != hzServer.ID {
				count++
			}
		}
		if count == 0 {
			_, err := client.PlacementGroup.Delete(ctx, pgHzServer)
			if err != nil {
				return false, hzErrorToTerminalError(err, "failed to delete empty placement group")
			}
		}
	}

	return false, nil
}

func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) Get(machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	ctx := context.TODO()
	client := getClient(c.Token)

	servers, _, err := client.Server.List(ctx, hcloud.ServerListOpts{ListOpts: hcloud.ListOpts{
		LabelSelector: machineUIDLabelKey + "==" + string(machine.UID),
	}})
	if err != nil {
		return nil, hzErrorToTerminalError(err, "failed to list servers")
	}

	for _, server := range servers {
		if server.Labels[machineUIDLabelKey] == string(machine.UID) {
			return &hetznerServer{server: server}, nil
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) MigrateUID(machine *clusterv1alpha1.Machine, new types.UID) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}
	client := getClient(c.Token)

	// We didn't use the UID for Hetzner before
	server, _, err := client.Server.Get(ctx, machine.Spec.Name)
	if err != nil {
		return fmt.Errorf("failed to get server: %v", err)
	}
	if server == nil {
		klog.Infof("No instance exists for machine %s", machine.Name)
		return nil
	}

	klog.Infof("Setting UID label for machine %s", machine.Name)
	_, response, err := client.Server.Update(ctx, server, hcloud.ServerUpdateOpts{
		Labels: map[string]string{machineUIDLabelKey: string(new)},
	})
	if err != nil {
		return fmt.Errorf("failed to update UID label: %v", err)
	}
	if response.Response.StatusCode != http.StatusOK {
		return fmt.Errorf("got unexpected response code %v, expected %v", response.Response.Status, http.StatusOK)
	}
	// This succeeds, but does not result in a label on the server, seems to be a bug
	// on Hetzner side
	klog.Infof("Successfully set UID label for machine %s", machine.Name)

	return nil
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["size"] = c.ServerType
		labels["dc"] = c.Datacenter
		labels["location"] = c.Location
	}

	return labels, err
}

type hetznerServer struct {
	server *hcloud.Server
}

func (s *hetznerServer) Name() string {
	return s.server.Name
}

func (s *hetznerServer) ID() string {
	return strconv.Itoa(s.server.ID)
}

func (s *hetznerServer) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{}
	for _, fips := range s.server.PublicNet.FloatingIPs {
		addresses[fips.IP.String()] = v1.NodeExternalIP
	}
	for _, privateNetwork := range s.server.PrivateNet {
		addresses[privateNetwork.IP.String()] = v1.NodeInternalIP
	}
	addresses[s.server.PublicNet.IPv4.IP.String()] = v1.NodeExternalIP
	addresses[s.server.PublicNet.IPv6.IP.String()] = v1.NodeExternalIP
	return addresses
}

func (s *hetznerServer) Status() instance.Status {
	switch s.server.Status {
	case hcloud.ServerStatusInitializing:
		return instance.StatusCreating
	case hcloud.ServerStatusRunning:
		return instance.StatusRunning
	default:
		return instance.StatusUnknown
	}
}

// hzErrorToTerminalError judges if the given error
// can be qualified as a "terminal" error, for more info see v1alpha1.MachineStatus
//
// if the given error doesn't qualify the error passed as an argument will be returned
func hzErrorToTerminalError(err error, msg string) error {
	prepareAndReturnError := func() error {
		return fmt.Errorf("%s, due to %s", msg, err)
	}

	if err != nil {
		if hcloud.IsError(err, hcloud.ErrorCode("unauthorized")) {
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

func (p *provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	return nil
}
