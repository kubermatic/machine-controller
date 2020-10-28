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

	anxclient "github.com/anexia-it/go-anxcloud/pkg/client"
	anxinfo "github.com/anexia-it/go-anxcloud/pkg/info"
	anxips "github.com/anexia-it/go-anxcloud/pkg/provisioning/ips"
	anxprog "github.com/anexia-it/go-anxcloud/pkg/provisioning/progress"
	anxvm "github.com/anexia-it/go-anxcloud/pkg/provisioning/vm"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
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
	client, err := getClient()
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

	info, err := anxinfo.Get(ctx, status.InstanceID, client)
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

	client, err := getClient()
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to get api-client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), anxtypes.CreateRequestTimeout)
	defer cancel()

	ips, err := anxips.GetFree(ctx, config.LocationID, config.VlanID, client)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to get ip pool", err)
	}
	if len(ips) < 1 {
		return nil, newError(common.InsufficientResourcesMachineError, "no ip address is available for this machine")
	}

	networkInterfaces := []anxvm.Network{{
		NICType: anxtypes.VmxNet3NIC,
		IPs:     []string{ips[0].Identifier},
		VLAN:    config.VlanID,
	}}

	vm := anxvm.NewDefinition(
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

	status, err := getStatus(machine.Status.ProviderStatus)
	if err != nil {
		return nil, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}

	if status.ProvisioningID == "" {
		klog.Infof("Provisioning a new machine %s", machine.ObjectMeta.Name)
		provisionResponse, err := anxvm.Provision(ctx, vm, client)
		if err != nil {
			return nil, newError(common.CreateMachineError, "instance provisioning failed: %v", err)
		}
		status.ProvisioningID = provisionResponse.Identifier
		if err := updateStatus(machine, status, providerData.Update); err != nil {
			return nil, newError(common.UpdateMachineError, "machine status update failed: %v", err)
		}
	}

	klog.Infof("Awaiting machine %s provisioning completion", machine.ObjectMeta.Name)
	instanceID, err := anxprog.AwaitCompletion(ctx, status.ProvisioningID, client)
	if err != nil {
		return nil, newError(common.CreateMachineError, "instance provisioning failed: %v", err)
	}
	klog.Infof("Machine %s provisioned", machine.ObjectMeta.Name)

	status.InstanceID = instanceID
	if err := updateStatus(machine, status, providerData.Update); err != nil {
		return nil, newError(common.UpdateMachineError, "machine status update failed: %v", err)
	}

	return p.Get(machine, providerData)
}

func (p *provider) Cleanup(machine *v1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (bool, error) {
	client, err := getClient()
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to get api-client: %v", err)
	}

	status, err := getStatus(machine.Status.ProviderStatus)
	if err != nil {
		return false, newError(common.InvalidConfigurationMachineError, "failed to get machine status: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), anxtypes.DeleteRequestTimeout)
	defer cancel()

	err = anxvm.Deprovision(ctx, status.InstanceID, false, client)
	if err != nil {
		var respErr *anxclient.ResponseError
		if errors.As(err, &respErr) && respErr.ErrorData.Code == http.StatusNotFound {
			return true, nil
		}
		return false, newError(common.DeleteMachineError, "failed to delete machine: %v", err)
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

func getClient() (anxclient.Client, error) {
	client, err := anxclient.NewAnyClientFromEnvs(true, nil)
	if err != nil {
		return nil, err
	}
	return client, nil
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
