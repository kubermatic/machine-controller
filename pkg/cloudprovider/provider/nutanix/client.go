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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	nutanixclient "github.com/embik/nutanix-client-go/pkg/client"
	nutanixv3 "github.com/embik/nutanix-client-go/pkg/client/v3"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	nutanixtypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/nutanix/types"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
)

const (
	invalidCredentials = "invalid Nutanix Credentials"
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

	// set up 9440 as default port if none is passed via config
	port := 9440
	if config.Port != nil {
		port = *config.Port
	}

	credentials := nutanixclient.Credentials{
		URL:      fmt.Sprintf("%s:%d", config.Endpoint, port),
		Endpoint: config.Endpoint,
		Port:     fmt.Sprint(port),
		Username: config.Username,
		Password: config.Password,
		Insecure: config.AllowInsecure,
	}

	if config.ProxyURL != "" {
		credentials.ProxyURL = config.ProxyURL
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
		return nil, err
	}

	subnet, err := getSubnetByName(client, conf.SubnetName, *cluster.Metadata.UUID)
	if err != nil {
		return nil, err
	}

	image, err := getImageByName(client, conf.ImageName)
	if err != nil {
		return nil, err
	}

	request := &nutanixv3.VMIntentInput{
		Metadata: &nutanixv3.Metadata{
			Kind:       pointer.String(nutanixtypes.VMKind),
			Categories: conf.Categories,
		},
		Spec: &nutanixv3.VM{
			Name: pointer.String(name),
			ClusterReference: &nutanixv3.Reference{
				Kind: pointer.String(nutanixtypes.ClusterKind),
				UUID: cluster.Metadata.UUID,
			},
		},
	}

	resources := &nutanixv3.VMResources{
		PowerState:    pointer.String("ON"),
		NumSockets:    pointer.Int64(conf.CPUs),
		MemorySizeMib: pointer.Int64(conf.MemoryMB),
		NicList: []*nutanixv3.VMNic{
			{
				SubnetReference: &nutanixv3.Reference{
					Kind: pointer.String(nutanixtypes.SubnetKind),
					UUID: subnet.Metadata.UUID,
				},
			},
		},
		DiskList: []*nutanixv3.VMDisk{
			{
				DeviceProperties: &nutanixv3.VMDiskDeviceProperties{
					DeviceType: pointer.String("DISK"),
					DiskAddress: &nutanixv3.DiskAddress{
						DeviceIndex: pointer.Int64(0),
						AdapterType: pointer.String("SCSI"),
					},
				},
				DataSourceReference: &nutanixv3.Reference{
					Kind: pointer.String(nutanixtypes.ImageKind),
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

	if conf.ProjectName != "" {
		project, err := getProjectByName(client, conf.ProjectName)
		if err != nil {
			return nil, fmt.Errorf("failed to get project: %v", err)
		}

		request.Metadata.ProjectReference = &nutanixv3.Reference{
			Kind: pointer.String(nutanixtypes.ProjectKind),
			UUID: project.Metadata.UUID,
		}
	}

	if conf.CPUCores != nil {
		resources.NumVcpusPerSocket = conf.CPUCores
	}

	if conf.CPUPassthrough != nil {
		resources.EnableCPUPassthrough = conf.CPUPassthrough
	}

	if conf.DiskSizeGB != nil {
		resources.DiskList[0].DiskSizeMib = pointer.Int64(*conf.DiskSizeGB * 1024)
	}

	request.Spec.Resources = resources

	resp, err := client.Prism.V3.CreateVM(request)
	if err != nil {
		return nil, wrapNutanixError(err)
	}

	taskUUID := resp.Status.ExecutionContext.TaskUUID.(string)

	if err := waitForCompletion(client, taskUUID, time.Second*10, time.Minute*15); err != nil {
		return nil, fmt.Errorf("failed to wait for task: %v", err)
	}

	if resp.Metadata.UUID == nil {
		return nil, errors.New("did not get response with UUID")
	}

	if err := waitForPowerState(client, *resp.Metadata.UUID, time.Second*10, time.Minute*10); err != nil {
		return nil, fmt.Errorf("failed to wait for power state: %v", err)
	}

	vm, err := client.Prism.V3.GetVM(*resp.Metadata.UUID)
	if err != nil {
		return nil, wrapNutanixError(err)
	}

	if vm.Status.Name == nil {
		return nil, fmt.Errorf("request for VM UUID '%s' did not return name", *resp.Metadata.UUID)
	}

	addresses, err := getIPs(client, *vm.Metadata.UUID, time.Second*5, time.Minute*10)
	if err != nil {
		return nil, fmt.Errorf("failed to get addresses: %v", err)
	}

	return Server{
		name:      *vm.Status.Name,
		id:        *resp.Metadata.UUID,
		status:    instance.StatusRunning,
		addresses: addresses,
	}, nil
}

func getSubnetByName(client *ClientSet, name, clusterID string) (*nutanixv3.SubnetIntentResponse, error) {
	filter := fmt.Sprintf("name==%s", name)
	subnets, err := client.Prism.V3.ListAllSubnet(filter)

	if err != nil {
		return nil, wrapNutanixError(err)
	}

	for _, subnet := range subnets.Entities {
		if subnet != nil && subnet.Status != nil && subnet.Status.Name != nil && *subnet.Status.Name == name {
			// some subnet types (e.g. VPC overlays) do not come with a cluster reference; we don't need to check them
			if subnet.Status.ClusterReference == nil || (subnet.Status.ClusterReference.UUID != nil && *subnet.Status.ClusterReference.UUID == clusterID) {
				return subnet, nil
			}
		}
	}

	return nil, cloudprovidererrors.TerminalError{
		Reason:  common.InvalidConfigurationMachineError,
		Message: fmt.Sprintf("no subnet found for name==%s", name),
	}
}

func getProjectByName(client *ClientSet, name string) (*nutanixv3.Project, error) {
	filter := fmt.Sprintf("name==%s", name)
	projects, err := client.Prism.V3.ListAllProject(filter)

	if err != nil {
		return nil, wrapNutanixError(err)
	}

	if projects == nil || projects.Entities == nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("no project found for name==%s", name),
		}

	}

	for _, project := range projects.Entities {
		if project != nil && project.Status != nil && project.Status.Name == name {
			return project, nil
		}
	}

	return nil, cloudprovidererrors.TerminalError{
		Reason:  common.InvalidConfigurationMachineError,
		Message: fmt.Sprintf("no project found for name==%s", name),
	}
}

func getClusterByName(client *ClientSet, name string) (*nutanixv3.ClusterIntentResponse, error) {
	filter := fmt.Sprintf("name==%s", name)
	clusters, err := client.Prism.V3.ListAllCluster(filter)

	if err != nil {
		return nil, wrapNutanixError(err)
	}

	if clusters == nil || clusters.Entities == nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("no cluster found for name==%s", name),
		}
	}

	for _, cluster := range clusters.Entities {
		if cluster.Status != nil && cluster.Status.Name != nil && *cluster.Status.Name == name {
			return cluster, nil
		}
	}

	return nil, cloudprovidererrors.TerminalError{
		Reason:  common.InvalidConfigurationMachineError,
		Message: fmt.Sprintf("no cluster found for name==%s", name),
	}
}

func getImageByName(client *ClientSet, name string) (*nutanixv3.ImageIntentResponse, error) {
	filter := fmt.Sprintf("name==%s", name)
	images, err := client.Prism.V3.ListAllImage(filter)

	if err != nil {
		return nil, wrapNutanixError(err)
	}

	if images == nil || images.Entities == nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("no image found for name==%s", name),
		}
	}

	for _, image := range images.Entities {
		if image.Status != nil && image.Status.Name != nil && *image.Status.Name == name {
			return image, nil
		}
	}

	return nil, cloudprovidererrors.TerminalError{
		Reason:  common.InvalidConfigurationMachineError,
		Message: fmt.Sprintf("no image found for name==%s", name),
	}
}

func getVMByName(client *ClientSet, name string, projectID *string) (*nutanixv3.VMIntentResource, error) {
	filter := fmt.Sprintf("vm_name==%s", name)
	vms, err := client.Prism.V3.ListAllVM(filter)

	if err != nil {
		return nil, wrapNutanixError(err)
	}

	for _, vm := range vms.Entities {
		if *vm.Status.Name == name {
			if projectID != nil && vm.Metadata != nil && vm.Metadata.ProjectReference != nil &&
				vm.Metadata.ProjectReference.UUID != nil && *vm.Metadata.ProjectReference.UUID != *projectID {
				continue
			}
			return vm, nil
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func getIPs(client *ClientSet, vmID string, interval time.Duration, timeout time.Duration) (map[string]corev1.NodeAddressType, error) {
	addresses := make(map[string]corev1.NodeAddressType)

	if err := wait.Poll(interval, timeout, func() (bool, error) {
		vm, err := client.Prism.V3.GetVM(vmID)
		if err != nil {
			return false, wrapNutanixError(err)
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

func waitForCompletion(client *ClientSet, taskID string, interval time.Duration, timeout time.Duration) error {
	return wait.Poll(interval, timeout, func() (bool, error) {
		task, err := client.Prism.V3.GetTask(taskID)
		if err != nil {
			return false, wrapNutanixError(err)
		}

		if task.Status == nil {
			return false, nil
		}

		switch *task.Status {
		case "INVALID_UUID", "FAILED":
			return false, fmt.Errorf("bad status: %s", *task.Status)
		case "QUEUED", "RUNNING":
			return false, nil
		case "SUCCEEDED":
			return true, nil
		default:
			return false, fmt.Errorf("unknown status: %s", *task.Status)
		}

	})
}

func waitForPowerState(client *ClientSet, vmID string, interval time.Duration, timeout time.Duration) error {
	return wait.Poll(interval, timeout, func() (bool, error) {
		vm, err := client.Prism.V3.GetVM(vmID)
		if err != nil {
			return false, wrapNutanixError(err)
		}

		if vm.Status == nil || vm.Status.Resources == nil || vm.Status.Resources.PowerState == nil {
			return false, nil
		}

		switch *vm.Status.Resources.PowerState {
		case "ON":
			return true, nil
		case "OFF":
			return false, nil
		default:
			return false, fmt.Errorf("unexpected power state: %s", *vm.Status.Resources.PowerState)
		}
	})
}

func wrapNutanixError(initialErr error) error {
	if initialErr == nil {
		return nil
	}

	var resp nutanixtypes.ErrorResponse

	if err := json.Unmarshal([]byte(initialErr.Error()), &resp); err != nil {
		// invalid credentials are returned with a simple string
		if strings.Contains(initialErr.Error(), invalidCredentials) {
			return cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: initialErr.Error(),
			}
		}

		// failed to parse error, let's make sure it doesnt't have new lines at least
		return fmt.Errorf("api error: %s", strings.ReplaceAll(initialErr.Error(), "\n", ""))
	}

	// TODO: handle different states by potentially returning a TerminalError
	// this needs experience with errors coming from Nutanix because the state
	// values are not defined anywhere. So if you hit an error that qualifies,
	// why not add something handling it!
	switch resp.State {
	default:
		var msgs []string
		for _, msg := range resp.MessageList {
			msgs = append(msgs, fmt.Sprintf("%s: %s", msg.Message, msg.Reason))
		}

		return fmt.Errorf("api error (%s, code %d): %s", resp.State, resp.Code, strings.Join(msgs, ", "))
	}
}
