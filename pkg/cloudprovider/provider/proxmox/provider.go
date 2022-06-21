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

package proxmox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	proxmoxtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/proxmox/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	defaultCIStoragePath = "/var/lib/vz"
	defaultCIStorageName = "local"
	defaultDiskName      = "virtio0"
)

type Config struct {
	Endpoint    string
	UserID      string
	Token       string
	TLSInsecure bool
	ProxyURL    string

	CIStorageName          string
	CIStoragePath          string
	CIStorageSSHPrivateKey string

	VMTemplateID int
	CPUSockets   *int
	CPUCores     *int
	MemoryMB     int
	DiskSizeGB   *int
	DiskName     *string
}

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// Server holds the proxmox VM information.
type Server struct {
	configQemu *proxmox.ConfigQemu
	vmRef      *proxmox.VmRef
	status     instance.Status
	addresses  map[string]corev1.NodeAddressType
}

// Ensures that Server implements Instance interface.
var _ instance.Instance = &Server{}

// Ensures that provider implements Provider interface.
var _ cloudprovidertypes.Provider = &provider{}

// Name returns the instance name.
func (server *Server) Name() string {
	return server.configQemu.Name
}

// ID returns the instance identifier.
func (server *Server) ID() string {
	return fmt.Sprintf("node-%s-vm-%d", server.vmRef.Node(), server.vmRef.VmId())
}

// Addresses returns a list of addresses associated with the instance.
func (server *Server) Addresses() map[string]corev1.NodeAddressType {
	return server.addresses
}

// Status returns the instance status.
func (server *Server) Status() instance.Status {
	return server.status
}

// ProviderID is n/a for Proxmox.
func (*Server) ProviderID() string {
	return ""
}

func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	provider := &provider{configVarResolver: configVarResolver}
	return provider
}

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *proxmoxtypes.RawConfig, error) {
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

	rawConfig, err := proxmoxtypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, err
	}

	config := Config{}

	config.Endpoint, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Endpoint, "PM_API_ENDPOINT")
	if err != nil {
		return nil, nil, err
	}

	config.UserID, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.UserID, "PM_API_USER_ID")
	if err != nil {
		return nil, nil, err
	}

	config.Token, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Token, "PM_API_TOKEN")
	if err != nil {
		return nil, nil, err
	}

	config.TLSInsecure, err = p.configVarResolver.GetConfigVarBoolValueOrEnv(rawConfig.AllowInsecure, "PM_TLS_INSECURE")
	if err != nil {
		return nil, nil, err
	}

	config.ProxyURL, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.ProxyURL, "PM_PROXY_URL")
	if err != nil {
		return nil, nil, err
	}

	config.CIStorageSSHPrivateKey, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.CIStorageSSHPrivateKey)
	if err != nil {
		return nil, nil, err
	}

	config.CIStorageName = *rawConfig.CIStorageName
	config.CIStoragePath = *rawConfig.CIStoragePath

	config.VMTemplateID = rawConfig.VMTemplateID
	config.CPUCores = rawConfig.CPUCores
	config.CPUSockets = rawConfig.CPUSockets
	config.MemoryMB = rawConfig.MemoryMB
	config.DiskName = rawConfig.DiskName
	config.DiskSizeGB = rawConfig.DiskSizeGB

	return &config, rawConfig, nil
}

// AddDefaults will read the MachineSpec and apply defaults for provider specific fields.
func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	_, rawConfig, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return spec, err
	}

	if rawConfig.CIStorageName == nil {
		rawConfig.CIStorageName = proxmox.PointerString(defaultCIStorageName)
	}

	if rawConfig.CIStoragePath == nil {
		rawConfig.CIStoragePath = proxmox.PointerString(defaultCIStoragePath)
	}

	if rawConfig.DiskName == nil {
		rawConfig.DiskName = proxmox.PointerString(defaultDiskName)
	}

	spec.ProviderSpec.Value, err = setProviderSpec(*rawConfig, spec.ProviderSpec)
	return spec, err
}

func (p *provider) Validate(ctx context.Context, spec clusterv1alpha1.MachineSpec) error {
	config, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to parse machineSpec: %v", err),
		}
	}

	c, err := GetClientSet(config)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to construct client: %v", err),
		}
	}

	templateExists, err := c.checkTemplateExists(config.VMTemplateID)
	if err != nil {
		return err
	}
	if !templateExists {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("%q is not a VM template", config.VMTemplateID),
		}
	}

	return nil
}

func (p *provider) Get(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to parse machineSpec: %v", err),
		}
	}

	c, err := GetClientSet(config)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to construct client: %v", err),
		}
	}

	vmr, err := c.getVMRefByName(machine.Name)
	if err != nil {
		return nil, err
	}

	configQemu, err := proxmox.NewConfigQemuFromApi(vmr, c.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config of VM: %w", err)
	}

	addresses, err := c.getIPsByVMRef(vmr)
	if err != nil {
		return nil, fmt.Errorf("failed to get IP addresses of VM: %w", err)
	}

	var status instance.Status
	vmState, err := c.GetVmState(vmr)
	if err != nil {
		return nil, fmt.Errorf("failed to get state of VM: %w", err)
	}
	switch vmState["status"] {
	case "running":
		status = instance.StatusRunning
	case "stopped":
		status = instance.StatusCreating
	default:
		status = instance.StatusUnknown
	}

	return &Server{
		vmRef:      vmr,
		configQemu: configQemu,
		addresses:  addresses,
		status:     status,
	}, nil
}

func (*provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) Create(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to parse machineSpec: %v", err),
		}
	}

	c, err := GetClientSet(config)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to construct client: %v", err),
		}
	}

	vm, err := p.create(ctx, c, config, machine, userdata)
	if err != nil {
		_, cleanupErr := p.Cleanup(ctx, machine, data)
		if cleanupErr != nil {
			return nil, fmt.Errorf("cleaning up failed with err %v after creation failed with err %w", cleanupErr, err)
		}
		return nil, err
	}

	return vm, nil
}

func (p *provider) create(ctx context.Context, c *ClientSet, config *Config, machine *clusterv1alpha1.Machine, userdata string) (*Server, error) {
	sourceVmr := proxmox.NewVmRef(config.VMTemplateID)
	nodes, err := c.getNodeList()
	if err != nil && len(nodes.Data) == 0 {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: "failed to retrieve any nodes",
		}
	}
	// The template needs to be set as HA. This makes it available on any node,
	// so it's sufficient to pick any existing as source node.
	sourceVmr.SetNode(nodes.Data[0].Name)

	if err := c.CheckVmRef(sourceVmr); err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to retrieve VM template %q", config.VMTemplateID),
		}
	}

	vmID, err := c.GetNextID(0)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to get next available VM ID: %v", err),
		}
	}

	configQemu := &proxmox.ConfigQemu{
		Name:      machine.Name,
		VmID:      vmID,
		FullClone: proxmox.PointerInt(0),
	}

	targetVmr := proxmox.NewVmRef(vmID)
	targetNode, err := c.selectNode(*config.CPUSockets, config.MemoryMB)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to select target node: %v", err),
		}
	}
	targetVmr.SetNode(targetNode)

	err = configQemu.CloneVm(sourceVmr, targetVmr, c.Client)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.CreateMachineError,
			Message: fmt.Sprintf("failed to create VM: %v", err),
		}
	}

	configClone, err := proxmox.NewConfigQemuFromApi(targetVmr, c.Client)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.CreateMachineError,
			Message: fmt.Sprintf("failed to fetch config of newly created VM: %v", err),
		}
	}

	configClone.VmID = vmID
	configClone.QemuSockets = *config.CPUSockets
	configClone.QemuCores = *config.CPUCores
	configClone.Memory = config.MemoryMB

	filePath, err := c.copyUserdata(ctx, targetNode, config.CIStoragePath, config.UserID, userdata, config.CIStorageSSHPrivateKey, vmID)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.CreateMachineError,
			Message: fmt.Sprintf("failed to upload cloud-init userdata: %v", err),
		}
	}
	configClone.CIcustom = fmt.Sprintf("user=%s:%s", config.CIStorageName, filePath)
	configClone.Ipconfig0 = "ip=dhcp"

	err = configClone.UpdateConfig(targetVmr, c.Client)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.CreateMachineError,
			Message: fmt.Sprintf("failed to update VM size: %v", err),
		}
	}

	_, err = c.ResizeQemuDiskRaw(targetVmr, *config.DiskName, fmt.Sprintf("%dG", *config.DiskSizeGB))
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.CreateMachineError,
			Message: fmt.Sprintf("failed to update disk size: %v", err),
		}
	}

	exitStatus, err := c.StartVm(targetVmr)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.CreateMachineError,
			Message: fmt.Sprintf("failed to start VM: %v", err),
		}
	}
	if exitStatus != exitStatusSuccess {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.CreateMachineError,
			Message: fmt.Sprintf("starting VM returned unexpected status: %q", exitStatus),
		}
	}

	deadline := time.Now().Add(time.Second * taskTimeout)
	for time.Now().Before(deadline) {
		if _, err = c.QemuAgentPing(targetVmr); err == nil {
			break
		}
		time.Sleep(time.Second)
	}

	addresses, err := c.getIPsByVMRef(targetVmr)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.CreateMachineError,
			Message: fmt.Sprintf("failed to get IP addresses of VM: %v", err),
		}
	}

	return &Server{
		vmRef:      targetVmr,
		configQemu: configClone,
		addresses:  addresses,
		status:     instance.StatusRunning,
	}, nil
}

func (p *provider) Cleanup(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to parse machineSpec: %v", err),
		}
	}

	c, err := GetClientSet(config)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to construct client: %v", err),
		}
	}

	vmr, err := c.getVMRefByName(machine.Name)
	if err != nil {
		if cloudprovidererrors.IsNotFound(err) {
			// VM is already gone
			return true, nil
		}
		return false, err
	}

	exitStatusStop, err := c.StopVm(vmr)
	if err != nil {
		return false, fmt.Errorf("failed to start VM: %w", err)
	}
	if exitStatusStop != exitStatusSuccess {
		return false, fmt.Errorf("starting VM returned unexpected status: %q", exitStatusStop)
	}

	deleteParams := map[string]interface{}{
		// Clean all disks matching this VM ID even not referenced in the current VM config.
		"destroy-unreferenced-disks": true,
		// Remove all traces of this VM ID (backup, replication, HA)
		"purge": true,
	}
	exitStatusDelete, err := c.DeleteVmParams(vmr, deleteParams)

	return exitStatusDelete == exitStatusSuccess, err
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	config, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return labels, fmt.Errorf("failed to parse config: %w", err)
	}

	labels["size"] = fmt.Sprintf("%d-cpus-%d-mb", config.CPUSockets, config.MemoryMB)
	labels["templateID"] = fmt.Sprintf("%d", config.VMTemplateID)

	return labels, nil
}

func (*provider) MigrateUID(ctx context.Context, machine *clusterv1alpha1.Machine, newUID types.UID) error {
	return nil
}

func (*provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	return nil
}

func setProviderSpec(rawConfig proxmoxtypes.RawConfig, s clusterv1alpha1.ProviderSpec) (*runtime.RawExtension, error) {
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
