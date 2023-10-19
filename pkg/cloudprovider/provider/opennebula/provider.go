/*
Copyright 2022 The Machine Controller Authors.

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

package opennebula

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/OpenNebula/one/src/oca/go/src/goca"
	"github.com/OpenNebula/one/src/oca/go/src/goca/schemas/shared"
	"github.com/OpenNebula/one/src/oca/go/src/goca/schemas/vm"
	"github.com/OpenNebula/one/src/oca/go/src/goca/schemas/vm/keys"

	"go.uber.org/zap"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	opennebulatypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/opennebula/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

type CloudProviderSpec struct {
	PassValidation bool `json:"passValidation"`
}

const (
	machineUIDContextKey = "K8S_MACHINE_UID"
)

// New returns a OpenNebula provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

type Config struct {
	// Auth details
	Username string
	Password string
	Endpoint string

	// Machine details
	CPU             *float64
	VCPU            *int
	Memory          *int
	Image           string
	Datastore       string
	DiskSize        *int
	Network         string
	EnableVNC       bool
	VMTemplateExtra map[string]string
}

func getClient(config *Config) *goca.Client {
	return goca.NewDefaultClient(goca.NewConfig(config.Username, config.Password, config.Endpoint))
}

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, error) {
	if provSpec.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, err
	}

	rawConfig, err := opennebulatypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, err
	}

	c := Config{}
	c.Username, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Username, "ONE_USERNAME")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"username\" field, error = %w", err)
	}

	c.Password, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Password, "ONE_PASSWORD")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"password\" field, error = %w", err)
	}

	c.Endpoint, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Endpoint, "ONE_ENDPOINT")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"endpoint\" field, error = %w", err)
	}

	c.CPU = rawConfig.CPU

	c.VCPU = rawConfig.VCPU

	c.Memory = rawConfig.Memory

	c.Image, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Image)
	if err != nil {
		return nil, nil, err
	}

	c.Datastore, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Datastore)
	if err != nil {
		return nil, nil, err
	}

	c.DiskSize = rawConfig.DiskSize

	c.Network, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Network)
	if err != nil {
		return nil, nil, err
	}

	c.EnableVNC, _, err = p.configVarResolver.GetConfigVarBoolValue(rawConfig.EnableVNC)
	if err != nil {
		return nil, nil, err
	}

	c.VMTemplateExtra = rawConfig.VMTemplateExtra

	return &c, pconfig, err
}

func (p *provider) Validate(_ context.Context, _ *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) error {
	_, pc, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	opennebulaCloudProviderSpec := CloudProviderSpec{}
	if err = json.Unmarshal(pc.CloudProviderSpec.Raw, &opennebulaCloudProviderSpec); err != nil {
		return fmt.Errorf("failed to parse cloud provider spec: %w", err)
	}

	return nil
}

func (p *provider) GetCloudConfig(_ clusterv1alpha1.MachineSpec) (string, string, error) {
	return "", "", nil
}

func (p *provider) Create(_ context.Context, _ *zap.SugaredLogger, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c)

	// build a template
	tpl := vm.NewTemplate()

	// add extra template vars first
	for key, value := range c.VMTemplateExtra {
		tpl.Add(keys.Template(key), value)
	}

	tpl.Add(keys.Name, machine.Spec.Name)
	tpl.CPU(*c.CPU).Memory(*c.Memory).VCPU(*c.VCPU)

	disk := tpl.AddDisk()
	disk.Add(shared.Image, c.Image)
	disk.Add(shared.Datastore, c.Datastore)
	disk.Add(shared.Size, *c.DiskSize)

	nic := tpl.AddNIC()
	nic.Add(shared.Network, c.Network)
	nic.Add(shared.Model, "virtio")

	if c.EnableVNC {
		err = tpl.AddIOGraphic(keys.GraphicType, "VNC")
		if err != nil {
			return nil, fmt.Errorf("failed to add graphic type to iographic in template: %w", err)
		}
		err = tpl.AddIOGraphic(keys.Listen, "0.0.0.0")
		if err != nil {
			return nil, fmt.Errorf("failed to add listen address to iographic in template: %w", err)
		}
	}

	err = tpl.AddCtx(keys.NetworkCtx, "YES")
	if err != nil {
		return nil, fmt.Errorf("failed to add network to context in template: %w", err)
	}
	err = tpl.AddCtx(keys.SSHPubKey, "$USER[SSH_PUBLIC_KEY]")
	if err != nil {
		return nil, fmt.Errorf("failed to add SSH public key to context in template: %w", err)
	}

	err = tpl.AddCtx(machineUIDContextKey, string(machine.UID))
	if err != nil {
		return nil, fmt.Errorf("failed to add machine UID to context in template: %w", err)
	}
	err = tpl.AddCtx("USER_DATA", base64.StdEncoding.EncodeToString([]byte(userdata)))
	if err != nil {
		return nil, fmt.Errorf("failed to add user data to context in template: %w", err)
	}
	err = tpl.AddCtx("USER_DATA_ENCODING", "base64")
	if err != nil {
		return nil, fmt.Errorf("failed to add user data encoding to context in template: %w", err)
	}
	err = tpl.AddCtx("SET_HOSTNAME", machine.Spec.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to add desired hostname to context in template: %w", err)
	}

	controller := goca.NewController(client)

	// create VM from the generated template above
	vmID, err := controller.VMs().Create(tpl.String(), false)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM: %w", err)
	}

	vm, err := controller.VM(vmID).Info(false)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch VM information: %w", err)
	}

	return &openNebulaInstance{vm}, nil
}

func (p *provider) Cleanup(_ context.Context, _ *zap.SugaredLogger, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	instance, err := p.get(machine)
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

	client := getClient(c)
	controller := goca.NewController(client)

	vmctrl := controller.VM(instance.vm.ID)
	err = vmctrl.TerminateHard()
	// ignore error of nonexistent machines by matching for "NO_EXISTS", the error string is something like "OpenNebula error [NO_EXISTS]: [one.vm.action] Error getting virtual machine [999914743]."
	if err != nil && !strings.Contains(err.Error(), "NO_EXISTS") {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to delete virtual machine, due to %v", err),
		}
	}

	return true, nil
}

func (p *provider) Get(_ context.Context, _ *zap.SugaredLogger, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	return p.get(machine)
}

func (p *provider) get(machine *clusterv1alpha1.Machine) (*openNebulaInstance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client := getClient(c)
	controller := goca.NewController(client)

	vmPool, err := controller.VMs().Info()
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to list virtual machines, due to %v", err),
		}
	}

	// first collect all IDs, the vm infos in the vmPool don't contain the context which has the uid
	var vmIDs []int
	for _, vm := range vmPool.VMs {
		if vm.Name != machine.Spec.Name {
			continue
		}

		vmIDs = append(vmIDs, vm.ID)
	}

	// go over each vm that matches the name and check if the uid is the same
	for _, vmID := range vmIDs {
		vm, err := controller.VM(vmID).Info(false)
		if err != nil {
			return nil, cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: fmt.Sprintf("failed to get info for VM %v, due to %v", vmID, err),
			}
		}

		uid, err := vm.Template.GetCtx(machineUIDContextKey)
		if err != nil {
			// ignore errors like "key blabla not found"
			continue
		}

		if uid == string(machine.UID) {
			return &openNebulaInstance{vm}, nil
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) AddDefaults(_ *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) MigrateUID(ctx context.Context, _ *zap.SugaredLogger, machine *clusterv1alpha1.Machine, newUID types.UID) error {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	instance, err := p.get(machine)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to get instance, due to %v", err),
		}
	}

	client := getClient(c)

	// get current template
	tpl := &instance.vm.Template
	contextVector, err := tpl.GetVector(keys.ContextVec)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to get VM template context vector, due to %v", err),
		}
	}

	// replace the old uid in context with the new one
	contextVector.Del(machineUIDContextKey)
	err = contextVector.AddPair(machineUIDContextKey, string(newUID))
	if err != nil {
		return fmt.Errorf("failed to add the new machine UID to context in template: %w", err)
	}

	// create a new template that only has the context vector in it so it gets properly replaced
	tpl = vm.NewTemplate()
	for _, pair := range contextVector.Pairs {
		key := pair.XMLName.Local
		value := pair.Value
		err = tpl.AddCtx(keys.Context(key), value)
		if err != nil {
			return fmt.Errorf("failed to add %s to context in template: %w", key, err)
		}
	}

	// finally, update the VM template
	controller := goca.NewController(client)
	vmCtrl := controller.VM(instance.vm.ID)
	err = vmCtrl.UpdateConf(tpl.String())
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to update VM template, due to %v", err),
		}
	}

	return nil
}

func (p *provider) MachineMetricsLabels(_ *clusterv1alpha1.Machine) (map[string]string, error) {
	return map[string]string{}, nil
}

func (p *provider) SetMetricsForMachines(_ clusterv1alpha1.MachineList) error {
	return nil
}

type openNebulaInstance struct {
	vm *vm.VM
}

func (i *openNebulaInstance) Name() string {
	return i.vm.Name
}

func (i *openNebulaInstance) ID() string {
	return strconv.Itoa(i.vm.ID)
}

func (i *openNebulaInstance) ProviderID() string {
	return "opennebula://" + strconv.Itoa(i.vm.ID)
}

func (i *openNebulaInstance) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{}

	for _, nic := range i.vm.Template.GetNICs() {
		ip, _ := nic.Get(shared.IP)
		addresses[ip] = v1.NodeInternalIP
	}

	return addresses
}

func (i *openNebulaInstance) Status() instance.Status {
	// state is the general state of the VM, lcmState is the state of the life-cycle manager of the VM
	// lcmState is anything else other than LcmInit when the VM's state is Active
	state, lcmState, _ := i.vm.State()
	switch state {
	case vm.Init, vm.Pending, vm.Hold:
		return instance.StatusCreating
	case vm.Active:
		switch lcmState {
		case vm.LcmInit, vm.Prolog, vm.Boot:
			return instance.StatusCreating
		case vm.Epilog:
			return instance.StatusDeleting
		default:
			return instance.StatusRunning
		}
	case vm.Done:
		return instance.StatusDeleted
	default:
		return instance.StatusUnknown
	}
}
