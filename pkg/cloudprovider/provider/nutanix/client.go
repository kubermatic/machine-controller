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
	"errors"
	"fmt"

	nutanixclient "github.com/terraform-providers/terraform-provider-nutanix/client"
	nutanixv3 "github.com/terraform-providers/terraform-provider-nutanix/client/v3"

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

	if config.Username == "" || config.Password == "" {
		return nil, errors.New("no valid credentials")
	}

	if config.Endpoint == "" {
		return nil, errors.New("no endpoint provided")
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
	}

	if conf.CPUCores != nil {
		vmRes.NumVcpusPerSocket = conf.CPUCores
	}

	if conf.DiskSizeGB != nil {
		vmRes.DiskList[0].DiskSizeMib = pointer.Int64(*conf.DiskSizeGB * 1024)
	}

	vmSpec.Resources = vmRes
	vmRequest.Spec = vmSpec

	_, err = client.Prism.V3.CreateVM(vmRequest)
	if err != nil {
		return nil, err
	}

	return nil, nil
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
