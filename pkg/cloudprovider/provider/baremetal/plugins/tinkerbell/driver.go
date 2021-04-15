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
	tinktmpl "github.com/tinkerbell/tink/protos/template"
	"gopkg.in/yaml.v3"

	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins"
	tinkerbellclient "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/client"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

type driver struct {
	TinkServerAddress string
	ImageRepoAddress  string

	hardwareClient *tinkerbellclient.Hardware
	templateClient *tinkerbellclient.Template
	workflowClient *tinkerbellclient.Workflow
}

// NewTinkerbellDriver returns a new TinkerBell driver with a configured tinkserver address and a client timeout.
func NewTinkerbellDriver(tinkServerAddress, imageRepoAddress string) (plugins.PluginDriver, error) {
	if tinkServerAddress == "" || imageRepoAddress == "" {
		return nil, errors.New("tink-server address, ImageRepoAddress cannot be empty")
	}

	if err := tinkclient.Setup(); err != nil {
		return nil, fmt.Errorf("failed to setup tink-server client: %v", err)
	}

	d := &driver{
		TinkServerAddress: tinkServerAddress,
		ImageRepoAddress:  imageRepoAddress,
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
	if err := d.hardwareClient.Create(ctx, hw.Hardware.Hardware); err != nil {
		return nil, fmt.Errorf("failed to register hardware to tink-server: %v", err)
	}

	workflowTemplate, err := d.templateClient.Get(ctx, "", provisioningTemplate)
	if err != nil {
		if resourceNotFoundErr(err) {
			tmpl := createTemplate(hw.GetMACAddress(), d.TinkServerAddress, d.ImageRepoAddress, cfg)
			payload, err := yaml.Marshal(tmpl)
			if err != nil {
				return nil, fmt.Errorf("failed marshalling workflow template: %v", err)
			}

			workflowTemplate := &tinktmpl.WorkflowTemplate{
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

func (d *driver) DeprovisionServer(uid types.UID, hwSpec runtime.RawExtension) (string, error) {
	return "", nil
}

func resourceNotFoundErr(err error) bool {
	hardwareErrorMsg := fmt.Sprintf("hardware %s", tinkerbellclient.ErrNotFound.Error())
	return err.Error() == hardwareErrorMsg
}
