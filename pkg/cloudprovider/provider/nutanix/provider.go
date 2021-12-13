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
	"strings"
	"time"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	nutanixtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/nutanix/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
)

type Config struct {
	Endpoint      string
	Username      string
	Password      string
	AllowInsecure bool

	ClusterName string
	ProjectName string
	SubnetName  string
	ImageName   string

	Categories map[string]string

	CPUs       int64
	CPUCores   *int64
	MemoryMB   int64
	DiskSizeGB *int64
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

	c.Endpoint, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Endpoint)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Username, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Username)
	if err != nil {
		return nil, nil, nil, err
	}

	c.Password, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Password)
	if err != nil {
		return nil, nil, nil, err
	}

	c.AllowInsecure, err = p.configVarResolver.GetConfigVarBoolValue(rawConfig.AllowInsecure)
	if err != nil {
		return nil, nil, nil, err
	}

	c.ClusterName, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ClusterName)
	if err != nil {
		return nil, nil, nil, err
	}

	c.ProjectName, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ProjectName)
	if err != nil {
		return nil, nil, nil, err
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
		return fmt.Errorf("failed to get config: %v", err)
	}

	client, err := GetClientSet(config)
	if err != nil {
		return fmt.Errorf("failed to construct client: %v", err)
	}

	cluster, err := getClusterByName(client, config.ClusterName)
	if err != nil {
		return fmt.Errorf("failed to get cluster: %v", err)
	}

	_, err = getProjectByName(client, config.ProjectName)
	if err != nil {
		return fmt.Errorf("failed to get project: %v", err)
	}

	_, err = getSubnetByName(client, config.SubnetName, *cluster.Metadata.UUID)
	if err != nil {
		return fmt.Errorf("failed to get subnet: %v", err)
	}

	_, err = getImageByName(client, config.ImageName)
	if err != nil {
		return fmt.Errorf("failed to get image: %v", err)
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
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	client, err := GetClientSet(config)
	if err != nil {
		return nil, fmt.Errorf("failed to construct client: %v", err)
	}

	return createVM(client, machine.Spec.Name, *config, pc.OperatingSystem, userdata)
}

func (p *provider) Cleanup(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	return p.cleanup(machine, data)
}

func (p *provider) cleanup(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, fmt.Errorf("failed to parse config: %v", err)
	}

	client, err := GetClientSet(config)
	if err != nil {
		return false, fmt.Errorf("failed to construct client: %v", err)
	}

	project, err := getProjectByName(client, config.ProjectName)
	if err != nil {
		return false, fmt.Errorf("failed to get project: %v", err)
	}

	vm, err := getVMByName(client, machine.Name, *project.Metadata.UUID)
	if err != nil {
		if strings.Contains(err.Error(), entityNotFoundError) {
			// VM is gone already
			return true, nil
		}

		return false, fmt.Errorf("failed to get vm: %v", err)
	}

	resp, err := client.Prism.V3.DeleteVM(*vm.Metadata.UUID)
	taskID := resp.Status.ExecutionContext.TaskUUID.(string)

	if err := waitForCompletion(client, taskID, time.Second*5, time.Minute*10); err != nil {
		return false, fmt.Errorf("failed to wait for completion: %v", err)
	}

	return true, nil
}

func (p *provider) Get(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	client, err := GetClientSet(config)
	if err != nil {
		return nil, fmt.Errorf("failed to construct client: %v", err)
	}

	project, err := getProjectByName(client, config.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %v", err)
	}

	vm, err := getVMByName(client, machine.Name, *project.Metadata.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get vm: %v", err)
	}

	if vm.Status == nil || vm.Status.Resources == nil || vm.Status.Resources.PowerState == nil {
		return nil, errors.New("could not read vm power state")
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
		return nil, errors.New("could not find any IP addresses on VM")
	}

	return Server{
		name:      *vm.Metadata.Name,
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
