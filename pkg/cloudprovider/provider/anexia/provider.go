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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	anxclient "github.com/anexia-it/go-anxcloud/pkg/client"
	anxaddr "github.com/anexia-it/go-anxcloud/pkg/ipam/address"
	anxvsphere "github.com/anexia-it/go-anxcloud/pkg/vsphere"
	anxvm "github.com/anexia-it/go-anxcloud/pkg/vsphere/provisioning/vm"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/common/ssh"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	anxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

type Config struct {
	Token      string
	VlanID     string
	LocationID string
	TemplateID string
	CPUs       int
	Memory     int
	DiskSize   int
}

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, error) {
	if provSpec.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerSpec.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, err
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig, err := anxtypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, err
	}

	c := Config{}
	c.Token, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Token, anxtypes.AnxTokenEnv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get 'token': %v", err)
	}

	c.CPUs = rawConfig.CPUs
	c.Memory = rawConfig.Memory
	c.DiskSize = rawConfig.DiskSize

	c.LocationID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.LocationID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get 'locationID': %v", err)
	}

	c.TemplateID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.TemplateID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get 'templateID': %v", err)
	}

	c.VlanID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VlanID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get 'vlanID': %v", err)
	}

	return &c, pconfig, nil
}

// New returns an Anexia provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

// AddDefaults adds omitted optional values to the given MachineSpec
func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
}

// Validate returns success or failure based according to its ProviderSpec
func (p *provider) Validate(machinespec clusterv1alpha1.MachineSpec) error {
	config, _, err := p.getConfig(machinespec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if config.Token == "" {
		return errors.New("token is missing")
	}

	if config.CPUs == 0 {
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

func (p *provider) Get(machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to parse MachineSpec: %v", err)
	}

	cli, err := getClient(config.Token)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to create Anexia client: %v", err)
	}
	vsphere := anxvsphere.NewAPI(cli)

	status, err := getStatus(machine.Status.ProviderStatus)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}
	if status.InstanceID == "" {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), anxtypes.GetRequestTimeout)
	defer cancel()

	info, err := vsphere.Info().Get(ctx, status.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("failed get machine info: %w", err)
	}

	return &anexiaInstance{
		info: &info,
	}, nil
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (string, string, error) {
	return "", "", nil
}

// Create creates a cloud instance according to the given machine
func (p *provider) Create(machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string, networkConfig *cloudprovidertypes.NetworkConfig) (instance.Instance, error) {
	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to parse MachineSpec: %v", err)
	}

	cli, err := getClient(config.Token)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to create Anexia client: %v", err)
	}
	vsphere := anxvsphere.NewAPI(cli)
	addr := anxaddr.NewAPI(cli)

	ctx, cancel := context.WithTimeout(context.Background(), anxtypes.CreateRequestTimeout)
	defer cancel()

	status, err := getStatus(machine.Status.ProviderStatus)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}

	if status.ProvisioningID == "" {
		res, err := addr.ReserveRandom(ctx, anxaddr.ReserveRandom{
			LocationID: config.LocationID,
			VlanID:     config.VlanID,
			Count:      1,
		})
		if err != nil {
			return nil, newError(common.InvalidConfigurationMachineError, "failed to reserve an ip address: %v", err)
		}
		if len(res.Data) < 1 {
			return nil, newError(common.InsufficientResourcesMachineError, "no ip address is available for this machine")
		}

		networkInterfaces := []anxvm.Network{{
			NICType: anxtypes.VmxNet3NIC,
			IPs:     []string{res.Data[0].Address},
			VLAN:    config.VlanID,
		}}

		vm := vsphere.Provisioning().VM().NewDefinition(
			config.LocationID,
			"templates",
			config.TemplateID,
			machine.ObjectMeta.Name,
			config.CPUs,
			config.Memory,
			config.DiskSize,
			networkInterfaces,
		)

		vm.Script = base64.StdEncoding.EncodeToString([]byte(userdata))

		sshKey, err := ssh.NewKey()
		if err != nil {
			return nil, newError(common.CreateMachineError, "failed to generate ssh key: %v", err)
		}
		vm.SSH = sshKey.PublicKey

		provisionResponse, err := vsphere.Provisioning().VM().Provision(ctx, vm)
		if err != nil {
			return nil, newError(common.CreateMachineError, "instance provisioning failed: %v", err)
		}

		status.ProvisioningID = provisionResponse.Identifier
		if err := updateStatus(machine, status, data.Update); err != nil {
			return nil, newError(common.UpdateMachineError, "machine status update failed: %v", err)
		}
	}

	instanceID, err := vsphere.Provisioning().Progress().AwaitCompletion(ctx, status.ProvisioningID)
	if err != nil {
		return nil, newError(common.CreateMachineError, "instance provisioning failed: %v", err)
	}

	status.InstanceID = instanceID
	if err := updateStatus(machine, status, data.Update); err != nil {
		return nil, newError(common.UpdateMachineError, "machine status update failed: %v", err)
	}

	return p.Get(machine, data)
}

func (p *provider) Cleanup(machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (bool, error) {
	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to parse MachineSpec: %v", err)
	}

	cli, err := getClient(config.Token)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to create Anexia client: %v", err)
	}
	vsphere := anxvsphere.NewAPI(cli)

	status, err := getStatus(machine.Status.ProviderStatus)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), anxtypes.DeleteRequestTimeout)
	defer cancel()

	err = vsphere.Provisioning().VM().Deprovision(ctx, status.InstanceID, false)
	if err != nil {
		var respErr *anxclient.ResponseError
		// Only error if the error was not "not found"
		if !(errors.As(err, &respErr) && respErr.ErrorData.Code == http.StatusNotFound) {
			return false, newError(common.DeleteMachineError, "failed to delete machine: %v", err)
		}
	}

	return true, nil
}

func (p *provider) MigrateUID(_ *clusterv1alpha1.Machine, _ k8stypes.UID) error {
	return nil
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	return map[string]string{}, nil
}

func (p *provider) SetMetricsForMachines(machine clusterv1alpha1.MachineList) error {
	return nil
}

func getClient(token string) (anxclient.Client, error) {
	tokenOpt := anxclient.TokenFromString(token)
	return anxclient.New(tokenOpt)
}

func getStatus(rawStatus *runtime.RawExtension) (*anxtypes.ProviderStatus, error) {
	var status anxtypes.ProviderStatus
	if rawStatus != nil && rawStatus.Raw != nil {
		if err := json.Unmarshal(rawStatus.Raw, &status); err != nil {
			return nil, err
		}
	}
	return &status, nil
}

// newError creates a terminal error matching to the provider interface.
func newError(reason common.MachineStatusError, msg string, args ...interface{}) error {
	return cloudprovidererrors.TerminalError{
		Reason:  reason,
		Message: fmt.Sprintf(msg, args...),
	}
}

func updateStatus(machine *clusterv1alpha1.Machine, status *anxtypes.ProviderStatus, updater cloudprovidertypes.MachineUpdater) error {
	rawStatus, err := json.Marshal(status)
	if err != nil {
		return err
	}
	err = updater(machine, func(machine *clusterv1alpha1.Machine) {
		machine.Status.ProviderStatus = &runtime.RawExtension{
			Raw: rawStatus,
		}
	})
	if err != nil {
		return err
	}

	return nil
}
