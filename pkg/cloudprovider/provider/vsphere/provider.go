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

package vsphere

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	vspheretypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/vsphere/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a VSphere provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	provider := &provider{configVarResolver: configVarResolver}
	return provider
}

// Config contains vSphere provider configuration.
type Config struct {
	TemplateVMName   string
	VMNetName        string
	Username         string
	Password         string
	VSphereURL       string
	Datacenter       string
	Folder           string
	ResourcePool     string
	Datastore        string
	DatastoreCluster string
	AllowInsecure    bool
	CPUs             int32
	MemoryMB         int64
	DiskSizeGB       *int64
}

// Ensures that Server implements Instance interface.
var _ instance.Instance = &Server{}

// Server holds vSphere server information.
type Server struct {
	name      string
	id        string
	status    instance.Status
	addresses map[string]corev1.NodeAddressType
}

func (vsphereServer Server) Name() string {
	return vsphereServer.name
}

func (vsphereServer Server) ID() string {
	return vsphereServer.id
}

func (vsphereServer Server) Addresses() map[string]corev1.NodeAddressType {
	return vsphereServer.addresses
}

func (vsphereServer Server) Status() instance.Status {
	return vsphereServer.status
}

// Ensures that provider implements Provider interface.
var _ cloudprovidertypes.Provider = &provider{}

func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, *vspheretypes.RawConfig, error) {
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

	rawConfig, err := vspheretypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, nil, err
	}

	c := Config{}
	c.TemplateVMName, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.TemplateVMName)
	if err != nil {
		return nil, nil, nil, err
	}

	c.VMNetName, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VMNetName)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Username, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Username, "VSPHERE_USERNAME")
	if err != nil {
		return nil, nil, nil, err
	}

	c.Password, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Password, "VSPHERE_PASSWORD")
	if err != nil {
		return nil, nil, nil, err
	}

	c.VSphereURL, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.VSphereURL, "VSPHERE_ADDRESS")
	if err != nil {
		return nil, nil, nil, err
	}

	c.Datacenter, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Datacenter)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Folder, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Folder)
	if err != nil {
		return nil, nil, nil, err
	}

	c.ResourcePool, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ResourcePool)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Datastore, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Datastore)
	if err != nil {
		return nil, nil, nil, err
	}

	c.DatastoreCluster, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.DatastoreCluster)
	if err != nil {
		return nil, nil, nil, err
	}

	c.AllowInsecure, err = p.configVarResolver.GetConfigVarBoolValueOrEnv(rawConfig.AllowInsecure, "VSPHERE_ALLOW_INSECURE")
	if err != nil {
		return nil, nil, nil, err
	}

	c.CPUs = rawConfig.CPUs
	c.MemoryMB = rawConfig.MemoryMB
	c.DiskSizeGB = rawConfig.DiskSizeGB

	return &c, pconfig, rawConfig, nil
}

func (p *provider) Validate(spec clusterv1alpha1.MachineSpec) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	config, pc, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to get config: %v", err)
	}

	session, err := NewSession(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create vCenter session: %v", err)
	}
	defer session.Logout()

	// Only and only one between datastore and datastre cluster should be
	// present, otherwise an error is raised.
	if config.DatastoreCluster != "" && config.Datastore == "" {
		if _, err := session.Finder.DatastoreCluster(ctx, config.DatastoreCluster); err != nil {
			return fmt.Errorf("failed to get datastore cluster %s: %v", config.DatastoreCluster, err)
		}
	} else if config.Datastore != "" && config.DatastoreCluster == "" {
		if _, err := session.Finder.Datastore(ctx, config.Datastore); err != nil {
			return fmt.Errorf("failed to get datastore %s: %v", config.Datastore, err)
		}
	} else {
		return fmt.Errorf("one between datastore and datastore cluster should be specified: %v", err)
	}

	if _, err := session.Finder.Folder(ctx, config.Folder); err != nil {
		return fmt.Errorf("failed to get folder %q: %v", config.Folder, err)
	}

	if _, err := p.get(ctx, config.Folder, spec, session.Finder); err == nil {
		return fmt.Errorf("a vm %s/%s already exists", config.Folder, spec.Name)
	}

	if config.ResourcePool != "" {
		if _, err := session.Finder.ResourcePool(ctx, config.ResourcePool); err != nil {
			return fmt.Errorf("failed to get resourcepool %q: %v", config.ResourcePool, err)
		}
	}

	templateVM, err := session.Finder.VirtualMachine(ctx, config.TemplateVMName)
	if err != nil {
		return fmt.Errorf("failed to get template vm %q: %v", config.TemplateVMName, err)
	}

	disks, err := getDisksFromVM(ctx, templateVM)
	if err != nil {
		return fmt.Errorf("failed to get disks from VM: %v", err)
	}
	if diskLen := len(disks); diskLen != 1 {
		return fmt.Errorf("expected vm to have exactly one disk, had %d", diskLen)
	}

	if config.DiskSizeGB != nil {
		if err := validateDiskResizing(disks, *config.DiskSizeGB); err != nil {
			return err
		}
	}
	if pc.OperatingSystem == providerconfigtypes.OperatingSystemSLES {
		return fmt.Errorf("invalid/not supported operating system specified %q: %v", pc.OperatingSystem, providerconfigtypes.ErrOSNotSupported)
	}
	return nil
}

func machineInvalidConfigurationTerminalError(err error) error {
	return cloudprovidererrors.TerminalError{
		Reason:  common.InvalidConfigurationMachineError,
		Message: err.Error(),
	}
}

func (p *provider) Create(machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	vm, err := p.create(machine, userdata)
	if err != nil {
		_, cleanupErr := p.Cleanup(machine, data)
		if cleanupErr != nil {
			return nil, fmt.Errorf("cleaning up failed with err %v after creation failed with err %v", cleanupErr, err)
		}
		return nil, err
	}
	return vm, nil
}

func (p *provider) create(machine *clusterv1alpha1.Machine, userdata string) (instance.Instance, error) {
	ctx := context.Background()

	config, pc, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	session, err := NewSession(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vCenter session: %v", err)
	}
	defer session.Logout()

	var containerLinuxUserdata string
	if pc.OperatingSystem == providerconfigtypes.OperatingSystemFlatcar {
		containerLinuxUserdata = userdata
	}

	virtualMachine, err := createClonedVM(ctx,
		machine.Spec.Name,
		config,
		session,
		pc.OperatingSystem,
		containerLinuxUserdata,
	)
	if err != nil {
		return nil, machineInvalidConfigurationTerminalError(fmt.Errorf("failed to create cloned vm: '%v'", err))
	}

	if pc.OperatingSystem != providerconfigtypes.OperatingSystemFlatcar {
		localUserdataIsoFilePath, err := generateLocalUserdataISO(userdata, machine.Spec.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to generate local userdadata iso: %v", err)
		}

		defer func() {
			err := os.Remove(localUserdataIsoFilePath)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("failed to clean up local userdata iso file at %s: %v", localUserdataIsoFilePath, err))
			}
		}()

		if err := uploadAndAttachISO(ctx, session, virtualMachine, localUserdataIsoFilePath); err != nil {
			// Destroy VM to avoid a leftover.
			destroyTask, vmErr := virtualMachine.Destroy(ctx)
			if vmErr != nil {
				return nil, fmt.Errorf("failed to destroy vm %s after failing upload and attach userdata iso: %v / %v", virtualMachine.Name(), err, vmErr)
			}
			if vmErr := destroyTask.Wait(ctx); vmErr != nil {
				return nil, fmt.Errorf("failed to destroy vm %s after failing upload and attach userdata iso: %v / %v", virtualMachine.Name(), err, vmErr)
			}
			return nil, machineInvalidConfigurationTerminalError(fmt.Errorf("failed to upload and attach userdata iso: %v", err))
		}
	}

	powerOnTask, err := virtualMachine.PowerOn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to power on machine: %v", err)
	}

	if err := powerOnTask.Wait(ctx); err != nil {
		return nil, fmt.Errorf("error when waiting for vm powerOn task: %v", err)
	}

	return Server{name: virtualMachine.Name(), status: instance.StatusRunning, id: virtualMachine.Reference().Value}, nil
}

func (p *provider) Cleanup(machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, pc, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, fmt.Errorf("failed to parse config: %v", err)
	}

	session, err := NewSession(ctx, config)
	if err != nil {
		return false, fmt.Errorf("failed to create vCenter session: %v", err)
	}
	defer session.Logout()

	virtualMachine, err := p.get(ctx, config.Folder, machine.Spec, session.Finder)
	if err != nil {
		if cloudprovidererrors.IsNotFound(err) {
			return true, nil
		}
		return false, fmt.Errorf("failed to get instance from vSphere: %v", err)
	}

	powerState, err := virtualMachine.PowerState(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get virtual machine power state: %v", err)
	}

	// We cannot destroy a VM that's powered on, but we also
	// cannot power off a machine that is already off.
	if powerState != types.VirtualMachinePowerStatePoweredOff {
		powerOffTask, err := virtualMachine.PowerOff(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to poweroff vm %s: %v", virtualMachine.Name(), err)
		}
		if err = powerOffTask.Wait(ctx); err != nil {
			return false, fmt.Errorf("failed to poweroff vm %s: %v", virtualMachine.Name(), err)
		}
	}

	virtualMachineDeviceList, err := virtualMachine.Device(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get devices for virtual machine: %v", err)
	}

	pvs := &corev1.PersistentVolumeList{}
	if err := data.Client.List(data.Ctx, pvs); err != nil {
		return false, fmt.Errorf("failed to list PVs: %v", err)
	}

	for _, pv := range pvs.Items {
		if pv.Spec.VsphereVolume == nil {
			continue
		}
		for _, device := range virtualMachineDeviceList {
			if virtualMachineDeviceList.Type(device) == object.DeviceTypeDisk {
				fileName := device.GetVirtualDevice().Backing.(types.BaseVirtualDeviceFileBackingInfo).GetVirtualDeviceFileBackingInfo().FileName
				if pv.Spec.VsphereVolume.VolumePath == fileName {
					if err := virtualMachine.RemoveDevice(ctx, true, device); err != nil {
						return false, fmt.Errorf("error detaching pv-backing disk %s: %v", fileName, err)
					}
				}
			}
		}
	}

	datastore, err := getDatastoreFromVM(ctx, session, virtualMachine)
	if err != nil {
		return false, fmt.Errorf("Error getting datastore from VM %s: %v", virtualMachine.Name(), err)
	}
	destroyTask, err := virtualMachine.Destroy(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to destroy vm %s: %v", virtualMachine.Name(), err)
	}
	if err := destroyTask.Wait(ctx); err != nil {
		return false, fmt.Errorf("failed to destroy vm %s: %v", virtualMachine.Name(), err)
	}

	if pc.OperatingSystem != providerconfigtypes.OperatingSystemFlatcar {
		filemanager := datastore.NewFileManager(session.Datacenter, false)

		if err := filemanager.Delete(ctx, virtualMachine.Name()); err != nil {
			if err.Error() == fmt.Sprintf("File [%s] %s was not found", datastore.Name(), virtualMachine.Name()) {
				return true, nil
			}
			return false, fmt.Errorf("failed to delete storage of deleted instance %s: %v", virtualMachine.Name(), err)
		}
	}

	klog.V(2).Infof("Successfully destroyed vm %s", virtualMachine.Name())
	return true, nil
}

func (p *provider) Get(machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	ctx := context.Background()

	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	session, err := NewSession(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vCenter session: %v", err)
	}
	defer session.Logout()

	virtualMachine, err := p.get(ctx, config.Folder, machine.Spec, session.Finder)
	if err != nil {
		// Must not wrap because we match on the error type
		return nil, err
	}

	powerState, err := virtualMachine.PowerState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get powerstate: %v", err)
	}

	if powerState != types.VirtualMachinePowerStatePoweredOn {
		powerOnTask, err := virtualMachine.PowerOn(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to power on instance that was in state %q: %v", powerState, err)
		}
		if err := powerOnTask.Wait(ctx); err != nil {
			return nil, fmt.Errorf("failed waiting for instance to be powered on: %v", err)
		}
		// We must return here because the vendored code for determining if the guest
		// utils are running yields an NPD when using with an instance that is not running
		return Server{name: virtualMachine.Name(), status: instance.StatusUnknown}, nil
	}

	// virtualMachine.IsToolsRunning panics when executed on a VM that is not powered on
	addresses := map[string]corev1.NodeAddressType{}
	isGuestToolsRunning, err := virtualMachine.IsToolsRunning(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check if guest utils are running: %v", err)
	}
	if isGuestToolsRunning {
		var moVirtualMachine mo.VirtualMachine
		pc := property.DefaultCollector(session.Client.Client)
		if err := pc.RetrieveOne(ctx, virtualMachine.Reference(), []string{"guest"}, &moVirtualMachine); err != nil {
			return nil, fmt.Errorf("failed to retrieve guest info: %v", err)
		}

		for _, nic := range moVirtualMachine.Guest.Net {
			for _, address := range nic.IpAddress {
				// Exclude ipv6 link-local addresses and default Docker bridge
				if !strings.HasPrefix(address, "fe80:") && !strings.HasPrefix(address, "172.17.") {
					addresses[address] = ""
				}
			}
		}
	} else {
		klog.V(3).Infof("Can't fetch the IP addresses for machine %s, the VMware guest utils are not running yet. This might take a few minutes", machine.Spec.Name)
	}

	return Server{name: virtualMachine.Name(), status: instance.StatusRunning, addresses: addresses, id: virtualMachine.Reference().Value}, nil
}

func (p *provider) MigrateUID(machine *clusterv1alpha1.Machine, new ktypes.UID) error {
	return nil
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	c, _, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse config: %v", err)
	}

	passedURL := c.VSphereURL
	// Required because url.Parse returns an empty string for the hostname if there was no schema
	if !strings.HasPrefix(passedURL, "https://") {
		passedURL = "https://" + passedURL
	}

	u, err := url.Parse(passedURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse '%s' as url: %v", passedURL, err)
	}

	workingDir := c.Folder
	// Default to basedir
	if workingDir == "" {
		workingDir = fmt.Sprintf("/%s/vm", c.Datacenter)
	}

	cc := &vspheretypes.CloudConfig{
		Global: vspheretypes.GlobalOpts{
			User:         c.Username,
			Password:     c.Password,
			InsecureFlag: c.AllowInsecure,
			VCenterPort:  u.Port(),
		},
		Disk: vspheretypes.DiskOpts{
			SCSIControllerType: "pvscsi",
		},
		Workspace: vspheretypes.WorkspaceOpts{
			Datacenter:       c.Datacenter,
			VCenterIP:        u.Hostname(),
			DefaultDatastore: c.Datastore,
			Folder:           workingDir,
		},
		VirtualCenter: map[string]*vspheretypes.VirtualCenterConfig{
			u.Hostname(): {
				VCenterPort: u.Port(),
				Datacenters: c.Datacenter,
				User:        c.Username,
				Password:    c.Password,
			},
		},
	}

	s, err := vspheretypes.CloudConfigToString(cc)
	if err != nil {
		return "", "", fmt.Errorf("failed to convert the cloud-config to string: %v", err)
	}

	return s, "vsphere", nil
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["size"] = fmt.Sprintf("%d-cpus-%d-mb", c.CPUs, c.MemoryMB)
		labels["dc"] = c.Datacenter
	}

	return labels, err
}

func (p *provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	return nil
}

func (p *provider) get(ctx context.Context, folder string, spec clusterv1alpha1.MachineSpec, datacenterFinder *find.Finder) (*object.VirtualMachine, error) {
	path := fmt.Sprintf("%s/%s", folder, spec.Name)
	virtualMachineList, err := datacenterFinder.VirtualMachineList(ctx, path)
	if err != nil {
		if err.Error() == fmt.Sprintf("vm '%s' not found", path) {
			return nil, cloudprovidererrors.ErrInstanceNotFound
		}
		return nil, fmt.Errorf("failed to list virtual machines: %v", err)
	}

	if len(virtualMachineList) == 0 {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}
	if n := len(virtualMachineList); n > 1 {
		return nil, fmt.Errorf("expected to find at most one vm at %q, got %d", path, n)
	}
	return virtualMachineList[0], nil
}
