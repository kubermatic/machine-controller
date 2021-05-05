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

package tinkerbell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	tinkclient "github.com/tinkerbell/tink/client"
	tinkpkg "github.com/tinkerbell/tink/pkg"
	"github.com/tinkerbell/tink/protos/hardware"
	tinktmpl "github.com/tinkerbell/tink/protos/template"
	"gopkg.in/yaml.v3"

	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	metadataclient "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/metadata/nautobot"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins"
	tinkerbellclient "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/client"
	"github.com/kubermatic/machine-controller/pkg/nautobot"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

type driver struct {
	TinkServerAddress string
	ImageRepoAddress  string

	metadataClient *metadataclient.Client
	hardwareClient *tinkerbellclient.Hardware
	templateClient *tinkerbellclient.Template
	workflowClient *tinkerbellclient.Workflow
}

// NewTinkerbellDriver returns a new TinkerBell driver with a configured tinkserver address and a client timeout.
func NewTinkerbellDriver(mdConfig *metadataclient.MetadataClientConfig, tinkServerAddress, imageRepoAddress string) (plugins.PluginDriver, error) {
	if tinkServerAddress == "" || imageRepoAddress == "" {
		return nil, errors.New("tink-server address, ImageRepoAddress cannot be empty")
	}

	if err := tinkclient.Setup(); err != nil {
		return nil, fmt.Errorf("failed to setup tink-server client: %v", err)
	}

	mdClient, err := metadataclient.NewClient(mdConfig.Config.Token, mdConfig.Config.APIServer, mdConfig.Config.Tag)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata client: %v", err)
	}

	d := &driver{
		TinkServerAddress: tinkServerAddress,
		ImageRepoAddress:  imageRepoAddress,
		metadataClient:    mdClient,
		hardwareClient:    tinkerbellclient.NewHardwareClient(tinkclient.HardwareClient),
		workflowClient:    tinkerbellclient.NewWorkflowClient(tinkclient.WorkflowClient, tinkerbellclient.NewHardwareClient(tinkclient.HardwareClient)),
		templateClient:    tinkerbellclient.NewTemplateClient(tinkclient.TemplateClient),
	}

	return d, nil
}

func (d *driver) GetServer(ctx context.Context, uid types.UID, hwSpec runtime.RawExtension) (plugins.Server, error) {
	hw := HardwareSpec{}
	if err := json.Unmarshal(hwSpec.Raw, &hw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tinkerbell hardware spec: %v", err)
	}

	fetchedHW, err := d.hardwareClient.Get(ctx, string(uid), hw.GetIPAddress(),
		hw.GetMACAddress())
	if err != nil {
		if resourceNotFoundErr(err) {
			return nil, cloudprovidererrors.ErrInstanceNotFound
		}

		return nil, fmt.Errorf("failed to get hardware: %v", err)
	}

	return &HardwareSpec{
		Hardware: tinkpkg.HardwareWrapper{
			Hardware: fetchedHW,
		},
	}, nil
}

func (d *driver) ProvisionServer(ctx context.Context, uid types.UID, cfg *plugins.CloudConfigSettings, hwSpec runtime.RawExtension) (plugins.Server, error) {
	hw := HardwareSpec{}
	if err := json.Unmarshal(hwSpec.Raw, &hw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tinkerbell hardware spec: %v", err)
	}
	hw.Hardware.Id = string(uid)
	_, err := d.hardwareClient.Get(ctx, hw.Hardware.Id, "", "")
	if err != nil {
		if resourceNotFoundErr(err) {
			ipConfig, mac, deviceID, err := d.dhcpConfig()
			if err != nil {
				return nil, fmt.Errorf("failed to get dhcp config: %v", err)
			}

			hw.Hardware.Network.Interfaces[0].Dhcp.Mac = mac
			hw.Hardware.Network.Interfaces[0].Dhcp.Ip = ipConfig

			if err := d.hardwareClient.Create(ctx, hw.Hardware.Hardware); err != nil {
				return nil, fmt.Errorf("failed to register hardware to tink-server: %v", err)
			}

			params := &nautobot.PatchedDeviceParams{
				Status:   nautobot.Staged,
				AssetTag: string(uid),
			}
			if err := d.metadataClient.PatchDeviceStatus(deviceID, params); err != nil {
				return nil, fmt.Errorf("failed to patch server device status: %v", err)
			}
		}
	}

	// cfg.SecretName has the same name as the machine name
	workflowTemplate, err := d.templateClient.Get(ctx, "", cfg.SecretName)
	if err != nil {
		if resourceNotFoundErr(err) {
			tmpl := createTemplate(d.TinkServerAddress, d.ImageRepoAddress, cfg)
			payload, err := yaml.Marshal(tmpl)
			if err != nil {
				return nil, fmt.Errorf("failed marshalling workflow template: %v", err)
			}

			workflowTemplate = &tinktmpl.WorkflowTemplate{
				Name: tmpl.Name,
				Id:   tmpl.ID,
				Data: string(payload),
			}

			if err := d.templateClient.Create(ctx, workflowTemplate); err != nil {
				return nil, fmt.Errorf("failed to create workflow template: %v", err)
			}
		}
	}

	if _, err := d.workflowClient.Create(ctx, workflowTemplate.Id, hw.GetID()); err != nil {
		return nil, fmt.Errorf("failed to provisioing server id %s running template id %s: %v", workflowTemplate.Id, hw.GetID(), err)
	}

	return &hw, nil
}

func (d *driver) Validate(hwSpec runtime.RawExtension) error {
	hw := HardwareSpec{}
	if err := json.Unmarshal(hwSpec.Raw, &hw); err != nil {
		return fmt.Errorf("failed to unmarshal tinkerbell hardware spec: %v", err)
	}

	if hw.Hardware.Hardware == nil {
		return fmt.Errorf("tinkerbell hardware data can not be empty")
	}

	if hw.Hardware.Network == nil {
		return fmt.Errorf("tinkerbell hardware network configs can not be empty")
	}

	if hw.Hardware.Metadata == "" {
		return fmt.Errorf("tinkerbell hardware metadata can not be empty")
	}

	return nil
}

func (d *driver) DeprovisionServer(ctx context.Context, uid types.UID) error {
	if err := d.hardwareClient.Delete(ctx, string(uid)); err != nil {
		if resourceNotFoundErr(err) {
			return nil
		}
		return fmt.Errorf("failed to delete tinkerbell hardware data: %v", err)
	}

	params := &nautobot.PatchedDeviceParams{
		Status:   nautobot.Active,
		AssetTag: "",
	}

	device, err := d.metadataClient.GetDeviceByMachineUID(string(uid))
	if err != nil {
		return fmt.Errorf("failed to get machine device: %v", err)
	}

	if err := d.metadataClient.PatchDeviceStatus(device.ID, params); err != nil {
		return fmt.Errorf("failed to patch server device status: %v", err)
	}

	return nil
}

func resourceNotFoundErr(err error) bool {
	switch err.Error() {
	case fmt.Sprintf("hardware %s", tinkerbellclient.ErrNotFound.Error()):
		return true
	case fmt.Sprintf("template %s", tinkerbellclient.ErrNotFound.Error()):
		return true
	}

	return false
}

func (d *driver) dhcpConfig() (*hardware.Hardware_DHCP_IP, string, string, error) {
	deviceID, err := d.metadataClient.GetActiveDevice()
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get device: %v", err)
	}

	machineCIDR, err := d.metadataClient.GetMachineCIDR(deviceID)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get machine cidr: %v", err)
	}

	machineIP, netmask, _, err := nautobot.CidrToIPAndNetMask(machineCIDR.Address)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get ip and netmask: %v", err)
	}

	macAddress, err := d.metadataClient.GetMachineMacAddress(deviceID)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get mchine mac address: %v", err)
	}

	gatewayIP, err := d.metadataClient.GetMachineGatewayIP(machineCIDR, "router")
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get mchine mac address: %v", err)
	}

	return &hardware.Hardware_DHCP_IP{
		Address: machineIP,
		Netmask: netmask,
		Gateway: gatewayIP,
	}, macAddress, deviceID, nil

}
