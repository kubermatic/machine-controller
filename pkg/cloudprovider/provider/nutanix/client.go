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
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	nutanixclient "github.com/terraform-providers/terraform-provider-nutanix/client"
	nutanixv3 "github.com/terraform-providers/terraform-provider-nutanix/client/v3"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"

	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

const (
	vmKind      = "vm"
	projectKind = "project"
	clusterKind = "cluster"
	subnetKind  = "subnet"
	diskKind    = "disk"

	entityNotFoundError = "ENTITY_NOT_FOUND"
)

type ClientSet struct {
	Prism *nutanixv3.Client
}

func GetClientSet(config *Config) (*ClientSet, error) {
	if config == nil {
		return nil, errors.New("no configuration passed")
	}

	if config.Username == "" {
		return nil, errors.New("no username specified")
	}

	if config.Password == "" {
		return nil, errors.New("no password specificed")
	}

	if config.Endpoint == "" {
		return nil, errors.New("no endpoint specified")
	}

	credentials := nutanixclient.Credentials{
		URL:      config.Endpoint,
		Username: config.Username,
		Password: config.Password,
		Insecure: config.AllowInsecure,
	}

	clientV3, err := nutanixv3.NewV3Client(credentials)
	if err != nil {
		return nil, err
	}

	return &ClientSet{
		Prism: clientV3,
	}, nil
}

func createVM(client *ClientSet, name string, conf Config, os providerconfigtypes.OperatingSystem, userdata string) (instance.Instance, error) {
	cluster, err := getClusterByName(client, conf.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster: %v", err)
	}

	project, err := getProjectByName(client, conf.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %v", err)
	}

	subnet, err := getSubnetByName(client, conf.SubnetName, *cluster.Metadata.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subnet: %v", err)
	}

	image, err := getImageByName(client, conf.ImageName)
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %v", err)
	}

	vmRequest := &nutanixv3.VMIntentInput{
		Metadata: &nutanixv3.Metadata{
			Kind: pointer.String(vmKind),
			Name: pointer.String(name),
			ProjectReference: &nutanixv3.Reference{
				Kind: pointer.String(projectKind),
				UUID: project.Metadata.UUID,
			},
			Categories: conf.Categories,
		},
	}

	vmSpec := &nutanixv3.VM{
		ClusterReference: &nutanixv3.Reference{
			Kind: pointer.String(clusterKind),
			UUID: cluster.Metadata.UUID,
		},
	}

	vmRes := &nutanixv3.VMResources{
		NumSockets:    pointer.Int64(conf.CPUs),
		MemorySizeMib: pointer.Int64(conf.MemoryMB),
		NicList: []*nutanixv3.VMNic{
			{
				SubnetReference: &nutanixv3.Reference{
					Kind: pointer.String(subnetKind),
					UUID: subnet.Metadata.UUID,
				},
			},
		},
		DiskList: []*nutanixv3.VMDisk{
			{
				DataSourceReference: &nutanixv3.Reference{
					Kind: pointer.String(diskKind),
					UUID: image.Metadata.UUID,
				},
			},
		},
		GuestCustomization: &nutanixv3.GuestCustomization{
			CloudInit: &nutanixv3.GuestCustomizationCloudInit{
				UserData: pointer.String(base64.StdEncoding.EncodeToString([]byte(userdata))),
			},
		},
	}

	if conf.CPUCores != nil {
		vmRes.NumVcpusPerSocket = conf.CPUCores
	}

	if conf.DiskSizeGB != nil {
		vmRes.DiskList[0].DiskSizeMib = pointer.Int64(*conf.DiskSizeGB * 1024)
	}

	vmSpec.Resources = vmRes
	vmRequest.Spec = vmSpec

	resp, err := client.Prism.V3.CreateVM(vmRequest)
	if err != nil {
		return nil, err
	}

	taskUUID := resp.Status.ExecutionContext.TaskUUID.(string)

	if err := waitForCompletion(client, taskUUID, time.Second*10, time.Minute*30); err != nil {
		return nil, fmt.Errorf("failed to wait for task: %v", err)
	}

	if resp.Metadata.UUID == nil {
		return nil, errors.New("did not get response with UUID")
	}

	if err := waitForPowerState(client, *resp.Metadata.UUID, time.Second*10, time.Minute*30); err != nil {
		return nil, fmt.Errorf("failed to wait for power state: %v", err)
	}

	vm, err := client.Prism.V3.GetVM(*resp.Metadata.UUID)

	if vm.Metadata.Name == nil {
		return nil, fmt.Errorf("request for VM UUID '%s' did not return name", *resp.Metadata.UUID)
	}

	addresses, err := getIPs(client, *vm.Metadata.UUID, time.Second*5, time.Minute*10)
	if err != nil {
		return nil, fmt.Errorf("failed to get addresses: %v", err)
	}

	return Server{
		name:      *vm.Metadata.Name,
		id:        *resp.Metadata.UUID,
		status:    instance.StatusRunning,
		addresses: addresses,
	}, nil
}

func getSubnetByName(client *ClientSet, name, clusterUUID string) (*nutanixv3.SubnetIntentResponse, error) {
	filter := fmt.Sprintf("name==%s", name)
	subnets, err := client.Prism.V3.ListAllSubnet(filter)

	if err != nil {
		return nil, err
	}

	for _, subnet := range subnets.Entities {
		if *subnet.Metadata.Name == name && *subnet.Status.ClusterReference.UUID == clusterUUID {
			return subnet, nil
		}
	}

	return nil, errors.New(entityNotFoundError)
}

func getProjectByName(client *ClientSet, name string) (*nutanixv3.Project, error) {
	filter := fmt.Sprintf("name==%s", name)
	projects, err := client.Prism.V3.ListAllProject(filter)

	if err != nil {
		return nil, err
	}

	for _, project := range projects.Entities {
		if *project.Metadata.Name == name {
			return project, nil
		}
	}

	return nil, errors.New(entityNotFoundError)
}

func getClusterByName(client *ClientSet, name string) (*nutanixv3.ClusterIntentResponse, error) {
	filter := fmt.Sprintf("name==%s", name)
	clusters, err := client.Prism.V3.ListAllCluster(filter)

	if err != nil {
		return nil, err
	}

	for _, cluster := range clusters.Entities {
		if *cluster.Metadata.Name == name {
			return cluster, nil
		}
	}

	return nil, errors.New(entityNotFoundError)
}

func getImageByName(client *ClientSet, name string) (*nutanixv3.ImageIntentResponse, error) {
	filter := fmt.Sprintf("name==%s", name)
	images, err := client.Prism.V3.ListAllImage(filter)

	if err != nil {
		return nil, err
	}

	for _, image := range images.Entities {
		if *image.Metadata.Name == name {
			return image, nil
		}
	}

	return nil, errors.New(entityNotFoundError)
}

func getVMByName(client *ClientSet, name, projectID string) (*nutanixv3.VMIntentResource, error) {
	filter := fmt.Sprintf("name==%s", name)
	vms, err := client.Prism.V3.ListAllVM(filter)

	if err != nil {
		return nil, err
	}

	for _, vm := range vms.Entities {
		if *vm.Metadata.Name == name && *vm.Metadata.ProjectReference.UUID == projectID {
			return vm, nil
		}
	}

	return nil, errors.New(entityNotFoundError)
}

func getIPs(client *ClientSet, vmID string, interval time.Duration, timeout time.Duration) (map[string]corev1.NodeAddressType, error) {
	addresses := make(map[string]corev1.NodeAddressType)

	if err := wait.Poll(interval, timeout, func() (bool, error) {
		vm, err := client.Prism.V3.GetVM(vmID)
		if err != nil {
			return false, err
		}

		if len(vm.Status.Resources.NicList) == 0 || len(vm.Status.Resources.NicList[0].IPEndpointList) == 0 {
			return false, nil
		}

		ip := *vm.Status.Resources.NicList[0].IPEndpointList[0].IP
		addresses[ip] = corev1.NodeInternalIP

		return true, nil
	}); err != nil {
		return map[string]corev1.NodeAddressType{}, err
	}

	return addresses, nil
}
