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

package alibaba

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"k8s.io/apimachinery/pkg/types"
)

const (
	machineUIDTag = "machine_uid"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

type RawConfig struct {
	AccessKeyID             providerconfig.ConfigVarString `json:"accessKeyID,omitempty"`
	AccessKeySecret         providerconfig.ConfigVarString `json:"accessKeySecret,omitempty"`
	RegionID                providerconfig.ConfigVarString `json:"regionID,omitempty"`
	InstanceName            providerconfig.ConfigVarString `json:"instanceName,omitempty"`
	InstanceType            providerconfig.ConfigVarString `json:"instanceType,omitempty"`
	VSwitchID               providerconfig.ConfigVarString `json:"vSwitchID,omitempty"`
	InternetMaxBandwidthOut providerconfig.ConfigVarString `json:"internetMaxBandwidthOut,omitempty"`
	Labels                  map[string]string              `json:"labels,omitempty"`
}

type Config struct {
	AccessKeyID             string
	AccessKeySecret         string
	RegionID                string
	InstanceType            string
	InstanceID              string
	VSwitchID               string
	InternetMaxBandwidthOut string
	Labels                  map[string]string
}

type alibabaInstance struct {
	instance *ecs.Instance
}

func (a *alibabaInstance) Name() string {
	return a.instance.InstanceName
}

func (a *alibabaInstance) ID() string {
	return a.instance.InstanceId
}

func (a *alibabaInstance) Addresses() []string {
	var primaryIpAddresses []string
	for _, networkInterface := range a.instance.NetworkInterfaces.NetworkInterface {
		primaryIpAddresses = append(primaryIpAddresses, networkInterface.PrimaryIpAddress)
	}

	return primaryIpAddresses
}

func (a *alibabaInstance) Status() instance.Status {
	return instance.Status(a.instance.Status)
}

// New returns an Alibaba cloud provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) Validate(machineSpec v1alpha1.MachineSpec) error {
	c, _, pc, err := p.getConfig(machineSpec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	if c.AccessKeyID == "" {
		return fmt.Errorf("accessKeyID is missing")
	}
	if c.AccessKeySecret == "" {
		return fmt.Errorf("accessKeySecret is missing")
	}
	if c.RegionID == "" {
		return fmt.Errorf("regionID is missing")
	}
	if c.InstanceType == "" {
		return fmt.Errorf("instanceType is missing")
	}
	if c.VSwitchID == "" {
		return fmt.Errorf("vSwitchID is missing")
	}
	if c.InternetMaxBandwidthOut == "" {
		return fmt.Errorf("internetMaxBandwidthOut is missing")
	}
	_, err = getImageIDForOS(pc.OperatingSystem)
	if err != nil {
		return fmt.Errorf("invalid/not supported operating system specified %q: %v", pc.OperatingSystem, err)
	}

	return nil
}

func (p *provider) Get(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := getClient(c.RegionID, c.AccessKeyID, c.AccessKeySecret)
	if err != nil {
		return nil, err
	}

	i, err := getInstance(client, machine.Name)
	if err != nil {
		return nil, err
	}

	if i != nil {
		return &alibabaInstance{instance: i}, nil
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) Create(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, _, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := getClient(c.RegionID, c.AccessKeyID, c.AccessKeySecret)
	if err != nil {
		return nil, err
	}

	if c.Labels == nil {
		c.Labels = map[string]string{}
	}

	c.Labels[machineUIDTag] = string(machine.UID)

	var instanceTags []ecs.CreateInstanceTag
	for k, v := range c.Labels {
		instanceTags = append(instanceTags, ecs.CreateInstanceTag{
			Key:   k,
			Value: v,
		})
	}

	createInstanceRequest := ecs.CreateCreateInstanceRequest()
	createInstanceRequest.ImageId, _ = getImageIDForOS(pc.OperatingSystem)
	createInstanceRequest.InstanceName = machine.Name
	createInstanceRequest.InstanceType = c.InstanceType
	createInstanceRequest.VSwitchId = c.VSwitchID
	createInstanceRequest.InternetMaxBandwidthOut = requests.Integer(c.InternetMaxBandwidthOut)
	encodedUserData := base64.StdEncoding.EncodeToString([]byte(userdata))
	createInstanceRequest.UserData = encodedUserData
	createInstanceRequest.SystemDiskCategory = "cloud_efficiency"
	createInstanceRequest.Tag = &instanceTags

	_, err = client.CreateInstance(createInstanceRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance at Alibaba cloud: %v", err)
	}

	foundInstance, err := getInstance(client, machine.Name)
	if err != nil {
		return nil, err
	}

	ipAddress := ecs.CreateAllocatePublicIpAddressRequest()
	ipAddress.InstanceId = foundInstance.InstanceId

	_, err = client.AllocatePublicIpAddress(ipAddress)
	if err != nil {
		return nil, err
	}

	startRequest := ecs.CreateStartInstanceRequest()
	startRequest.InstanceId = foundInstance.InstanceId

	_, err = client.StartInstance(startRequest)
	if err != nil {
		return nil, err
	}

	return &alibabaInstance{instance: foundInstance}, nil
}

func (p *provider) Cleanup(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	if _, err := p.Get(machine, data); err != nil {
		if err == cloudprovidererrors.ErrInstanceNotFound {
			return true, nil
		}
		return false, err
	}

	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := getClient(c.RegionID, c.AccessKeyID, c.AccessKeySecret)
	if err != nil {
		return false, err
	}

	foundInstance, err := getInstance(client, machine.Name)
	if err != nil {
		return false, err
	}

	stopInstanceRequest := ecs.CreateStopInstanceRequest()
	stopInstanceRequest.InstanceId = foundInstance.InstanceId
	client.StopInstance(stopInstanceRequest)

	deleteInstancesRequest := ecs.CreateDeleteInstancesRequest()
	deleteInstancesRequest.InstanceId = &[]string{foundInstance.InstanceId}
	if _, err = client.DeleteInstances(deleteInstancesRequest); err != nil {
		return false, fmt.Errorf("failed to delete instance with instanceID %s, due to %v", c.InstanceID, err)
	}

	return false, nil
}

func (p *provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["instanceType"] = c.InstanceType
		labels["region"] = c.RegionID
	}

	return labels, err
}

func (p *provider) MigrateUID(machine *v1alpha1.Machine, new types.UID) error {
	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to decode providerconfig: %v", err)
	}

	client, err := getClient(c.RegionID, c.AccessKeyID, c.AccessKeySecret)
	if err != nil {
		return err
	}

	tag := ecs.AddTagsTag{
		Value: string(new),
		Key:   machineUIDTag,
	}
	request := ecs.CreateAddTagsRequest()
	request.ResourceId = c.InstanceID
	tags := []ecs.AddTagsTag{tag}
	request.Tag = &tags

	if _, err := client.AddTags(request); err != nil {
		return fmt.Errorf("failed to create new UID tag: %v", err)
	}

	return nil
}

func (p *provider) SetMetricsForMachines(machines v1alpha1.MachineList) error {
	return nil
}

func (p *provider) getConfig(s v1alpha1.ProviderSpec) (*Config, *RawConfig, *providerconfig.Config, error) {
	if s.Value == nil {
		return nil, nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	pconfig := providerconfig.Config{}
	err := json.Unmarshal(s.Value.Raw, &pconfig)
	if err != nil {
		return nil, nil, nil, err
	}

	rawConfig := RawConfig{}
	if err = json.Unmarshal(pconfig.CloudProviderSpec.Raw, &rawConfig); err != nil {
		return nil, nil, nil, err
	}

	c := Config{}
	c.AccessKeyID, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.AccessKeyID, "ALIBABA_ACCESS_KEY_ID")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"AccessKeyID\" field, error = %v", err)
	}
	c.AccessKeySecret, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.AccessKeySecret, "ALIBABA_ACCESS_KEY_SECRET")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"AccessKeySecret\" field, error = %v", err)
	}
	c.InstanceType, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.InstanceType)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"instanceType\" field, error = %v", err)
	}
	c.RegionID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.RegionID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"regionID\" field, error = %v", err)
	}
	c.VSwitchID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VSwitchID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"vSwitchID\" field, error = %v", err)
	}
	c.InternetMaxBandwidthOut, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.InternetMaxBandwidthOut)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"internetMaxBandwidthOut\" field, error = %v", err)
	}
	c.Labels = rawConfig.Labels
	return &c, &rawConfig, &pconfig, err
}

func getClient(regionID, accessKeyID, accessKeySecret string) (*ecs.Client, error) {
	client, err := ecs.NewClientWithAccessKey(regionID, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get Alibaba cloud client: %v", err)
	}
	return client, nil
}

func getInstance(client *ecs.Client, instanceName string) (*ecs.Instance, error) {
	describeInstanceRequest := ecs.CreateDescribeInstancesRequest()
	describeInstanceRequest.InstanceName = instanceName

	response, err := client.DescribeInstances(describeInstanceRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance with instanceName: %s: %v", instanceName, err)
	}

	if response.Instances.Instance == nil || len(response.Instances.Instance) == 0 {
		return nil, errors.ErrInstanceNotFound
	}

	return &response.Instances.Instance[0], nil
}

func getImageIDForOS(os providerconfig.OperatingSystem) (string, error) {
	switch os {
	case providerconfig.OperatingSystemUbuntu:
		return "ubuntu_18_04_64_20G_alibase_20190624.vhd", nil
	case providerconfig.OperatingSystemCentOS:
		return "centos_7_06_64_20G_alibase_20190711.vhd", nil
	case providerconfig.OperatingSystemCoreos:
		return "coreos_2023_4_0_64_30G_alibase_20190319.vhd", nil
	}
	return "", providerconfig.ErrOSNotSupported
}
