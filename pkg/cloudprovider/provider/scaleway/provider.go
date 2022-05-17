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

package scaleway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/scaleway/scaleway-sdk-go/validation"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	cloudInstance "github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	scalewaytypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/scaleway/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a Scaleway provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type Config struct {
	AccessKey      string
	SecretKey      string
	ProjectID      string
	Zone           string
	CommercialType string
	IPv6           bool
	Tags           []string
}

func (c *Config) getInstanceAPI() (*instance.API, error) {
	client, err := scw.NewClient(
		scw.WithAuth(c.AccessKey, c.SecretKey),
		scw.WithDefaultZone(scw.Zone(c.Zone)),
		scw.WithDefaultProjectID(c.ProjectID),
		scw.WithUserAgent("kubermatic/machine-controller"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the scaleway client: %w", err)
	}

	return instance.NewAPI(client), nil
}

func getImageNameForOS(os providerconfigtypes.OperatingSystem) (string, error) {
	switch os {
	case providerconfigtypes.OperatingSystemUbuntu:
		// ubuntu_focal doesn't work (see https://bugs.launchpad.net/ubuntu/+source/linux-kvm/+bug/1880522)
		// modprobe ip_vs will fail
		return "ubuntu_bionic", nil
	case providerconfigtypes.OperatingSystemCentOS:
		return "centos_7.6", nil
	}
	return "", providerconfigtypes.ErrOSNotSupported
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

	rawConfig, err := scalewaytypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, err
	}

	c := Config{}
	c.AccessKey, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.AccessKey, scw.ScwAccessKeyEnv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"access_key\" field, error = %w", err)
	}
	c.SecretKey, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.SecretKey, scw.ScwSecretKeyEnv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"secret_key\" field, error = %w", err)
	}
	c.ProjectID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ProjectID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"project_id\" field, error = %w", err)
	}
	c.Zone, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Zone)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"zone\" field, error = %w", err)
	}
	c.CommercialType, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.CommercialType)
	if err != nil {
		return nil, nil, err
	}
	c.IPv6, _, err = p.configVarResolver.GetConfigVarBoolValue(rawConfig.IPv6)
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

	if !validation.IsAccessKey(c.AccessKey) {
		return fmt.Errorf("invalid access key format '%s', expected SCWXXXXXXXXXXXXXXXXX format", c.AccessKey)
	}
	if !validation.IsSecretKey(c.SecretKey) {
		return fmt.Errorf("invalid secret key format '%s', expected a UUID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", c.SecretKey)
	}
	if !validation.IsProjectID(c.ProjectID) {
		return fmt.Errorf("invalid project ID format '%s', expected a UUID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", c.ProjectID)
	}

	_, err = scw.ParseZone(c.Zone)
	if err != nil {
		return err
	}

	if c.CommercialType == "" {
		return errors.New("commercial type is missing")
	}

	_, err = getImageNameForOS(pc.OperatingSystem)
	if err != nil {
		return fmt.Errorf("invalid operating system specified %q: %w", pc.OperatingSystem, err)
	}

	return nil
}

func (p *provider) Create(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (cloudInstance.Instance, error) {
	c, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	api, err := c.getInstanceAPI()
	if err != nil {
		return nil, err
	}

	imageName, err := getImageNameForOS(pc.OperatingSystem)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, invalid operating system specified %q: %v", pc.OperatingSystem, err),
		}
	}
	createServerRequest := &instance.CreateServerRequest{
		Image:          imageName,
		Name:           machine.Spec.Name,
		CommercialType: c.CommercialType,
		Tags:           append(c.Tags, string(machine.UID)),
		EnableIPv6:     c.IPv6,
	}

	serverResp, err := api.CreateServer(createServerRequest, scw.WithContext(ctx))
	if err != nil {
		return nil, scalewayErrToTerminalError(err)
	}

	err = api.SetServerUserData(&instance.SetServerUserDataRequest{
		Key:      "cloud-init",
		ServerID: serverResp.Server.ID,
		Content:  strings.NewReader(userdata),
	})
	if err != nil {
		return nil, scalewayErrToTerminalError(err)
	}

	klog.V(6).Infof("Scaleway server (id='%s') got fully created", serverResp.Server.ID)

	return &scwServer{server: serverResp.Server}, err
}

func (p *provider) Cleanup(ctx context.Context, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (bool, error) {
	i, err := p.get(machine)
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
	api, err := c.getInstanceAPI()
	if err != nil {
		return false, err
	}

	_, err = api.ServerAction(&instance.ServerActionRequest{
		Action:   instance.ServerActionTerminate,
		ServerID: i.ID(),
	}, scw.WithContext(ctx))
	if err != nil {
		return false, scalewayErrToTerminalError(err)
	}

	return false, nil
}

func (p *provider) Get(_ context.Context, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (cloudInstance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	api, err := c.getInstanceAPI()
	if err != nil {
		return nil, err
	}

	i, err := p.get(machine)
	if err != nil {
		return nil, err
	}

	if i.server.State == instance.ServerStateStopped || i.server.State == instance.ServerStateStoppedInPlace {
		_, err := api.ServerAction(&instance.ServerActionRequest{
			Action:   instance.ServerActionPoweron,
			ServerID: i.server.ID,
		})
		if err != nil {
			return nil, scalewayErrToTerminalError(err)
		}

		return nil, fmt.Errorf("scaleway instance %s is in a stopped state, powering the instance on is in progress", i.Name())
	}

	return i, nil
}

func (p *provider) get(machine *clusterv1alpha1.Machine) (*scwServer, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	api, err := c.getInstanceAPI()
	if err != nil {
		return nil, err
	}

	serversResp, err := api.ListServers(&instance.ListServersRequest{
		Name: scw.StringPtr(machine.Spec.Name),
		Tags: []string{string(machine.UID)},
	}, scw.WithAllPages())
	if err != nil {
		return nil, scalewayErrToTerminalError(err)
	}

	for _, server := range serversResp.Servers {
		if server.Name == machine.Spec.Name && sets.NewString(server.Tags...).Has(string(machine.UID)) {
			return &scwServer{server: server}, nil
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) MigrateUID(_ context.Context, machine *clusterv1alpha1.Machine, newUID types.UID) error {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to decode providerconfig: %w", err)
	}
	api, err := c.getInstanceAPI()
	if err != nil {
		return err
	}

	server, err := p.get(machine)
	if err != nil {
		return err
	}

	oldTags := server.server.Tags
	newTags := []string{string(newUID)}
	for _, oldTag := range oldTags {
		if oldTag != string(machine.UID) {
			newTags = append(newTags, oldTag)
		}
	}

	_, err = api.UpdateServer(&instance.UpdateServerRequest{
		Tags:     scw.StringsPtr(newTags),
		ServerID: server.ID(),
	})
	if err != nil {
		return scalewayErrToTerminalError(err)
	}

	return nil
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["commercial_type"] = c.CommercialType
		labels["zone"] = c.Zone
	}

	return labels, err
}

type scwServer struct {
	server *instance.Server
}

func (s *scwServer) Name() string {
	return s.server.Name
}

func (s *scwServer) ID() string {
	return s.server.ID
}

func (s *scwServer) Addresses() map[string]corev1.NodeAddressType {
	addresses := map[string]corev1.NodeAddressType{}
	if s.server.PrivateIP != nil {
		addresses[*s.server.PrivateIP] = corev1.NodeInternalIP
	}

	if s.server.PublicIP != nil {
		addresses[s.server.PublicIP.Address.String()] = corev1.NodeExternalIP
	}

	if s.server.IPv6 != nil {
		addresses[s.server.IPv6.Address.String()] = corev1.NodeExternalIP
	}

	return addresses
}

func (s *scwServer) Status() cloudInstance.Status {
	switch s.server.State {
	case instance.ServerStateStarting:
		return cloudInstance.StatusCreating
	case instance.ServerStateRunning:
		return cloudInstance.StatusRunning
	case instance.ServerStateStopping:
		return cloudInstance.StatusDeleting
	default:
		return cloudInstance.StatusUnknown
	}
}

// scalewayErrToTerminalError judges if the given error
// can be qualified as a "terminal" error, for more info see v1alpha1.MachineStatus

// if the given error doesn't qualify the error passed as
// an argument will be returned.
func scalewayErrToTerminalError(err error) error {
	var deinedErr *scw.PermissionsDeniedError
	var invalidArgErr *scw.InvalidArgumentsError
	var outOfStackErr *scw.OutOfStockError
	var quotaErr *scw.QuotasExceededError

	if errors.As(err, &deinedErr) {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: "A request has been rejected due to invalid credentials which were taken from the MachineSpec",
		}
	} else if errors.As(err, &invalidArgErr) {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: "A request has been rejected due to invalid arguments which were taken from the MachineSpec",
		}
	} else if errors.As(err, &outOfStackErr) {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InsufficientResourcesMachineError,
			Message: "A request has been rejected due to out of stocks",
		}
	} else if errors.As(err, &quotaErr) {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InsufficientResourcesMachineError,
			Message: "A request has been rejected due to insufficient quotas",
		}
	}
	return err
}

func (p *provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	return nil
}
