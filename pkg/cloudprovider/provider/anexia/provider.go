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

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/common/ssh"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	anxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/anexia/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	anx "github.com/anexia-it/go-anxcloud/pkg"
	anxclient "github.com/anexia-it/go-anxcloud/pkg/client"
	anxvm "github.com/anexia-it/go-anxcloud/pkg/vsphere/provisioning/vm"
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
	SSHKey     string
}

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

func (p *provider) getConfig(s v1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, error) {
	if s.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerSpec.value is nil")
	}
	pConfig := providerconfigtypes.Config{}
	err := json.Unmarshal(s.Value.Raw, &pConfig)
	if err != nil {
		return nil, nil, err
	}

	rawConfig := anxtypes.RawConfig{}
	if err = json.Unmarshal(pConfig.CloudProviderSpec.Raw, &rawConfig); err != nil {
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

	return &c, &pConfig, nil
}

// New returns an Anexia provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

// AddDefaults adds omitted optional values to the given MachineSpec
func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	return spec, nil
}

// Validate returns success or failure based according to its FakeCloudProviderSpec
func (p *provider) Validate(machinespec v1alpha1.MachineSpec) error {
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

func (p *provider) Get(machine *v1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	apiClient, err := getClient()
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to get api-client: %v", err)
	}

	status, err := getStatus(machine.Status.ProviderStatus)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}
	if status.InstanceID == "" {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), anxtypes.GetRequestTimeout)
	defer cancel()

	info, err := apiClient.VSphere().Info().Get(ctx, status.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("failed get machine info: %w", err)
	}

	return &anexiaInstance{
		info: &info,
	}, nil
}

func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (string, string, error) {
	return "", "", nil
}

// Create creates a cloud instance according to the given machine
func (p *provider) Create(machine *v1alpha1.Machine, providerData *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to parse MachineSpec: %v", err)
	}

	apiClient, err := getClient()
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to get api-client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), anxtypes.CreateRequestTimeout)
	defer cancel()

	status, err := getStatus(machine.Status.ProviderStatus)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}

	if status.ProvisioningID == "" {
		ips, err := apiClient.VSphere().Provisioning().IPs().GetFree(ctx, config.LocationID, config.VlanID)
		if err != nil {
			return nil, newError(common.InvalidConfigurationMachineError, "failed to get ip pool: %v", err)
		}
		if len(ips) < 1 {
			return nil, newError(common.InsufficientResourcesMachineError, "no ip address is available for this machine")
		}

		ipID := ips[0].Identifier
		networkInterfaces := []anxvm.Network{{
			NICType: anxtypes.VmxNet3NIC,
			IPs:     []string{ipID},
			VLAN:    config.VlanID,
		}}

		vm := apiClient.VSphere().Provisioning().VM().NewDefinition(
			config.LocationID,
			"templates",
			config.TemplateID,
			machine.ObjectMeta.Name,
			config.CPUs,
			config.Memory,
			config.DiskSize,
			networkInterfaces,
		)

		vm.Script = base64.StdEncoding.EncodeToString(
			[]byte(fmt.Sprintf("anexia: true\n\n%s", userdata)),
		)

		sshKey, err := ssh.NewKey()
		if err != nil {
			return nil, newError(common.CreateMachineError, "failed to generate ssh key: %v", err)
		}
		vm.SSH = sshKey.PublicKey

		provisionResponse, err := apiClient.VSphere().Provisioning().VM().Provision(ctx, vm)
		if err != nil {
			return nil, newError(common.CreateMachineError, "instance provisioning failed: %v", err)
		}

		status.ProvisioningID = provisionResponse.Identifier
		status.IPAllocationID = ipID
		if err := updateStatus(machine, status, providerData.Update); err != nil {
			return nil, newError(common.UpdateMachineError, "machine status update failed: %v", err)
		}
	}

	instanceID, err := apiClient.VSphere().Provisioning().Progress().AwaitCompletion(ctx, status.ProvisioningID)
	if err != nil {
		return nil, newError(common.CreateMachineError, "instance provisioning failed: %v", err)
	}

	status.InstanceID = instanceID
	if err := updateStatus(machine, status, providerData.Update); err != nil {
		return nil, newError(common.UpdateMachineError, "machine status update failed: %v", err)
	}

	return p.Get(machine, providerData)
}

func (p *provider) Cleanup(machine *v1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (bool, error) {
	apiClient, err := getClient()
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to get api-client: %v", err)
	}

	status, err := getStatus(machine.Status.ProviderStatus)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), anxtypes.DeleteRequestTimeout)
	defer cancel()

	err = apiClient.VSphere().Provisioning().VM().Deprovision(ctx, status.InstanceID, false)
	if err != nil {
		var respErr *anxclient.ResponseError
		// Only error if the error was not "not found"
		if !(errors.As(err, &respErr) && respErr.ErrorData.Code == http.StatusNotFound) {
			return false, newError(common.DeleteMachineError, "failed to delete machine: %v", err)
		}
	}

	err = apiClient.IPAM().Address().Delete(ctx, status.IPAllocationID)
	if err != nil {
		var respErr *anxclient.ResponseError
		// Only error if the error was not "not found"
		if !(errors.As(err, &respErr) && respErr.ErrorData.Code == http.StatusNotFound) {
			return false, newError(common.DeleteMachineError, "failed to delete machine ip allocation: %v", err)
		}
	}

	return true, nil
}

func (p *provider) MigrateUID(_ *v1alpha1.Machine, _ k8stypes.UID) error {
	return nil
}

func (p *provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	return map[string]string{}, nil
}

func (p *provider) SetMetricsForMachines(machine v1alpha1.MachineList) error {
	return nil
}

func getClient() (anx.API, error) {
	client, err := anxclient.NewAnyClientFromEnvs(true, nil)
	if err != nil {
		return nil, err
	}
	return anx.NewAPI(client), nil
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

func updateStatus(machine *v1alpha1.Machine, status *anxtypes.ProviderStatus, updater cloudprovidertypes.MachineUpdater) error {
	rawStatus, err := json.Marshal(status)
	if err != nil {
		return err
	}
	err = updater(machine, func(machine *v1alpha1.Machine) {
		machine.Status.ProviderStatus = &runtime.RawExtension{
			Raw: rawStatus,
		}
	})
	if err != nil {
		return err
	}

	return nil
}
