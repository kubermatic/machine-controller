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
	"errors"
	"fmt"
	"net/http"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	alibabatypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/alibaba/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"
	kuberneteshelper "github.com/kubermatic/machine-controller/pkg/kubernetes"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	machineUIDTag   = "machine_uid"
	centosImageName = "CentOS  7.9 64 bit"
	ubuntuImageName = "Ubuntu  20.04 64 bit"

	finalizerInstance = "kubermatic.io/cleanup-alibaba-instance"
)

type instanceStatus string

const (
	stoppedStatus instanceStatus = "Stopped"
	runningStatus instanceStatus = "Running"
)

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
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
	ZoneID                  string
	DiskType                string
	DiskSize                string
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

func (a *alibabaInstance) Addresses() map[string]v1.NodeAddressType {
	primaryIPAddresses := map[string]v1.NodeAddressType{}
	for _, networkInterface := range a.instance.NetworkInterfaces.NetworkInterface {
		primaryIPAddresses[networkInterface.PrimaryIpAddress] = v1.NodeInternalIP
	}

	return primaryIPAddresses
}

func (a *alibabaInstance) Status() instance.Status {
	return instance.Status(a.instance.Status)
}

// New returns an Alibaba cloud provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	return spec, nil
}

func (p *provider) Validate(machineSpec clusterv1alpha1.MachineSpec) error {
	c, pc, err := p.getConfig(machineSpec.ProviderSpec)
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
	if c.InstanceType == "" {
		return errors.New("instanceType is missing")
	}
	if c.VSwitchID == "" {
		return errors.New("vSwitchID is missing")
	}
	if c.InternetMaxBandwidthOut == "" {
		return errors.New("internetMaxBandwidthOut is missing")
	}
	if c.ZoneID == "" {
		return errors.New("zoneID is missing")
	}
	_, err = p.getImageIDForOS(machineSpec, pc.OperatingSystem)
	if err != nil {
		return fmt.Errorf("invalid/not supported operating system specified %q: %v", pc.OperatingSystem, err)
	}
	if c.DiskType == "" {
		return errors.New("DiskType is missing")
	}
	if c.DiskSize == "" {
		return errors.New("DiskSize is missing")
	}

	return nil
}

func (p *provider) Get(machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := getClient(c.RegionID, c.AccessKeyID, c.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get alibaba client: %v", err)
	}

	foundInstance, err := getInstance(client, machine.Name, string(machine.UID))
	if err != nil {
		return nil, err
	}

	switch instanceStatus(foundInstance.Status) {
	case stoppedStatus:
		startRequest := ecs.CreateStartInstanceRequest()
		startRequest.InstanceId = foundInstance.InstanceId

		_, err = client.StartInstance(startRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to start instance %v: %v", foundInstance.InstanceId, err)
		}
		return nil, fmt.Errorf("instance %v is in a stopped state", foundInstance.InstanceId)
	case runningStatus:
		if len(foundInstance.PublicIpAddress.IpAddress) == 0 {
			ipAddress := ecs.CreateAllocatePublicIpAddressRequest()
			ipAddress.InstanceId = foundInstance.InstanceId

			_, err = client.AllocatePublicIpAddress(ipAddress)
			if err != nil {
				return nil, fmt.Errorf("failed to allocate ip address for instance %v: %v", foundInstance.InstanceId, err)
			}

		}
		return &alibabaInstance{instance: foundInstance}, nil
	}

	return nil, fmt.Errorf("instance %v is not ready", foundInstance.InstanceId)
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	return "", "", nil
}

func (p *provider) Create(machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	c, pc, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("failed to parse MachineSpec, due to %v", err),
		}
	}

	client, err := getClient(c.RegionID, c.AccessKeyID, c.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get alibaba client: %v", err)
	}

	createInstanceRequest := ecs.CreateCreateInstanceRequest()
	createInstanceRequest.ImageId, err = p.getImageIDForOS(machine.Spec, pc.OperatingSystem)
	if err != nil {
		return nil, fmt.Errorf("failed to get a valid image for machine : %v", err)
	}
	createInstanceRequest.InstanceName = machine.Name
	createInstanceRequest.InstanceType = c.InstanceType
	createInstanceRequest.VSwitchId = c.VSwitchID
	createInstanceRequest.InternetMaxBandwidthOut = requests.Integer(c.InternetMaxBandwidthOut)
	encodedUserData := base64.StdEncoding.EncodeToString([]byte(userdata))
	createInstanceRequest.UserData = encodedUserData
	createInstanceRequest.SystemDiskCategory = c.DiskType
	createInstanceRequest.DataDisk = &[]ecs.CreateInstanceDataDisk{
		{
			Size: c.DiskSize,
		},
	}
	createInstanceRequest.SystemDiskSize = requests.Integer(c.DiskSize)
	createInstanceRequest.ZoneId = c.ZoneID
	tag := ecs.CreateInstanceTag{
		Key:   machineUIDTag,
		Value: string(machine.UID),
	}
	createInstanceRequest.Tag = &[]ecs.CreateInstanceTag{tag}

	_, err = client.CreateInstance(createInstanceRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance at Alibaba cloud: %v", err)
	}

	if err = data.Update(machine, func(updatedMachine *clusterv1alpha1.Machine) {
		if !kuberneteshelper.HasFinalizer(updatedMachine, finalizerInstance) {
			updatedMachine.Finalizers = append(updatedMachine.Finalizers, finalizerInstance)
		}
	}); err != nil {
		return nil, fmt.Errorf("failed updating machine %v finzaliers: %v", machine.Name, err)
	}

	foundInstance, err := getInstance(client, machine.Name, string(machine.UID))
	if err != nil {
		return nil, fmt.Errorf("failed to get alibaba instance %v due to %v", machine.Name, err)
	}

	return &alibabaInstance{instance: foundInstance}, nil
}

func (p *provider) Cleanup(machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData) (bool, error) {
	foundInstance, err := p.Get(machine, data)
	if err != nil {
		if err == cloudprovidererrors.ErrInstanceNotFound {
			return util.RemoveFinalizerOnInstanceNotFound(finalizerInstance, machine, data)
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
		return false, fmt.Errorf("failed to get alibaba client: %v", err)
	}

	deleteInstancesRequest := ecs.CreateDeleteInstancesRequest()
	deleteInstancesRequest.InstanceId = &[]string{foundInstance.ID()}

	deleteInstancesRequest.Force = requests.Boolean("True")
	if _, err = client.DeleteInstances(deleteInstancesRequest); err != nil {
		return false, fmt.Errorf("failed to delete instance with instanceID %s, due to %v", foundInstance.ID(), err)
	}

	return false, nil
}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["instanceType"] = c.InstanceType
		labels["region"] = c.RegionID
	}

	return labels, err
}

func (p *provider) MigrateUID(machine *clusterv1alpha1.Machine, new types.UID) error {
	c, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to decode providerconfig: %v", err)
	}

	client, err := getClient(c.RegionID, c.AccessKeyID, c.AccessKeySecret)
	if err != nil {
		return fmt.Errorf("failed to get alibaba client: %v", err)
	}

	foundInstance, err := getInstance(client, machine.Name, string(machine.UID))
	if err != nil {
		return fmt.Errorf("failed to get alibaba instance %v due to %v", machine.Name, err)
	}

	tag := ecs.AddTagsTag{
		Value: string(new),
		Key:   machineUIDTag,
	}
	request := ecs.CreateAddTagsRequest()
	request.ResourceId = foundInstance.InstanceId
	request.ResourceType = "instance"
	tags := []ecs.AddTagsTag{tag}
	request.Tag = &tags

	if _, err := client.AddTags(request); err != nil {
		return fmt.Errorf("failed to create new UID tag: %v", err)
	}

	return nil
}

func (p *provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	return nil
}

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, error) {
	if provSpec.Value == nil {
		return nil, nil, errors.New("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode providers config: %v", err)
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig, err := alibabatypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode alibaba providers config: %v", err)
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
	c.RegionID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.RegionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"regionID\" field, error = %v", err)
	}
	c.VSwitchID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VSwitchID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"vSwitchID\" field, error = %v", err)
	}
	c.ZoneID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.ZoneID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"zoneID\" field, error = %v", err)
	}
	c.InternetMaxBandwidthOut, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.InternetMaxBandwidthOut)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"internetMaxBandwidthOut\" field, error = %v", err)
	}
	c.Labels = rawConfig.Labels
	c.DiskType, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.DiskType)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"diskType\" field, error = %v", err)
	}
	c.DiskSize, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.DiskSize)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the value of \"diskSize\" field, error = %v", err)
	}

	return &c, pconfig, err
}

func getClient(regionID, accessKeyID, accessKeySecret string) (*ecs.Client, error) {
	client, err := ecs.NewClientWithAccessKey(regionID, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get Alibaba cloud client: %v", err)
	}
	return client, nil
}

func getInstance(client *ecs.Client, instanceName string, uid string) (*ecs.Instance, error) {
	describeInstanceRequest := ecs.CreateDescribeInstancesRequest()
	describeInstanceRequest.InstanceName = instanceName
	tag := []ecs.DescribeInstancesTag{
		{
			Key:   machineUIDTag,
			Value: uid,
		},
	}
	describeInstanceRequest.Tag = &tag

	response, err := client.DescribeInstances(describeInstanceRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance with instanceName: %s: %v", instanceName, err)
	}

	if response.Instances.Instance == nil ||
		len(response.Instances.Instance) == 0 ||
		response.GetHttpStatus() == http.StatusNotFound {
		return nil, cloudprovidererrors.ErrInstanceNotFound
	}

	return &response.Instances.Instance[0], nil
}

func (p *provider) getImageIDForOS(machineSpec clusterv1alpha1.MachineSpec, os providerconfigtypes.OperatingSystem) (string, error) {
	c, _, err := p.getConfig(machineSpec.ProviderSpec)
	if err != nil {
		return "", fmt.Errorf("failed to get alibaba client: %v", err)
	}

	client, err := getClient(c.RegionID, c.AccessKeyID, c.AccessKeySecret)
	if err != nil {
		return "", fmt.Errorf("failed to get alibaba client: %v", err)
	}

	request := ecs.CreateDescribeImagesRequest()
	request.InstanceType = "ecs.sn1ne.large"
	request.OSType = "linux"
	request.Architecture = "x86_64"

	response, err := client.DescribeImages(request)
	if err != nil {
		return "", fmt.Errorf("failed to describe alibaba images: %v", err)
	}

	var availableImage = map[providerconfigtypes.OperatingSystem]string{}
	for _, image := range response.Images.Image {
		switch image.OSNameEn {
		case ubuntuImageName:
			availableImage[providerconfigtypes.OperatingSystemUbuntu] = image.ImageId
		case centosImageName:
			availableImage[providerconfigtypes.OperatingSystemCentOS] = image.ImageId
		}
	}

	if imageID, ok := availableImage[os]; ok {
		return imageID, nil
	}

	return "", providerconfigtypes.ErrOSNotSupported
}
