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
	"reflect"
	"testing"

	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/template"
	"github.com/tinkerbell/tink/workflow"

	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins"
	tinkerbellclient "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/client"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/baremetal/plugins/tinkerbell/metadata"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewTinkerbellDriver(t *testing.T) {
	var testCases = []struct {
		name            string
		tinkServer      string
		imageRepoServer string
		clientFactor    ClientFactory
		errorIsExpected bool
	}{
		{
			name:            "create new tinkerbell driver failure, missing image repo server",
			tinkServer:      "10.129.8.102",
			imageRepoServer: "",
			errorIsExpected: true,
		},
		{
			name:            "create new tinkerbell driver failure, missing tink server",
			tinkServer:      "",
			imageRepoServer: "10.129.8.102:8080",
			errorIsExpected: true,
		},
		{
			name:            "create new tinkerbell driver success",
			tinkServer:      "10.129.8.102",
			imageRepoServer: "10.129.8.102:8080",
			clientFactor: func() (metadata.Client, tinkerbellclient.HardwareClient, tinkerbellclient.TemplateClient, tinkerbellclient.WorkflowClient) {
				return &fakeMetadataClient{}, &fakeHardwareClient{}, &fakeTemplateClient{}, &fakeWorkflowClient{}
			},
			errorIsExpected: false,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewTinkerbellDriver(nil, test.clientFactor, test.tinkServer, test.imageRepoServer)
			if err != nil {
				if test.errorIsExpected {
					return
				}

				t.Fatalf("failed to create tinkerbell client: %v", err)
			}
		})
	}
}

func TestDriver_GetServer(t *testing.T) {
	var testCases = []struct {
		name                 string
		tinkServer           string
		imageRepoServer      string
		hardwareSpec         runtime.RawExtension
		clientFactor         ClientFactory
		expectedHardwareSpec string
		errorIsExpected      bool
		expectedError        error
	}{
		{
			name:            "failed to get server",
			tinkServer:      "10.129.8.102",
			imageRepoServer: "10.129.8.102:8080",
			hardwareSpec:    runtime.RawExtension{Raw: []byte("{\n    \"hardware\": {\n        \"network\": {\n            \"interfaces\": [\n                {\n                    \"dhcp\": {\n                        \"ip\": {\n                            \"address\": \"10.129.8.90\"\n                        },\n                        \"mac\": \"18:C0:4D:B1:18:E3\"\n                    }\n                }\n            ]\n        }\n    }\n}")},
			clientFactor: func() (metadata.Client, tinkerbellclient.HardwareClient, tinkerbellclient.TemplateClient, tinkerbellclient.WorkflowClient) {
				return &fakeMetadataClient{}, &fakeHardwareClient{
					err: &resourceError{
						resource: "hardware",
					},
				}, &fakeTemplateClient{}, &fakeWorkflowClient{}
			},
			errorIsExpected: true,
			expectedError:   cloudprovidererrors.ErrInstanceNotFound,
		},
		{
			name:            "get server success",
			tinkServer:      "10.129.8.102",
			imageRepoServer: "10.129.8.102:8080",
			hardwareSpec:    runtime.RawExtension{Raw: []byte("{\n    \"hardware\": {\n        \"network\": {\n            \"interfaces\": [\n                {\n                    \"dhcp\": {\n                        \"ip\": {\n                            \"address\": \"10.129.8.90\"\n                        },\n                        \"mac\": \"18:C0:4D:B1:18:E3\"\n                    }\n                }\n            ]\n        }\n    }\n}")},
			clientFactor: func() (metadata.Client, tinkerbellclient.HardwareClient, tinkerbellclient.TemplateClient, tinkerbellclient.WorkflowClient) {
				return &fakeMetadataClient{}, &fakeHardwareClient{}, &fakeTemplateClient{}, &fakeWorkflowClient{}
			},
			errorIsExpected:      false,
			expectedHardwareSpec: "{\n    \"hardware\": {\n        \"metadata\": {\n            \"facility\": {\n                \"facility_code\": \"ewr1\",\n                \"plan_slug\": \"c2.medium.x86\",\n                \"plan_version_slug\": \"\"\n            },\n            \"instance\": {\n                \"operating_system_version\": {\n                    \"distro\": \"ubuntu\",\n                    \"os_slug\": \"ubuntu_18_04\",\n                    \"version\": \"18.04\"\n                }\n            },\n            \"state\": \"\"\n        },\n        \"network\": {\n            \"interfaces\": [\n                {\n                    \"dhcp\": {\n                        \"arch\": \"x86_64\",\n                        \"ip\": {\n                            \"address\": \"10.129.8.90\",\n                            \"gateway\": \"10.129.8.89\",\n                            \"netmask\": \"255.255.255.252\"\n                        },\n                        \"mac\": \"18:C0:4D:B1:18:E3\",\n                        \"uefi\": false\n                    },\n                    \"netboot\": {\n                        \"allow_pxe\": true,\n                        \"allow_workflow\": true\n                    }\n                }\n            ]\n        }\n    }\n}",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			d, err := NewTinkerbellDriver(nil, test.clientFactor, test.tinkServer, test.imageRepoServer)
			if err != nil {
				t.Fatalf("failed to create tinkerbell driver: %v", err)
			}

			ctx := context.Background()
			s, err := d.GetServer(ctx, "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94", test.hardwareSpec)
			if err != nil {
				if test.errorIsExpected && errors.Is(err, test.expectedError) {
					return
				}

				t.Fatalf("failed to execute get server: %v", err)
			}

			hw := &HardwareSpec{}
			if err := json.Unmarshal([]byte(test.expectedHardwareSpec), hw); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(hw, s) {
				t.Fatal("server spec and hardware spec mismatched")
			}
		})
	}
}

func TestDriver_ProvisionServer(t *testing.T) {
	var testCases = []struct {
		name                 string
		tinkServer           string
		imageRepoServer      string
		hardwareSpec         runtime.RawExtension
		clientFactory        ClientFactory
		cloudConfig          *plugins.CloudConfigSettings
		expectedHardwareSpec string
		errorIsExpected      bool
		expectedError        error
	}{
		{
			name:            "provision server success",
			tinkServer:      "10.129.8.102",
			imageRepoServer: "10.129.8.102:8080",
			hardwareSpec:    runtime.RawExtension{Raw: []byte("{\n    \"hardware\": {\n        \"metadata\": {\n            \"facility\": {\n                \"facility_code\": \"ewr1\",\n                \"plan_slug\": \"c2.medium.x86\",\n                \"plan_version_slug\": \"\"\n            },\n            \"instance\": {\n                \"operating_system_version\": {\n                    \"distro\": \"ubuntu\",\n                    \"os_slug\": \"ubuntu_18_04\",\n                    \"version\": \"18.04\"\n                }\n            },\n            \"state\": \"\"\n        },\n        \"network\": {\n            \"interfaces\": [\n                {\n                    \"dhcp\": {\n                        \"arch\": \"x86_64\",\n                        \"ip\": {\n                            \"address\": \"10.129.8.90\",\n                            \"gateway\": \"10.129.8.89\",\n                            \"netmask\": \"255.255.255.252\"\n                        },\n                        \"mac\": \"18:C0:4D:B1:18:E3\",\n                        \"uefi\": false\n                    },\n                    \"netboot\": {\n                        \"allow_pxe\": true,\n                        \"allow_workflow\": true\n                    }\n                }\n            ]\n        }\n    }\n}")},
			clientFactory: func() (metadata.Client, tinkerbellclient.HardwareClient, tinkerbellclient.TemplateClient, tinkerbellclient.WorkflowClient) {
				return &fakeMetadataClient{}, &fakeHardwareClient{
					err: &resourceError{
						resource: "hardware",
					},
				}, &fakeTemplateClient{}, &fakeWorkflowClient{}
			},
			cloudConfig: &plugins.CloudConfigSettings{
				Token:       "test-token",
				Namespace:   "kube-system",
				SecretName:  "test-secret",
				ClusterHost: "10.10.10.10",
			},
			expectedHardwareSpec: "{\n    \"hardware\": {\n        \"id\": \"0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94\",\n        \"metadata\": {\n            \"facility\": {\n                \"facility_code\": \"ewr1\",\n                \"plan_slug\": \"c2.medium.x86\",\n                \"plan_version_slug\": \"\"\n            },\n            \"instance\": {\n                \"operating_system_version\": {\n                    \"distro\": \"ubuntu\",\n                    \"os_slug\": \"ubuntu_18_04\",\n                    \"version\": \"18.04\"\n                }\n            },\n            \"state\": \"\"\n        },\n        \"network\": {\n            \"interfaces\": [\n                {\n                    \"dhcp\": {\n                        \"arch\": \"x86_64\",\n                        \"ip\": {\n                            \"address\": \"10.129.8.90\",\n                            \"gateway\": \"10.129.8.89\",\n                            \"netmask\": \"255.255.255.252\"\n                        },\n                        \"mac\": \"18:C0:4D:B1:18:E3\",\n                        \"uefi\": false\n                    },\n                    \"netboot\": {\n                        \"allow_pxe\": true,\n                        \"allow_workflow\": true\n                    }\n                }\n            ]\n        }\n    }\n}",
			errorIsExpected:      false,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			d, err := NewTinkerbellDriver(nil, test.clientFactory, test.tinkServer, test.imageRepoServer)
			if err != nil {
				t.Fatalf("failed to create tinkerbell driver: %v", err)
			}

			ctx := context.Background()
			s, err := d.ProvisionServer(ctx, "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94", test.cloudConfig, test.hardwareSpec)
			if err != nil {
				t.Fatalf("failed to execute provision server: %v", err)
			}

			hw := &HardwareSpec{}
			if err := json.Unmarshal([]byte(test.expectedHardwareSpec), hw); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(hw, s) {
				t.Fatal("server spec and hardware spec mismatched")
			}
		})
	}
}

type fakeMetadataClient struct{}

func (f *fakeMetadataClient) GetMachineMetadata() (*metadata.MachineMetadata, error) {
	return &metadata.MachineMetadata{
		CIDR:       "10.129.8.90/30",
		MACAddress: "18:C0:4D:B1:18:E3",
		Gateway:    "10.129.8.89",
	}, nil
}

type fakeHardwareClient struct {
	err *resourceError
}

func (f *fakeHardwareClient) Get(_ context.Context, _ string, _ string, _ string) (*hardware.Hardware, error) {
	if f.err != nil {
		return nil, f.err
	}

	return &hardware.Hardware{
		Metadata: "{\"facility\":{\"facility_code\":\"ewr1\",\"plan_slug\":\"c2.medium.x86\",\"plan_version_slug\":\"\"},\"instance\":{\"operating_system_version\":{\"distro\":\"ubuntu\",\"os_slug\":\"ubuntu_18_04\",\"version\":\"18.04\"}},\"state\":\"\"}",
		Network: &hardware.Hardware_Network{
			Interfaces: []*hardware.Hardware_Network_Interface{
				{
					Dhcp: &hardware.Hardware_DHCP{
						Arch: "x86_64",
						Uefi: false,
						Mac:  "18:C0:4D:B1:18:E3",
						Ip: &hardware.Hardware_DHCP_IP{
							Address: "10.129.8.90",
							Netmask: "255.255.255.252",
							Gateway: "10.129.8.89",
						},
					},
					Netboot: &hardware.Hardware_Netboot{
						AllowPxe:      true,
						AllowWorkflow: true,
					},
				},
			},
		},
	}, nil
}

func (f *fakeHardwareClient) Delete(_ context.Context, _ string) error {
	return nil
}

func (f *fakeHardwareClient) Create(_ context.Context, hw *hardware.Hardware) error {
	expectedHW := &hardware.Hardware{
		Id:       "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94",
		Metadata: "{\"facility\":{\"facility_code\":\"ewr1\",\"plan_slug\":\"c2.medium.x86\",\"plan_version_slug\":\"\"},\"instance\":{\"operating_system_version\":{\"distro\":\"ubuntu\",\"os_slug\":\"ubuntu_18_04\",\"version\":\"18.04\"}},\"state\":\"\"}",
		Network: &hardware.Hardware_Network{
			Interfaces: []*hardware.Hardware_Network_Interface{
				{
					Dhcp: &hardware.Hardware_DHCP{
						Arch: "x86_64",
						Uefi: false,
						Mac:  "18:C0:4D:B1:18:E3",
						Ip: &hardware.Hardware_DHCP_IP{
							Address: "10.129.8.90",
							Netmask: "255.255.255.252",
							Gateway: "10.129.8.89",
						},
					},
					Netboot: &hardware.Hardware_Netboot{
						AllowPxe:      true,
						AllowWorkflow: true,
					},
				},
			},
		},
	}

	if !reflect.DeepEqual(hw, expectedHW) {
		return errors.New("unexpected hardware data")
	}

	return nil
}

type fakeTemplateClient struct{}

func (f *fakeTemplateClient) Get(_ context.Context, _ string, _ string) (*template.WorkflowTemplate, error) {
	wfl := &workflow.Workflow{
		Version:       "0.1",
		Name:          "fake_template",
		GlobalTimeout: 6000,
		Tasks: []workflow.Task{
			{
				Name:       "disk-wipe",
				WorkerAddr: "{{.device_1}}",
				Volumes: []string{
					"/dev:/dev",
					"/dev/console:/dev/console",
					"/lib/firmware:/lib/firmware:ro",
				},
				Actions: []workflow.Action{
					{
						Name:    "disk-wipe",
						Image:   "disk-wipe:v1",
						Timeout: 90,
					},
				},
			},
		},
	}

	payload, err := json.Marshal(wfl)
	if err != nil {
		return nil, err
	}

	return &template.WorkflowTemplate{
		Data: string(payload),
	}, nil
}

func (f *fakeTemplateClient) Create(_ context.Context, _ *template.WorkflowTemplate) error {
	return nil
}

type fakeWorkflowClient struct{}

func (f *fakeWorkflowClient) Create(_ context.Context, _ string, _ string) (string, error) {
	return "", nil
}

type resourceError struct {
	resource string
}

func (re *resourceError) Error() string {
	return fmt.Sprintf("%s %s", re.resource, tinkerbellclient.ErrNotFound.Error())
}
