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

package openstack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"text/template"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	th "github.com/gophercloud/gophercloud/testhelper"
	"github.com/gophercloud/gophercloud/testhelper/client"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidertesting "github.com/kubermatic/machine-controller/pkg/cloudprovider/testing"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const expectedServerRequest = `{
  "server": {
	  "availability_zone": "eu-de-01",
	  "flavorRef": "1",
	  "imageRef": "1bea47ed-f6a9-463b-b423-14b9cca9ad27",
	  "metadata": {
		"kubernetes-cluster": "xyz",
		"machine-uid": "",
		"system-cluster": "zyx",
		"system-project": "xxx"
	  },
	  "name": "test",
	  "networks": [
		{
		  "uuid": "d32019d3-bc6e-4319-9c1d-6722fc136a22"
		}
	  ],
	  "security_groups": [
		{
		  "name": "kubernetes-xyz"
		}
	  ],
	  "user_data": "ZmFrZS11c2VyZGF0YQ=="
  }
}
`

const expectedBlockDeviceBootRequest = `{
  "server": {
	"availability_zone": "eu-de-01",
	"block_device_mapping_v2": [
	  {
		"boot_index": 0,
		"delete_on_termination": true,
		"destination_type": "volume",
		"source_type": "image",
		"uuid": "1bea47ed-f6a9-463b-b423-14b9cca9ad27",
		"volume_size": 10
	  }
	],
	"flavorRef": "1",
	"imageRef": "",
	"metadata": {
	  "kubernetes-cluster": "xyz",
	  "machine-uid": "",
	  "system-cluster": "zyx",
	  "system-project": "xxx"
	},
	"name": "test",
	"networks": [
	  {
		"uuid": "d32019d3-bc6e-4319-9c1d-6722fc136a22"
	  }
	],
	"security_groups": [
	  {
		"name": "kubernetes-xyz"
	  }
	],
	"user_data": "ZmFrZS11c2VyZGF0YQ=="
  }
}`

const expectedBlockDeviceBootVolumeTypeRequest = `{
	"server": {
	  "availability_zone": "eu-de-01",
	  "block_device_mapping_v2": [
		{
		  "boot_index": 0,
		  "delete_on_termination": true,
		  "destination_type": "volume",
		  "source_type": "image",
		  "uuid": "1bea47ed-f6a9-463b-b423-14b9cca9ad27",
		  "volume_size": 10,
		  "volume_type": "ssd"
		}
	  ],
	  "flavorRef": "1",
	  "imageRef": "",
	  "metadata": {
		"kubernetes-cluster": "xyz",
		"machine-uid": "",
		"system-cluster": "zyx",
		"system-project": "xxx"
	  },
	  "name": "test",
	  "networks": [
		{
		  "uuid": "d32019d3-bc6e-4319-9c1d-6722fc136a22"
		}
	  ],
	  "security_groups": [
		{
		  "name": "kubernetes-xyz"
		}
	  ],
	  "user_data": "ZmFrZS11c2VyZGF0YQ=="
	}
  }`

type openstackProviderSpecConf struct {
	IdentityEndpointURL         string
	RootDiskSizeGB              *int32
	RootDiskVolumeType          string
	ApplicationCredentialID     string
	ApplicationCredentialSecret string
	ProjectName                 string
	ProjectID                   string
	TenantID                    string
	TenantName                  string
	ComputeAPIVersion           string
}

func (o openstackProviderSpecConf) rawProviderSpec(t *testing.T) []byte {
	var out bytes.Buffer
	tmpl, err := template.New("test").Parse(`{
	"cloudProvider": "openstack",
	"cloudProviderSpec": {
		"availabilityZone": "eu-de-01",
		"domainName": "openstack_domain_name",
		"flavor": "m1.tiny",
		"identityEndpoint": "{{ .IdentityEndpointURL }}",
		"image": "Standard_Ubuntu_18.04_latest",
		"network": "public",
		"nodeVolumeAttachLimit": null,
		"region": "eu-de",
		"instanceReadyCheckPeriod": "2m",
		"instanceReadyCheckTimeout": "2m",
		{{- if .ComputeAPIVersion }}
		"computeAPIVersion": {{ .ComputeAPIVersion }},
		{{- end }}
		{{- if .RootDiskSizeGB }}
		"rootDiskSizeGB": {{ .RootDiskSizeGB }},
		{{- end }}
		{{- if .RootDiskVolumeType }}
		"rootDiskVolumeType": "{{ .RootDiskVolumeType }}",
		{{- end }}
		"securityGroups": [
			"kubernetes-xyz"
		],
		"subnet": "subnetid",
		"tags": {
			"kubernetes-cluster": "xyz",
			"system-cluster": "zyx",
			"system-project": "xxx"
		},
		{{- if .ApplicationCredentialID }}
		"applicationCredentialID": "{{ .ApplicationCredentialID }}",
		"applicationCredentialSecret": "{{ .ApplicationCredentialSecret }}",
		{{- else }}
		{{ if .ProjectID }}
		"projectID": "{{ .ProjectID }}",
		"projectName": "{{ .ProjectName }}",
        {{- end }}
        {{- if .TenantID }}
		"tenantID": "{{ .TenantID }}",
		"tenantName": "{{ .TenantName }}",
        {{- end }}
		"username": "dummy",
		"password": "this_is_a_password",
		{{- end }}
		"tokenId": "",
		"trustDevicePath": false
	},
	"operatingSystem": "flatcar",
	"operatingSystemSpec": {
		"disableAutoUpdate": false,
		"disableLocksmithD": true,
		"disableUpdateEngine": false
	}
}`)
	if err != nil {
		t.Fatalf("Error occurred while parsing openstack provider spec template: %v", err)
	}
	err = tmpl.Execute(&out, o)
	if err != nil {
		t.Fatalf("Error occurred while executing openstack provider spec template: %v", err)
	}
	t.Logf("Generated providerSpec: %s", out.String())
	return out.Bytes()
}

func TestCreateServer(t *testing.T) {
	tests := []struct {
		name          string
		specConf      openstackProviderSpecConf
		data          *cloudprovidertypes.ProviderData
		userdata      string
		wantServerReq string
		wantErr       bool
	}{
		{
			name:          "Nominal case",
			specConf:      openstackProviderSpecConf{},
			userdata:      "fake-userdata",
			wantServerReq: expectedServerRequest,
		},
		{
			name:          "Custom disk size",
			specConf:      openstackProviderSpecConf{RootDiskSizeGB: pointer.Int32Ptr(10)},
			userdata:      "fake-userdata",
			wantServerReq: expectedBlockDeviceBootRequest,
		},
		{
			name:          "Custom disk type",
			specConf:      openstackProviderSpecConf{RootDiskSizeGB: pointer.Int32Ptr(10), RootDiskVolumeType: "ssd"},
			userdata:      "fake-userdata",
			wantServerReq: expectedBlockDeviceBootVolumeTypeRequest,
		},
		{
			name:          "Application Credentials",
			specConf:      openstackProviderSpecConf{ApplicationCredentialID: "app-cred-id", ApplicationCredentialSecret: "app-cred-secret"},
			userdata:      "fake-userdata",
			wantServerReq: expectedServerRequest,
		},
		{
			name:          "Compute API Version",
			specConf:      openstackProviderSpecConf{ComputeAPIVersion: "2.67"},
			userdata:      "fake-userdata",
			wantServerReq: expectedServerRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th.SetupHTTP()
			defer th.TeardownHTTP()
			ExpectServerCreated(t, tt.wantServerReq)
			p := &provider{
				// Note that configVarResolver is not used in this test as the getConfigFunc is mocked.
				configVarResolver: providerconfig.NewConfigVarResolver(context.Background(), fakectrlruntimeclient.NewClientBuilder().Build()),
				// mock client config getter
				clientGetter: func(c *Config) (*gophercloud.ProviderClient, error) {
					pc := client.ServiceClient()
					// endpoint locator used to redirect to local test endpoint
					pc.ProviderClient.EndpointLocator = func(_ gophercloud.EndpointOpts) (string, error) {
						return pc.Endpoint, nil
					}
					return pc.ProviderClient, nil
				},
				// mock server readiness checker
				portReadinessWaiter: func(*gophercloud.ServiceClient, string, string, time.Duration, time.Duration) error {
					return nil
				},
			}
			// Use the endpoint of the local server simulating OpenStack API
			tt.specConf.IdentityEndpointURL = th.Endpoint()
			m := cloudprovidertesting.Creator{
				Name:               "test",
				Namespace:          "openstack",
				ProviderSpecGetter: tt.specConf.rawProviderSpec,
			}.CreateMachine(t)
			// It only verifies that the content of the create request matches
			// the expectation
			// TODO(irozzo) check the returned instance too
			_, err := p.Create(context.Background(), m, tt.data, tt.userdata)
			if (err != nil) != tt.wantErr {
				t.Errorf("provider.Create() or = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestProjectAuthVarsAreCorrectlyLoaded(t *testing.T) {
	tests := []struct {
		name         string
		expectedName string
		expectedID   string
		specConf     openstackProviderSpecConf
	}{
		{
			name:         "Project auth vars should be when tenant vars are not defined",
			expectedID:   "the_project_id",
			expectedName: "the_project_name",
			specConf:     openstackProviderSpecConf{ProjectID: "the_project_id", ProjectName: "the_project_name"},
		},
		{
			name:         "Project auth vars should be used even if tenant vars are defined",
			expectedID:   "the_project_id",
			expectedName: "the_project_name",
			specConf:     openstackProviderSpecConf{ProjectID: "the_project_id", ProjectName: "the_project_name", TenantID: "the_tenant_id", TenantName: "the_tenant_name"},
		},
		{
			name:         "Tenant auth vars should be used when project vars are not defined",
			expectedID:   "the_tenant_id",
			expectedName: "the_tenant_name",
			specConf:     openstackProviderSpecConf{TenantID: "the_tenant_id", TenantName: "the_tenant_name"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &provider{
				// Note that configVarResolver is not used in this test as the getConfigFunc is mocked.
				configVarResolver: providerconfig.NewConfigVarResolver(context.Background(), fakectrlruntimeclient.
					NewClientBuilder().
					Build()),
			}
			conf, _, _, _ := p.getConfig(v1alpha1.ProviderSpec{
				Value: &runtime.RawExtension{
					Raw: tt.specConf.rawProviderSpec(t),
				},
			})

			if conf.ProjectID != tt.expectedID {
				t.Errorf("ProjectID = %v, wanted %v", conf.ProjectID, tt.expectedID)
			}
			if conf.ProjectName != tt.expectedName {
				t.Errorf("ProjectName = %v, wanted %v", conf.ProjectName, tt.expectedName)
			}
		})
	}
}

type ServerResponse struct {
	Server servers.Server `json:"server"`
}

// ExpectServerCreated is used to verify that the manifest used to create the
// server matches the expectation.
// It also provides all the the handlers required to mock the Openstack APIs
// used during the server creation.
func ExpectServerCreated(t *testing.T, expectedServer string) {
	// Prepare the server manifest
	var res ServerResponse
	// This is just a hacky way of getting the values matching from the
	// expectedServer copied into the response (e.g. name).
	err := json.Unmarshal([]byte(expectedServer), &res)
	if err != nil {
		t.Fatalf("Error occurred while unmarshaling the expected server manifest.")
	}
	res.Server.ID = "1bea47ed-f6a9-463b-b423-14b9cca9ad27"
	srvRes, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("Error occurred while marshaling the server response manifest.")
	}
	t.Logf("Server response: %s", srvRes)
	// Handle server creation requests.
	th.Mux.HandleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "POST")
		th.TestHeader(t, r, "X-Auth-Token", client.TokenID)
		th.TestJSONRequest(t, r, expectedServer)

		w.WriteHeader(http.StatusAccepted)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, string(srvRes))
	})
	th.Mux.HandleFunc("/os-volumes_boot", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "POST")
		th.TestHeader(t, r, "X-Auth-Token", client.TokenID)
		th.TestJSONRequest(t, r, expectedServer)

		w.WriteHeader(http.StatusAccepted)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprint(w, string(srvRes))
	})

	// Handle listing images v2.
	th.Mux.HandleFunc("/v2/images", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", client.TokenID)

		w.Header().Add("Content-Type", "application/json")
		// Example ref: https://docs.openstack.org/api-ref/image/v2/index.html?expanded=list-images-detail#list-images
		fmt.Fprintf(w, `
			{
				"images": [
					{
						"status": "active",
						"name": "cirros-0.3.2-x86_64-disk",
						"tags": [],
						"container_format": "bare",
						"created_at": "2014-11-07T17:07:06Z",
						"disk_format": "qcow2",
						"updated_at": "2014-11-07T17:19:09Z",
						"visibility": "public",
						"self": "/v2/images/1bea47ed-f6a9-463b-b423-14b9cca9ad27",
						"min_disk": 0,
						"protected": false,
						"id": "1bea47ed-f6a9-463b-b423-14b9cca9ad27",
						"file": "/v2/images/1bea47ed-f6a9-463b-b423-14b9cca9ad27/file",
						"checksum": "64d7c1cd2b6f60c92c14662941cb7913",
						"os_hash_algo": "sha512",
						"os_hash_value": "073b4523583784fbe01daff81eba092a262ec37ba6d04dd3f52e4cd5c93eb8258af44881345ecda0e49f3d8cc6d2df6b050ff3e72681d723234aff9d17d0cf09",
						"os_hidden": false,
						"owner": "5ef70662f8b34079a6eddb8da9d75fe8",
						"size": 13167616,
						"min_ram": 0,
						"schema": "/v2/schemas/image",
						"virtual_size": null
					},
					{
						"status": "active",
						"name": "F17-x86_64-cfntools",
						"tags": [],
						"container_format": "bare",
						"created_at": "2014-10-30T08:23:39Z",
						"disk_format": "qcow2",
						"updated_at": "2014-11-03T16:40:10Z",
						"visibility": "public",
						"self": "/v2/images/781b3762-9469-4cec-b58d-3349e5de4e9c",
						"min_disk": 0,
						"protected": false,
						"id": "781b3762-9469-4cec-b58d-3349e5de4e9c",
						"file": "/v2/images/781b3762-9469-4cec-b58d-3349e5de4e9c/file",
						"checksum": "afab0f79bac770d61d24b4d0560b5f70",
						"os_hash_algo": "sha512",
						"os_hash_value": "ea3e20140df1cc65f53d4c5b9ee3b38d0d6868f61bbe2230417b0f98cef0e0c7c37f0ebc5c6456fa47f013de48b452617d56c15fdba25e100379bd0e81ee15ec",
						"os_hidden": false,
						"owner": "5ef70662f8b34079a6eddb8da9d75fe8",
						"size": 476704768,
						"min_ram": 0,
						"schema": "/v2/schemas/image",
						"virtual_size": null
					}
				]
			}
		`)
	})

	// Handle listing flavours.
	th.Mux.HandleFunc("/flavors/detail", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", client.TokenID)

		w.Header().Add("Content-Type", "application/json")
		err := r.ParseForm()
		if err != nil {
			t.Fatalf("Error occurred while parsing form: %v", err)
		}
		marker := r.Form.Get("marker")
		switch marker {
		case "":
			fmt.Fprintf(w, `
				{
					"flavors": [
						{
							"id": "1",
							"name": "m1.tiny",
							"disk": 1,
							"ram": 512,
							"vcpus": 1,
							"swap":""
						},
						{
							"id": "2",
							"name": "m2.small",
							"disk": 10,
							"ram": 1024,
							"vcpus": 2,
							"swap": 1000
						}
					],
					"flavors_links": [
						{
							"href": "%s/flavors/detail?marker=2",
							"rel": "next"
						}
					]
				}
			`, th.Server.URL)
		case "2":
			fmt.Fprintf(w, `{ "flavors": [] }`)
		default:
			t.Fatalf("Unexpected marker: [%s]", marker)
		}
	})
	// Handle listing networks.
	th.Mux.HandleFunc("/v2.0/networks", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", client.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, `
			{
				"networks": [
					{
						"status": "ACTIVE",
						"subnets": [
							"54d6f61d-db07-451c-9ab3-b9609b6b6f0b"
						],
						"name": "public",
						"admin_state_up": true,
						"tenant_id": "4fd44f30292945e481c7b8a0c8908869",
						"shared": true,
						"id": "d32019d3-bc6e-4319-9c1d-6722fc136a22",
						"provider:segmentation_id": 9876543210,
						"provider:physical_network": null,
						"provider:network_type": "local",
						"router:external": true,
						"port_security_enabled": true,
						"dns_domain": "local.",
						"mtu": 1500
					}
				]
			}`)
	})
}
