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
	"encoding/json"
	"errors"
	"fmt"

	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
)

const (
	machineUIDTag = "kubermatic-machine-controller:machine-uid"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

type RawConfig struct {
	AccessKeyID     providerconfig.ConfigVarString `json:"accessKeyID"`
	AccessKeySecret providerconfig.ConfigVarString `json:"accessKeySecret"`
	RegionID        providerconfig.ConfigVarString `json:"regionID"`
	ImageID         providerconfig.ConfigVarString `json:"imageID"`
	InstanceName    providerconfig.ConfigVarString `json:"instanceName,omitempty"`
	InstanceType    providerconfig.ConfigVarString `json:"instanceType"`
}

type Config struct {
	AccessKeyID     string
	AccessKeySecret string
	RegionID        string
	ImageID         string
	InstanceName    string
	InstanceType    string
	InstanceID      string
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
	return a.instance.PublicIpAddress.IpAddress
}

func (a *alibabaInstance) Status() instance.Status {
	return instance.Status(a.instance.Status)
}

// New returns a Kubevirt provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

func (p *provider) AddDefaults(spec v1alpha1.MachineSpec) (v1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) Validate(machinespec v1alpha1.MachineSpec) error {
	c, _, err := p.getConfig(machinespec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	if c.AccessKeyID == "" {
		return errors.New("accessKeyID is missing")
	}
	if c.AccessKeySecret == "" {
		return errors.New("accessKeySecret is missing")
	}
	if c.RegionID == "" {
		return errors.New("regionID is missing")
	}
	if c.ImageID == "" {
		return errors.New("imageID is missing")
	}
	if c.InstanceType == "" {
		return errors.New("instanceType is missing")
	}

	return nil
}

func (p *provider) Get(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
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

	instance, err := getInstance(client, c.InstanceID)
	if err != nil {
		return nil, err
	}
	if instance != nil {
		return &alibabaInstance{instance: instance}, nil
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) GetCloudConfig(spec v1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) Create(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
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

	createInstanceRequest := &ecs.CreateInstanceRequest{
		ImageId:      c.ImageID,
		InstanceName: c.InstanceName,
		InstanceType: c.InstanceType,
	}

	response, err := client.CreateInstance(createInstanceRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance at Alibaba cloud: %v", err)
	}
	c.InstanceID = response.InstanceId

	instance, err := getInstance(client, c.InstanceID)
	if err != nil {
		return nil, err
	}
	return &alibabaInstance{instance: instance}, nil
}

func (p *provider) Cleanup(machine *v1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	if _, err := p.Get(machine, data); err != nil {
		if err == cloudprovidererrors.ErrInstanceNotFound {
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

	client, err := getClient(c.RegionID, c.AccessKeyID, c.AccessKeySecret)
	if err != nil {
		return false, err
	}

	request := ecs.CreateDeleteInstanceRequest()
	request.InstanceId = c.InstanceID

	if _, err = client.DeleteInstance(request); err != nil {
		return false, fmt.Errorf("failed to delete instance with instanceID %s, due to %v", c.InstanceID, err)
	}

	return false, nil
}

func (p *provider) MachineMetricsLabels(machine *v1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["instanceType"] = c.InstanceType
		labels["region"] = c.RegionID
	}

	return labels, err
}

func (p *provider) MigrateUID(machine *v1alpha1.Machine, new types.UID) error {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
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

func (p *provider) getConfig(s v1alpha1.ProviderSpec) (*Config, *RawConfig, error) {
	if s.Value == nil {
		return nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}
	pconfig := providerconfig.Config{}
	err := json.Unmarshal(s.Value.Raw, &pconfig)
	if err != nil {
		return nil, nil, err
	}

	rawConfig := RawConfig{}
	if err = json.Unmarshal(pconfig.CloudProviderSpec.Raw, &rawConfig); err != nil {
		return nil, nil, err
	}

	c := Config{}
	c.AccessKeyID, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.AccessKeyID, "ALIBABA_ACCESS_KEY_ID")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"AccessKeyID\" field, error = %v", err)
	}
	c.AccessKeySecret, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.AccessKeySecret, "ALIBABA_ACCESS_KEY_SECRET")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"AccessKeySecret\" field, error = %v", err)
	}
	c.InstanceType, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.InstanceType)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"instanceType\" field, error = %v", err)
	}
	c.InstanceName, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.InstanceName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"instanceName\" field, error = %v", err)
	}
	c.ImageID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ImageID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"imageID\" field, error = %v", err)
	}
	c.RegionID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.RegionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"regionID\" field, error = %v", err)
	}

	return &c, &rawConfig, err
}

func getClient(accessKeyID, accessKeySecret, regionID string) (*ecs.Client, error) {
	client, err := ecs.NewClientWithAccessKey(regionID, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get Alibaba cloud client: %v", err)
	}
	return client, nil
}

func getInstance(client *ecs.Client, instanceID string) (*ecs.Instance, error) {
	describeInstanceRequest := ecs.CreateDescribeInstancesRequest()
	describeInstanceRequest.InstanceIds = instanceID

	response, err := client.DescribeInstances(describeInstanceRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance with instanceID: %s: %v", instanceID, err)
	}

	if response.Instances.Instance == nil {
		return nil, fmt.Errorf("failed to get instance with instancedID: %s: %v", instanceID, err)
	}
	return &response.Instances.Instance[0], nil
}
