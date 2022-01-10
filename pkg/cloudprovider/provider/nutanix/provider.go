/*
Copyright 2021 The Machine Controller Authors.

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

package nutanix

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	nutanixtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/nutanix/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
)

type Config struct {
	Endpoint      string
	Port          *int
	Username      string
	Password      string
	AllowInsecure bool
	ProxyURL      string

	ClusterName string
	ProjectName string
	SubnetName  string
	ImageName   string

	Categories map[string]string

	CPUs           int64
	CPUCores       *int64
	CPUPassthrough *bool
	MemoryMB       int64
	DiskSizeGB     *int64
}

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// Server holds Nutanix server information.
type Server struct {
	name      string
	id        string
	status    instance.Status
	addresses map[string]corev1.NodeAddressType
}

// Ensures that Server implements Instance interface.
var _ instance.Instance = &Server{}

// Ensures that provider implements Provider interface.
var _ cloudprovidertypes.Provider = &provider{}

func (nutanixServer Server) Name() string {
	return nutanixServer.name
}

func (nutanixServer Server) ID() string {
	return nutanixServer.id
}

func (nutanixServer Server) Addresses() map[string]corev1.NodeAddressType {
	return nutanixServer.addresses
}

func (nutanixServer Server) Status() instance.Status {
	return nutanixServer.status
}

// New returns a nutanix provider.
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	provider := &provider{configVarResolver: configVarResolver}
	return provider
}

func (p *provider) getConfig(s v1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, *nutanixtypes.RawConfig, error) {
	if s.Value == nil {
		return nil, nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	pconfig := providerconfigtypes.Config{}
	err := json.Unmarshal(s.Value.Raw, &pconfig)
	if err != nil {
		return nil, nil, nil, err
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig := nutanixtypes.RawConfig{}
	err = json.Unmarshal(pconfig.CloudProviderSpec.Raw, &rawConfig)
	if err != nil {
		return nil, nil, nil, err
	}

	c := Config{}

	c.Endpoint, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Endpoint, "NUTANIX_ENDPOINT")
	if err != nil {
		return nil, nil, nil, err
	}

	port, err := p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Port, "NUTANIX_PORT")
	if err != nil {
		return nil, nil, nil, err
	}

	if port != "" {
		// we parse the port into an int to make sure we're being passed a somewhat valid port value
		portInt, err := strconv.Atoi(port)
		if err != nil {
			return nil, nil, nil, err
		}
		c.Port = pointer.Int(portInt)
	}

	c.Username, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Username, "NUTANIX_USERNAME")
	if err != nil {
		return nil, nil, nil, err
	}

	c.Password, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.Password, "NUTANIX_PASSWORD")
	if err != nil {
		return nil, nil, nil, err
	}

	c.AllowInsecure, err = p.configVarResolver.GetConfigVarBoolValueOrEnv(rawConfig.AllowInsecure, "NUTANIX_INSECURE")
	if err != nil {
		return nil, nil, nil, err
	}

	c.ProxyURL, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.ProxyURL, "NUTANIX_PROXY_URL")
	if err != nil {
		return nil, nil, nil, err
	}

	c.ClusterName, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.ClusterName, "NUTANIX_CLUSTER_NAME")
	if err != nil {
		return nil, nil, nil, err
	}

	if rawConfig.ProjectName != nil {
		c.ProjectName, err = p.configVarResolver.GetConfigVarStringValue(*rawConfig.ProjectName)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	c.SubnetName, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.SubnetName)
	if err != nil {
		return nil, nil, nil, err
	}

	c.ImageName, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ImageName)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Categories = rawConfig.Categories

	c.CPUs = rawConfig.CPUs
	c.CPUCores = rawConfig.CPUCores
	c.CPUPassthrough = rawConfig.CPUPassthrough
	c.MemoryMB = rawConfig.MemoryMB
	c.DiskSizeGB = rawConfig.DiskSize

	return &c, &pconfig, &rawConfig, nil
}

func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) Validate(spec v1alpha1.MachineSpec) error {
	config, _, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse machineSpec: %v", err)
	}

	client, err := GetClientSet(config)
	if err != nil {
		return fmt.Errorf("failed to construct client: %v", err)
	}

	cluster, err := getClusterByName(client, config.ClusterName)
	if err != nil {
		return fmt.Errorf("failed to get cluster: %v", err)
	}

	if config.ProjectName != "" {
		if _, err := getProjectByName(client, config.ProjectName); err != nil {
			return fmt.Errorf("failed to get project: %v", err)
		}
	}

	if _, err := getSubnetByName(client, config.SubnetName, *cluster.Metadata.UUID); err != nil {
		return fmt.Errorf("failed to get subnet: %v", err)
	}

	image, err := getImageByName(client, config.ImageName)
	if err != nil {
		return fmt.Errorf("failed to get image: %v", err)
	}

	var imageSizeBytes int64

	if image.Status != nil && image.Status.Resources.SizeBytes != nil {
		imageSizeBytes = *image.Status.Resources.SizeBytes
	} else {
		return fmt.Errorf("failed to read image size for '%s'", config.ImageName)
	}

	if config.DiskSizeGB != nil && *config.DiskSizeGB*1024*1024*1024 < imageSizeBytes {
		return fmt.Errorf("requested disk size (%d bytes) is smaller than image size (%d bytes)", *config.DiskSizeGB*1024*1024*1024, *image.Status.Resources.SizeBytes)
	}

	return nil
}

func (p *provider) Create(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
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

func (p *provider) create(machine *v1alpha1.Machine, userdata string) (instance.Instance, error) {
	config, pc, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to parse machineSpec: %v", err),
		}
	}

	client, err := GetClientSet(config)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to construct client: %v", err),
		}
	}

	return createVM(client, machine.Name, *config, pc.OperatingSystem, userdata)
}

func (p *provider) Cleanup(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	return p.cleanup(machine, data)
}

func (p *provider) cleanup(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to parse machineSpec: %v", err),
		}
	}

	client, err := GetClientSet(config)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to construct client: %v", err),
		}
	}

	var projectID *string

	if config.ProjectName != "" {
		project, err := getProjectByName(client, config.ProjectName)
		if err != nil {
			return false, err
		}

		projectID = project.Metadata.UUID
	}

	vm, err := getVMByName(client, machine.Name, projectID)
	if err != nil {
		if err == cloudprovidererrors.ErrInstanceNotFound {
			// VM is gone already
			return true, nil
		}

		return false, err
	}

	// TODO: figure out if VM is already in deleting state

	resp, err := client.Prism.V3.DeleteVM(*vm.Metadata.UUID)
	if err != nil {
		return false, err
	}

	taskID, ok := resp.Status.ExecutionContext.TaskUUID.(string)
	if !ok {
		return false, errors.New("failed to parse deletion task UUID")
	}

	if err := waitForCompletion(client, taskID, time.Second*5, time.Minute*10); err != nil {
		return false, fmt.Errorf("failed to wait for completion: %v", err)
	}

	return true, nil
}

func (p *provider) Get(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to parse machineSpec: %v", err),
		}
	}

	client, err := GetClientSet(config)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to construct client: %v", err),
		}
	}

	var projectID *string

	if config.ProjectName != "" {
		project, err := getProjectByName(client, config.ProjectName)
		if err != nil {
			return nil, err
		}

		projectID = project.Metadata.UUID
	}

	vm, err := getVMByName(client, machine.Name, projectID)
	if err != nil {
		return nil, err
	}

	if vm.Status == nil || vm.Status.Resources == nil || vm.Status.Resources.PowerState == nil {
		return nil, fmt.Errorf("could not read power state for VM '%s'", machine.Name)
	}

	var status instance.Status

	switch *vm.Status.Resources.PowerState {
	case "ON":
		status = instance.StatusRunning
	case "OFF":
		status = instance.StatusCreating
	default:
		status = instance.StatusUnknown
	}

	addresses := make(map[string]corev1.NodeAddressType)

	if len(vm.Status.Resources.NicList) > 0 && len(vm.Status.Resources.NicList[0].IPEndpointList) > 0 {
		ip := *vm.Status.Resources.NicList[0].IPEndpointList[0].IP
		addresses[ip] = corev1.NodeInternalIP
	} else {
		return nil, fmt.Errorf("could not find any IP addresses for VM '%s'", machine.Name)
	}

	return Server{
		name:      *vm.Status.Name,
		id:        *vm.Metadata.UUID,
		status:    status,
		addresses: addresses,
	}, nil
}

func (p *provider) MigrateUID(machine *v1alpha1.Machine, new ktypes.UID) error {
	return nil
}

// GetCloudConfig returns an empty cloud configuration for Nutanix as no CCM exists
func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return labels, fmt.Errorf("failed to parse config: %v", err)
	}

	labels["size"] = fmt.Sprintf("%d-cpus-%d-mb", config.CPUs, config.MemoryMB)
	labels["cluster"] = config.ClusterName

	return labels, nil
}

func (p *provider) SetMetricsForMachines(machines v1alpha1.MachineList) error {
	return nil
}
