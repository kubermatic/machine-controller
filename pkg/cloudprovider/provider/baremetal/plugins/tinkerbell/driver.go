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
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins"
	tinkerbellclient "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/client"
	metadataclient "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/metadata"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

type ClientFactory func() (metadataclient.Client, tinkerbellclient.HardwareClient, tinkerbellclient.TemplateClient, tinkerbellclient.WorkflowClient)

type driver struct {
	TinkServerAddress string
	ImageRepoAddress  string

	metadataClient metadataclient.Client
	hardwareClient tinkerbellclient.HardwareClient
	templateClient tinkerbellclient.TemplateClient
	workflowClient tinkerbellclient.WorkflowClient
}

// NewTinkerbellDriver returns a new TinkerBell driver with a configured tinkserver address and a client timeout.
func NewTinkerbellDriver(mdConfig *metadataclient.Config, factory ClientFactory, tinkServerAddress, imageRepoAddress string) (plugins.PluginDriver, error) {
	if tinkServerAddress == "" || imageRepoAddress == "" {
		return nil, errors.New("tink-server address, ImageRepoAddress cannot be empty")
	}

	var (
		mdClient   metadataclient.Client
		hwClient   tinkerbellclient.HardwareClient
		tmplClient tinkerbellclient.TemplateClient
		wflClient  tinkerbellclient.WorkflowClient
		err        error
	)

	if factory == nil {
		mdClient, err = metadataclient.NewMetadataClient(mdConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create metadata client: %w", err)
		}

		if err := tinkclient.Setup(); err != nil {
			return nil, fmt.Errorf("failed to setup tink-server client: %w", err)
		}

		hwClient = tinkerbellclient.NewHardwareClient(tinkclient.HardwareClient)
		tmplClient = tinkerbellclient.NewTemplateClient(tinkclient.TemplateClient)
		wflClient = tinkerbellclient.NewWorkflowClient(tinkclient.WorkflowClient, tinkerbellclient.NewHardwareClient(tinkclient.HardwareClient))
	} else {
		mdClient, hwClient, tmplClient, wflClient = factory()
	}

	d := &driver{
		TinkServerAddress: tinkServerAddress,
		ImageRepoAddress:  imageRepoAddress,
		metadataClient:    mdClient,
		hardwareClient:    hwClient,
		templateClient:    tmplClient,
		workflowClient:    wflClient,
	}

	return d, nil
}

func (d *driver) GetServer(ctx context.Context, uid types.UID, hwSpec runtime.RawExtension) (plugins.Server, error) {
	hw := HardwareSpec{}
	if err := json.Unmarshal(hwSpec.Raw, &hw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tinkerbell hardware spec: %w", err)
	}

	fetchedHW, err := d.hardwareClient.Get(ctx, string(uid), hw.GetIPAddress(),
		hw.GetMACAddress())
	if err != nil {
		if resourceNotFoundErr(err) {
			return nil, cloudprovidererrors.ErrInstanceNotFound
		}

		return nil, fmt.Errorf("failed to get hardware: %w", err)
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
		return nil, fmt.Errorf("failed to unmarshal tinkerbell hardware spec: %w", err)
	}
	hw.Hardware.Id = string(uid)
	_, err := d.hardwareClient.Get(ctx, hw.Hardware.Id, "", "")
	if err != nil {
		if resourceNotFoundErr(err) {
			cfg, err := d.metadataClient.GetMachineMetadata()
			if err != nil {
				return nil, fmt.Errorf("failed to get metadata configs: %w", err)
			}

			hw.Hardware.Network.Interfaces[0].Dhcp.Mac = cfg.MACAddress

			ip, netmask, _, err := util.CIDRToIPAndNetMask(cfg.CIDR)
			if err != nil {
				return nil, fmt.Errorf("failed to parse CIDR: %w", err)
			}
			dhcpIP := &hardware.Hardware_DHCP_IP{
				Address: ip,
				Netmask: netmask,
				Gateway: cfg.Gateway,
			}
			hw.Hardware.Network.Interfaces[0].Dhcp.Ip = dhcpIP

			if err := d.hardwareClient.Create(ctx, hw.Hardware.Hardware); err != nil {
				return nil, fmt.Errorf("failed to register hardware to tink-server: %w", err)
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
				return nil, fmt.Errorf("failed marshalling workflow template: %w", err)
			}

			workflowTemplate = &tinktmpl.WorkflowTemplate{
				Name: tmpl.Name,
				Id:   tmpl.ID,
				Data: string(payload),
			}

			if err := d.templateClient.Create(ctx, workflowTemplate); err != nil {
				return nil, fmt.Errorf("failed to create workflow template: %w", err)
			}
		}
	}

	if _, err := d.workflowClient.Create(ctx, workflowTemplate.Id, hw.GetID()); err != nil {
		return nil, fmt.Errorf("failed to provision server id %s running template id %s: %w", workflowTemplate.Id, hw.GetID(), err)
	}

	return &hw, nil
}

func (d *driver) Validate(hwSpec runtime.RawExtension) error {
	hw := HardwareSpec{}
	if err := json.Unmarshal(hwSpec.Raw, &hw); err != nil {
		return fmt.Errorf("failed to unmarshal tinkerbell hardware spec: %w", err)
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
		return fmt.Errorf("failed to delete tinkerbell hardware data: %w", err)
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
