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
	"os"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"go.uber.org/zap"

	cloudprovidererrors "k8c.io/machine-controller/pkg/cloudprovider/errors"
	"k8c.io/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "k8c.io/machine-controller/pkg/cloudprovider/types"
	"k8c.io/machine-controller/sdk/apis/cluster/common"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
	vspheretypes "k8c.io/machine-controller/sdk/cloudprovider/vsphere"
	"k8c.io/machine-controller/sdk/providerconfig"

	corev1 "k8s.io/api/core/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type provider struct {
	configVarResolver providerconfig.ConfigVarResolver
}

// New returns a VSphere provider.
func New(configVarResolver providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	provider := &provider{configVarResolver: configVarResolver}
	return provider
}

// Config contains vSphere provider configuration.
type Config struct {
	TemplateVMName   string
	VMNetName        string
	Networks         []string
	Username         string
	Password         string
	VSphereURL       string
	Datacenter       string
	Cluster          string
	Folder           string
	ResourcePool     string
	Datastore        string
	DatastoreCluster string
	AllowInsecure    bool
	VMAntiAffinity   bool
	CPUs             int32
	MemoryMB         int64
	DiskSizeGB       *int64
	Tags             []tags.Tag
	VMGroup          string
}

// Ensures that Server implements Instance interface.
var _ instance.Instance = &Server{}

// Server holds vSphere server information.
type Server struct {
	name      string
	id        string
	uuid      string
	status    instance.Status
	addresses map[string]corev1.NodeAddressType
}

func (vsphereServer Server) Name() string {
	return vsphereServer.name
}

func (vsphereServer Server) ID() string {
	return vsphereServer.id
}

func (vsphereServer Server) ProviderID() string {
	if vsphereServer.uuid == "" {
		return ""
	}
	return "vsphere://" + vsphereServer.uuid
}

func (vsphereServer Server) Addresses() map[string]corev1.NodeAddressType {
	return vsphereServer.addresses
}

func (vsphereServer Server) Status() instance.Status {
	return vsphereServer.status
}

// Ensures that provider implements Provider interface.
var _ cloudprovidertypes.Provider = &provider{}

func (p *provider) AddDefaults(_ *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *providerconfig.Config, *vspheretypes.RawConfig, error) {
	pconfig, err := providerconfig.GetConfig(provSpec)
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
	c.TemplateVMName, err = p.configVarResolver.GetStringValue(rawConfig.TemplateVMName)
	if err != nil {
		return nil, nil, nil, err
	}

	//nolint:staticcheck
	//lint:ignore SA1019: rawConfig.VMNetName is deprecated: use networks instead.
	c.VMNetName, err = p.configVarResolver.GetStringValue(rawConfig.VMNetName)
	if err != nil {
		return nil, nil, nil, err
	}

	for _, network := range rawConfig.Networks {
		networkValue, err := p.configVarResolver.GetStringValue(network)
		if err != nil {
			return nil, nil, rawConfig, err
		}
		c.Networks = append(c.Networks, networkValue)
	}

	c.Username, err = p.configVarResolver.GetStringValueOrEnv(rawConfig.Username, "VSPHERE_USERNAME")
	if err != nil {
		return nil, nil, nil, err
	}

	c.Password, err = p.configVarResolver.GetStringValueOrEnv(rawConfig.Password, "VSPHERE_PASSWORD")
	if err != nil {
		return nil, nil, nil, err
	}

	c.VSphereURL, err = p.configVarResolver.GetStringValueOrEnv(rawConfig.VSphereURL, "VSPHERE_ADDRESS")
	if err != nil {
		return nil, nil, nil, err
	}

	c.Datacenter, err = p.configVarResolver.GetStringValue(rawConfig.Datacenter)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Cluster, err = p.configVarResolver.GetStringValue(rawConfig.Cluster)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Folder, err = p.configVarResolver.GetStringValue(rawConfig.Folder)
	if err != nil {
		return nil, nil, nil, err
	}

	c.ResourcePool, err = p.configVarResolver.GetStringValue(rawConfig.ResourcePool)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Datastore, err = p.configVarResolver.GetStringValue(rawConfig.Datastore)
	if err != nil {
		return nil, nil, nil, err
	}

	c.DatastoreCluster, err = p.configVarResolver.GetStringValue(rawConfig.DatastoreCluster)
	if err != nil {
		return nil, nil, nil, err
	}

	c.AllowInsecure, err = p.configVarResolver.GetBoolValueOrEnv(rawConfig.AllowInsecure, "VSPHERE_ALLOW_INSECURE")
	if err != nil {
		return nil, nil, nil, err
	}

	c.VMAntiAffinity, _, err = p.configVarResolver.GetBoolValue(rawConfig.VMAntiAffinity)
	if err != nil {
		return nil, nil, nil, err
	}

	c.VMGroup, err = p.configVarResolver.GetStringValue(rawConfig.VMGroup)
	if err != nil {
		return nil, nil, nil, err
	}

	c.CPUs = rawConfig.CPUs
	c.MemoryMB = rawConfig.MemoryMB
	c.DiskSizeGB = rawConfig.DiskSizeGB

	for _, tag := range rawConfig.Tags {
		c.Tags = append(c.Tags, tags.Tag{
			Description: tag.Description,
			ID:          tag.ID,
			Name:        tag.Name,
			CategoryID:  tag.CategoryID,
		})
	}

	return &c, pconfig, rawConfig, nil
}

func (p *provider) Validate(ctx context.Context, log *zap.SugaredLogger, spec clusterv1alpha1.MachineSpec) error {
	config, _, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	session, err := NewSession(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create vCenter session: %w", err)
	}
	defer session.Logout(ctx)

	if len(config.Networks) > 0 && config.VMNetName != "" {
		return fmt.Errorf("both networks and vmNetName are specified, only one of them can be used")
	}

	if config.Tags != nil {
		restAPISession, err := NewRESTSession(ctx, config)
		if err != nil {
			return fmt.Errorf("failed to create REST API session: %w", err)
		}
		defer restAPISession.Logout(ctx)
		tagManager := tags.NewManager(restAPISession.Client)
		log.Debug("Found tags")
		for _, tag := range config.Tags {
			if tag.ID == "" && tag.Name == "" {
				return fmt.Errorf("either tag id or name must be specified")
			}
			if tag.CategoryID == "" {
				return fmt.Errorf("one of the tags category is empty")
			}
			if _, err := tagManager.GetCategory(ctx, tag.CategoryID); err != nil {
				return fmt.Errorf("can't get the category with ID %s, %w", tag.CategoryID, err)
			}
		}
		log.Debug("Tag validation passed")
	}

	// Only and only one between datastore and datastre cluster should be
	// present, otherwise an error is raised.
	if config.DatastoreCluster != "" && config.Datastore == "" {
		if _, err := session.Finder.DatastoreCluster(ctx, config.DatastoreCluster); err != nil {
			return fmt.Errorf("failed to get datastore cluster %s: %w", config.DatastoreCluster, err)
		}
	} else if config.Datastore != "" && config.DatastoreCluster == "" {
		if _, err := session.Finder.Datastore(ctx, config.Datastore); err != nil {
			return fmt.Errorf("failed to get datastore %s: %w", config.Datastore, err)
		}
	} else {
		return fmt.Errorf("one between datastore and datastore cluster should be specified: %w", err)
	}

	if _, err := session.Finder.Folder(ctx, config.Folder); err != nil {
		return fmt.Errorf("failed to get folder %q: %w", config.Folder, err)
	}

	if _, err := p.get(ctx, config.Folder, spec, session.Finder); err == nil {
		return fmt.Errorf("a vm %s/%s already exists", config.Folder, spec.Name)
	}

	if config.ResourcePool != "" {
		if _, err := session.Finder.ResourcePool(ctx, config.ResourcePool); err != nil {
			return fmt.Errorf("failed to get resourcepool %q: %w", config.ResourcePool, err)
		}
	}

	templateVM, err := session.Finder.VirtualMachine(ctx, config.TemplateVMName)
	if err != nil {
		return fmt.Errorf("failed to get template vm %q: %w", config.TemplateVMName, err)
	}

	disks, err := getDisksFromVM(ctx, templateVM)
	if err != nil {
		return fmt.Errorf("failed to get disks from VM: %w", err)
	}
	if diskLen := len(disks); diskLen != 1 {
		return fmt.Errorf("expected vm to have exactly one disk, had %d", diskLen)
	}

	if config.DiskSizeGB != nil {
		if err := validateDiskResizing(disks, *config.DiskSizeGB); err != nil {
			return err
		}
	}

	if config.VMAntiAffinity && config.Cluster == "" {
		return fmt.Errorf("cluster is required for vm anti affinity")
	} else if config.VMGroup != "" && config.Cluster == "" {
		return fmt.Errorf("cluster is required for vm group")
	}

	if config.Cluster != "" {
		_, err = session.Finder.ClusterComputeResource(ctx, config.Cluster)
		if err != nil {
			return fmt.Errorf("failed to get cluster %q, %w", config.Cluster, err)
		}
	}

	return nil
}

func machineInvalidConfigurationTerminalError(err error) error {
	return cloudprovidererrors.TerminalError{
		Reason:  common.InvalidConfigurationMachineError,
		Message: err.Error(),
	}
}

func (p *provider) Create(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	vm, err := p.create(ctx, log, machine, userdata)
	if err != nil {
		_, cleanupErr := p.Cleanup(ctx, log, machine, data)
		if cleanupErr != nil {
			return nil, fmt.Errorf("cleaning up failed with err %w after creation failed with err %w", cleanupErr, err)
		}
		return nil, err
	}
	return vm, nil
}

func (p *provider) create(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, userdata string) (instance.Instance, error) {
	config, pc, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	session, err := NewSession(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vCenter session: %w", err)
	}
	defer session.Logout(ctx)

	var containerLinuxUserdata string
	if pc.OperatingSystem == providerconfig.OperatingSystemFlatcar {
		containerLinuxUserdata = userdata
	}

	virtualMachine, err := createClonedVM(
		ctx,
		log,
		machine.Spec.Name,
		config,
		session,
		containerLinuxUserdata,
	)
	if err != nil {
		return nil, machineInvalidConfigurationTerminalError(fmt.Errorf("failed to create cloned vm: '%w'", err))
	}

	if err := attachTags(ctx, log, config, virtualMachine); err != nil {
		return nil, fmt.Errorf("failed to attach tags: %w", err)
	}

	if config.VMGroup != "" {
		if err := p.addToVMGroup(ctx, log, session, machine, config); err != nil {
			return nil, fmt.Errorf("failed to add VM to VM group: %w", err)
		}
	}

	if config.VMAntiAffinity {
		if err := p.createOrUpdateVMAntiAffinityRule(ctx, log, session, machine, config); err != nil {
			return nil, fmt.Errorf("failed to add VM to anti affinity rule: %w", err)
		}
	}

	if pc.OperatingSystem != providerconfig.OperatingSystemFlatcar {
		localUserdataIsoFilePath, err := generateLocalUserdataISO(userdata, machine.Spec.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to generate local userdadata iso: %w", err)
		}

		defer func() {
			err := os.Remove(localUserdataIsoFilePath)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("failed to clean up local userdata iso file at %s: %w", localUserdataIsoFilePath, err))
			}
		}()

		if err := uploadAndAttachISO(ctx, log, session, virtualMachine, localUserdataIsoFilePath); err != nil {
			// Destroy VM to avoid a leftover.
			destroyTask, vmErr := virtualMachine.Destroy(ctx)
			if vmErr != nil {
				return nil, fmt.Errorf("failed to destroy vm %s after failing upload and attach userdata iso: %w / %w", virtualMachine.Name(), err, vmErr)
			}
			if vmErr := destroyTask.WaitEx(ctx); vmErr != nil {
				return nil, fmt.Errorf("failed to destroy vm %s after failing upload and attach userdata iso: %w / %w", virtualMachine.Name(), err, vmErr)
			}
			return nil, machineInvalidConfigurationTerminalError(fmt.Errorf("failed to upload and attach userdata iso: %w", err))
		}
	}

	powerOnTask, err := virtualMachine.PowerOn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to power on machine: %w", err)
	}

	if err := powerOnTask.WaitEx(ctx); err != nil {
		return nil, fmt.Errorf("error when waiting for vm powerOn task: %w", err)
	}

	return Server{name: virtualMachine.Name(), status: instance.StatusRunning, id: virtualMachine.Reference().Value, uuid: virtualMachine.UUID(ctx)}, nil
}

func (p *provider) Cleanup(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	config, pc, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, fmt.Errorf("failed to parse config: %w", err)
	}

	session, err := NewSession(ctx, config)
	if err != nil {
		return false, fmt.Errorf("failed to create vCenter session: %w", err)
	}
	defer session.Logout(ctx)

	virtualMachine, err := p.get(ctx, config.Folder, machine.Spec, session.Finder)
	if err != nil {
		if cloudprovidererrors.IsNotFound(err) {
			return true, nil
		}
		return false, fmt.Errorf("failed to get instance from vSphere: %w", err)
	}

	if err := detachTags(ctx, log, config, virtualMachine); err != nil {
		return false, fmt.Errorf("failed to delete tags: %w", err)
	}

	if config.VMAntiAffinity {
		if err := p.createOrUpdateVMAntiAffinityRule(ctx, log, session, machine, config); err != nil {
			return false, fmt.Errorf("failed to update VMs in anti-affinity rule: %w", err)
		}
	}

	powerState, err := virtualMachine.PowerState(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get virtual machine power state: %w", err)
	}

	// We cannot destroy a VM that's powered on, but we also
	// cannot power off a machine that is already off.
	if powerState != types.VirtualMachinePowerStatePoweredOff {
		powerOffTask, err := virtualMachine.PowerOff(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to poweroff vm %s: %w", virtualMachine.Name(), err)
		}
		if err = powerOffTask.WaitEx(ctx); err != nil {
			return false, fmt.Errorf("failed to poweroff vm %s: %w", virtualMachine.Name(), err)
		}
	}

	virtualMachineDeviceList, err := virtualMachine.Device(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get devices for virtual machine: %w", err)
	}

	pvs := &corev1.PersistentVolumeList{}
	if err := data.Client.List(data.Ctx, pvs); err != nil {
		return false, fmt.Errorf("failed to list PVs: %w", err)
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
						return false, fmt.Errorf("error detaching pv-backing disk %s: %w", fileName, err)
					}
				}
			}
		}
	}

	datastore, err := getDatastoreFromVM(ctx, session, virtualMachine)
	if err != nil {
		return false, fmt.Errorf("failed to get datastore from VM %s: %w", virtualMachine.Name(), err)
	}
	destroyTask, err := virtualMachine.Destroy(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to destroy VM %s: %w", virtualMachine.Name(), err)
	}
	if err := destroyTask.WaitEx(ctx); err != nil {
		return false, fmt.Errorf("failed to destroy VM %s: %w", virtualMachine.Name(), err)
	}

	if pc.OperatingSystem != providerconfig.OperatingSystemFlatcar {
		filemanager := datastore.NewFileManager(session.Datacenter, false)

		if err := filemanager.Delete(ctx, virtualMachine.Name()); err != nil {
			if err.Error() == fmt.Sprintf("File [%s] %s was not found", datastore.Name(), virtualMachine.Name()) {
				return true, nil
			}
			return false, fmt.Errorf("failed to delete storage of deleted instance %s: %w", virtualMachine.Name(), err)
		}
	}

	log.Infow("Successfully destroyed VM", "vm", virtualMachine.Name())
	return true, nil
}

func (p *provider) Get(ctx context.Context, log *zap.SugaredLogger, machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	session, err := NewSession(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vCenter session: %w", err)
	}
	defer session.Logout(ctx)

	virtualMachine, err := p.get(ctx, config.Folder, machine.Spec, session.Finder)
	if err != nil {
		// Must not wrap because we match on the error type
		return nil, err
	}

	powerState, err := virtualMachine.PowerState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get powerstate: %w", err)
	}

	if powerState != types.VirtualMachinePowerStatePoweredOn {
		powerOnTask, err := virtualMachine.PowerOn(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to power on instance that was in state %q: %w", powerState, err)
		}
		if err := powerOnTask.WaitEx(ctx); err != nil {
			return nil, fmt.Errorf("failed waiting for instance to be powered on: %w", err)
		}
		// We must return here because the vendored code for determining if the guest
		// utils are running yields an NPD when using with an instance that is not running
		return Server{name: virtualMachine.Name(), status: instance.StatusUnknown, uuid: virtualMachine.UUID(ctx)}, nil
	}

	// virtualMachine.IsToolsRunning panics when executed on a VM that is not powered on
	addresses := map[string]corev1.NodeAddressType{}
	isGuestToolsRunning, err := virtualMachine.IsToolsRunning(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check if guest utils are running: %w", err)
	}
	if isGuestToolsRunning {
		var moVirtualMachine mo.VirtualMachine
		pc := property.DefaultCollector(session.Client.Client)
		if err := pc.RetrieveOne(ctx, virtualMachine.Reference(), []string{"guest"}, &moVirtualMachine); err != nil {
			return nil, fmt.Errorf("failed to retrieve guest info: %w", err)
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
		log.Debug("Can't fetch the IP addresses for machine, the VMware guest utils are not running yet. This might take a few minutes")
	}

	return Server{name: virtualMachine.Name(), status: instance.StatusRunning, addresses: addresses, id: virtualMachine.Reference().Value, uuid: virtualMachine.UUID(ctx)}, nil
}

func (p *provider) MigrateUID(_ context.Context, _ *zap.SugaredLogger, _ *clusterv1alpha1.Machine, _ ktypes.UID) error {
	return nil
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

func (p *provider) SetMetricsForMachines(_ clusterv1alpha1.MachineList) error {
	return nil
}

func (p *provider) get(ctx context.Context, folder string, spec clusterv1alpha1.MachineSpec, datacenterFinder *find.Finder) (*object.VirtualMachine, error) {
	path := fmt.Sprintf("%s/%s", folder, spec.Name)
	virtualMachineList, err := datacenterFinder.VirtualMachineList(ctx, path)
	if err != nil {
		if err.Error() == fmt.Sprintf("vm '%s' not found", path) {
			return nil, cloudprovidererrors.ErrInstanceNotFound
		}
		return nil, fmt.Errorf("failed to list virtual machines: %w", err)
	}

	if len(virtualMachineList) == 0 {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}
	if n := len(virtualMachineList); n > 1 {
		return nil, fmt.Errorf("expected to find at most one vm at %q, got %d", path, n)
	}
	return virtualMachineList[0], nil
}
