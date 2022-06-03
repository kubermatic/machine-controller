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

package vmwareclouddirector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/vmware/go-vcloud-director/v2/govcd"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	vcdtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/vmwareclouddirector/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
)

const (
	defaultDiskType       = "paravirtual"
	defaultStorageProfile = "*"
	defaultDiskIOPS       = 0
)

type NetworkType string

const (
	VAppNetworkType NetworkType = "vapp"
	OrgNetworkType  NetworkType = "org"
	// Network with a NIC that is not attached to any network.
	NoneNetworkType NetworkType = "none"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

type Auth struct {
	Username      string
	Password      string
	Organization  string
	URL           string
	VDC           string
	AllowInsecure bool
}

type Config struct {
	Auth `json:",inline"`

	// VM configuration.
	VApp            string
	Template        string
	Catalog         string
	PlacementPolicy *string
	SizingPolicy    *string

	// Network configuration.
	Network          string
	IPAllocationMode vcdtypes.IPAllocationMode

	// Compute configuration.
	CPUs     int64
	CPUCores int64
	MemoryMB int64

	// Storage configuration.
	DiskSizeGB     *int64
	DiskBusType    *string
	DiskIOPS       *int64
	StorageProfile *string

	// Metadata configuration.
	Metadata *map[string]string
}

// New returns a VMware Cloud Director provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

// Ensures that Server implements Instance interface.
var _ instance.Instance = &Server{}

// Server holds VMware Cloud Director VM information.
type Server struct {
	name      string
	id        string
	status    instance.Status
	addresses map[string]corev1.NodeAddressType
}

func (s Server) Name() string {
	return s.name
}

func (s Server) ID() string {
	return s.id
}

func (s Server) Addresses() map[string]corev1.NodeAddressType {
	return s.addresses
}

func (s Server) Status() instance.Status {
	return s.status
}

func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	_, _, rawConfig, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return spec, err
	}

	// Set defaults.
	if rawConfig.IPAllocationMode == "" {
		rawConfig.IPAllocationMode = vcdtypes.DHCPIPAllocationMode
	}

	// These defaults will have no effect if DiskSizeGB is not specified
	if rawConfig.DiskBusType == nil {
		rawConfig.DiskBusType = pointer.String(defaultDiskType)
	}
	if rawConfig.DiskIOPS == nil {
		rawConfig.DiskIOPS = pointer.Int64(defaultDiskIOPS)
	}
	spec.ProviderSpec.Value, err = setProviderSpec(*rawConfig, spec.ProviderSpec)
	return spec, err
}

func (p *provider) Cleanup(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, fmt.Errorf("failed to parse config: %w", err)
	}

	client, err := NewClient(c.Username, c.Password, c.Organization, c.URL, c.VDC, c.AllowInsecure)
	if err != nil {
		return false, fmt.Errorf("failed to create VMware Cloud Director client: %w", err)
	}

	vm, err := client.GetVMByName(c.VApp, machine.Name)
	if err != nil {
		if errors.Is(err, cloudprovidererrors.ErrInstanceNotFound) {
			return true, nil
		}
		return false, err
	}

	vmStatus, err := vm.GetStatus()
	if err != nil {
		return false, fmt.Errorf("failed to get VM status: %w", err)
	}

	// Turn off VM if it's `ON`
	if vmStatus == "POWERED_ON" {
		task, err := vm.PowerOff()
		if err != nil {
			return false, fmt.Errorf("failed to turn off VM: %w", err)
		}
		if err = task.WaitTaskCompletion(); err != nil {
			return false, fmt.Errorf("error waiting for VM power off task to complete: %w", err)
		}
	}

	if err := vm.Delete(); err != nil {
		return false, fmt.Errorf("failed to destroy vm %s: %w", vm.VM.Name, err)
	}
	return true, nil
}

func (p *provider) Create(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	vm, err := p.create(ctx, machine, userdata)
	if err != nil {
		_, cleanupErr := p.Cleanup(ctx, machine, data)
		if cleanupErr != nil {
			return nil, fmt.Errorf("cleaning up failed with err %v after creation failed with err %w", cleanupErr, err)
		}
		return nil, err
	}
	return vm, nil
}

func (p *provider) create(ctx context.Context, machine *clusterv1alpha1.Machine, userdata string) (instance.Instance, error) {
	c, providerConfig, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	client, err := NewClient(c.Username, c.Password, c.Organization, c.URL, c.VDC, c.AllowInsecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create VMware Cloud Director client: %w", err)
	}

	// Fetch the organization, VDC, and vApp resources.
	org, vdc, vapp, err := client.GetOrganizationVDCAndVapp(c.VApp)
	if err != nil {
		return nil, err
	}

	// 1. Create Standalone VM from template.
	err = createVM(client, machine, c, org, vdc, vapp)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM: %w", err)
	}

	// 2. Fetch updated vApp
	err = vapp.Refresh()
	if err != nil {
		return nil, fmt.Errorf("failed to get updated vApp '%s' after recompoisition: %w", c.VApp, err)
	}

	// 3. Fetch updated VM
	vm, err := vapp.GetVMByName(machine.Name, true)
	if err != nil {
		return nil, err
	}

	// 4. Perform VM recomposition for compute and disks
	vm, err = recomposeComputeAndDisk(c, vm)
	if err != nil {
		return nil, err
	}

	// 5. Before powering on the VM, configure customization to attach userdata with the VM
	// update guest properties.
	err = setUserData(userdata, vm, providerConfig)
	if err != nil {
		return nil, err
	}

	// 6. Fetch updated VM.
	err = vm.Refresh()
	if err != nil {
		return nil, err
	}

	// 7. Add Metadata to VM.
	err = addMetadata(vm, c.Metadata)
	if err != nil {
		return nil, err
	}

	// 8. Set computer name for the VM
	err = setComputerName(vm, machine.Name)
	if err != nil {
		return nil, err
	}

	// 9. Finally power on the VM after performing all required actions.
	task, err := vm.PowerOn()
	if err != nil {
		return nil, fmt.Errorf("failed to turn on VM: %w", err)
	}
	if err = task.WaitTaskCompletion(); err != nil {
		return nil, fmt.Errorf("error waiting for VM bootstrap  to complete: %w", err)
	}

	return p.getInstance(vm)
}

func (p *provider) Get(ctx context.Context, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	client, err := NewClient(c.Username, c.Password, c.Organization, c.URL, c.VDC, c.AllowInsecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create VMware Cloud Director client: %w", err)
	}

	vm, err := client.GetVMByName(c.VApp, machine.Name)
	if err != nil {
		return nil, err
	}

	return p.getInstance(vm)
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, *vcdtypes.RawConfig, error) {
	if provSpec.Value == nil {
		return nil, nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, nil, err
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig, err := vcdtypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, nil, err
	}

	c := Config{}
	c.Username, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Username, "VCD_USER")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"username\" field, error = %w", err)
	}

	c.Password, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Password, "VCD_PASSWORD")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"password\" field, error = %w", err)
	}

	c.Organization, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Organization, "VCD_ORG")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"organization\" field, error = %w", err)
	}

	c.URL, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.URL, "VCD_URL")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"url\" field, error = %w", err)
	}

	c.VDC, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.VDC, "VCD_VDC")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"vdc\" field, error = %w", err)
	}

	c.AllowInsecure, err = p.configVarResolver.GetConfigVarBoolValueOrEnv(rawConfig.AllowInsecure, "VCD_ALLOW_UNVERIFIED_SSL")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"allowInsecure\" field, error = %w", err)
	}

	c.VApp, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VApp)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Template, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Template)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Catalog, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Catalog)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Network, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Network)
	if err != nil {
		return nil, nil, nil, err
	}

	c.IPAllocationMode = rawConfig.IPAllocationMode

	if rawConfig.DiskSizeGB != nil && *rawConfig.DiskSizeGB < 0 {
		return nil, nil, nil, fmt.Errorf("value for \"diskSizeGB\" should either be nil or greater than or equal to 0")
	}
	c.DiskSizeGB = rawConfig.DiskSizeGB

	if rawConfig.DiskIOPS != nil && *rawConfig.DiskIOPS < 0 {
		return nil, nil, nil, fmt.Errorf("value for \"diskIOPS\" should either be nil or greater than or equal to 0")
	}
	c.DiskIOPS = rawConfig.DiskIOPS

	if rawConfig.CPUs <= 0 {
		return nil, nil, nil, fmt.Errorf("value for \"cpus\" should be greater than 0")
	}
	c.CPUs = rawConfig.CPUs

	if rawConfig.CPUCores <= 0 {
		return nil, nil, nil, fmt.Errorf("value for \"cpuCores\" should be greater than 0")
	}
	c.CPUCores = rawConfig.CPUCores

	if rawConfig.MemoryMB <= 4 {
		return nil, nil, nil, fmt.Errorf("value for \"memoryMB\" should be greater than 0")
	}
	if rawConfig.MemoryMB%4 != 0 {
		return nil, nil, nil, fmt.Errorf("value for \"memoryMB\" should be a multiple of 4")
	}
	c.MemoryMB = rawConfig.MemoryMB

	c.DiskBusType = rawConfig.DiskBusType
	c.StorageProfile = rawConfig.StorageProfile
	c.Metadata = rawConfig.Metadata
	return &c, pconfig, rawConfig, err
}

func (p *provider) getInstance(vm *govcd.VM) (instance.Instance, error) {
	vmStatus, err := vm.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get VM status: %w", err)
	}

	var status instance.Status

	switch vmStatus {
	case "POWERED_ON":
		status = instance.StatusRunning
	case "POWERED_OFF", "PARTIALLY_POWERED_OFF":
		status = instance.StatusCreating
	default:
		status = instance.StatusUnknown
	}

	addresses := make(map[string]corev1.NodeAddressType)
	if vm.VM.NetworkConnectionSection != nil && vm.VM.NetworkConnectionSection.NetworkConnection != nil {
		for _, nic := range vm.VM.NetworkConnectionSection.NetworkConnection {
			if nic.ExternalIPAddress != "" {
				addresses[nic.ExternalIPAddress] = corev1.NodeExternalIP
			}
			if nic.IPAddress != "" {
				addresses[nic.IPAddress] = corev1.NodeInternalIP
			}
		}
	}

	return Server{name: vm.VM.Name, status: status, addresses: addresses, id: vm.VM.ID}, nil
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["size"] = fmt.Sprintf("%d-cpus-%d-mb", c.CPUs, c.MemoryMB)
		labels["vapp"] = c.VApp
		labels["vdc"] = c.VDC
		labels["organization"] = c.Organization
	}

	return labels, err
}

func (p *provider) MigrateUID(_ context.Context, _ *clusterv1alpha1.Machine, _ types.UID) error {
	return nil
}

func (p *provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	return nil
}

func (p *provider) Validate(_ context.Context, spec clusterv1alpha1.MachineSpec) error {
	c, _, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	client, err := NewClient(c.Username, c.Password, c.Organization, c.URL, c.VDC, c.AllowInsecure)
	if err != nil {
		return fmt.Errorf("failed to create VMware Cloud Director client: %w", err)
	}

	// Ensure that the organization, VDC, and vApp exists.
	org, vdc, vapp, err := client.GetOrganizationVDCAndVapp(c.VApp)
	if err != nil {
		return err
	}

	// Ensure that the catalog exists.
	catalog, err := org.GetCatalogByNameOrId(c.Catalog, true)
	if err != nil {
		return fmt.Errorf("failed to get catalog '%s': %w", c.Catalog, err)
	}

	// Ensure that the template exists in the catalog
	// Catalog item can be a vApp template OVA or media ISO file.
	catalogItem, err := catalog.GetCatalogItemByNameOrId(c.Template, true)
	if err != nil {
		return fmt.Errorf("failed to get template '%s' in catalog '%s': %w", c.Template, c.Catalog, err)
	}
	if c.DiskSizeGB != nil && catalogItem.CatalogItem.Size > *c.DiskSizeGB {
		return fmt.Errorf("diskSizeGB '%v' cannot be less than the template size '%v': %w", *c.DiskSizeGB, catalogItem.CatalogItem.Size, err)
	}

	// Ensure that the network exists
	// It can either be a vApp network or a vApp Org network.
	_, err = GetVappNetworkType(c.Network, *vapp)
	if err != nil {
		return fmt.Errorf("failed to get network '%s' for vapp '%s': %w", c.Network, c.VApp, err)
	}

	if c.SizingPolicy != nil || c.PlacementPolicy != nil {
		allPolicies, err := org.GetAllVdcComputePolicies(url.Values{})
		if err != nil {
			return fmt.Errorf("failed to get template all VDC compute policies: %w", err)
		}

		if c.SizingPolicy != nil && *c.SizingPolicy != "" {
			sizingPolicy := getComputePolicy(*c.SizingPolicy, allPolicies)
			if sizingPolicy == nil {
				return fmt.Errorf("sizing policy '%s' doesn't exist", *c.SizingPolicy)
			}
		}

		if c.PlacementPolicy != nil && *c.PlacementPolicy != "" {
			placementPolicy := getComputePolicy(*c.PlacementPolicy, allPolicies)
			if placementPolicy == nil {
				return fmt.Errorf("placement policy '%s' doesn't exist", *c.SizingPolicy)
			}
		}
	}

	// Ensure that the storage profile exists.
	if c.StorageProfile != nil && *c.StorageProfile != defaultStorageProfile {
		_, err = vdc.FindStorageProfileReference(*c.StorageProfile)
		if err != nil {
			return fmt.Errorf("failed to get storage profile '%s': %w", *c.StorageProfile, err)
		}
	}
	return nil
}

func setProviderSpec(rawConfig vcdtypes.RawConfig, provSpec clusterv1alpha1.ProviderSpec) (*runtime.RawExtension, error) {
	if provSpec.Value == nil {
		return nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
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
