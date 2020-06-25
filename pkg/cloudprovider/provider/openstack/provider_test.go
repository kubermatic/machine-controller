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

	cloudprovidertesting "github.com/kubermatic/machine-controller/pkg/cloudprovider/testing"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"

	"k8s.io/utils/pointer"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	th "github.com/gophercloud/gophercloud/testhelper"
	"github.com/gophercloud/gophercloud/testhelper/client"
)

const expectedServerRequest = `{
  "server": {
	  "availability_zone": "eu-de-01",
	  "flavorRef": "1",
	  "imageRef": "f3e4a95d-1f4f-4989-97ce-f3a1fb8c04d7",
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
		"uuid": "f3e4a95d-1f4f-4989-97ce-f3a1fb8c04d7",
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

type openstackProviderSpecConf struct {
	IdentityEndpointURL string
	RootDiskSizeGB      *int32
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
		"password": "this_is_a_password",
		"region": "eu-de",
		{{- if .RootDiskSizeGB }}
		"rootDiskSizeGB": {{ .RootDiskSizeGB }},
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
		"tenantID": "",
		"tenantName": "eu-de",
		"tokenId": "",
		"trustDevicePath": false,
		"username": "dummy"
	},
	"operatingSystem": "coreos",
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th.SetupHTTP()
			defer th.TeardownHTTP()
			ExpectServerCreated(t, tt.wantServerReq)
			p := &provider{
				// Note that configVarResolver is not used in this test as the getConfigFunc is mocked.
				configVarResolver: providerconfig.NewConfigVarResolver(context.Background(), fakeclient.NewFakeClient()),
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
				serverReadinessWaiter: func(computeClient *gophercloud.ServiceClient, serverID string) error {
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
			_, err := p.Create(m, tt.data, tt.userdata)
			if (err != nil) != tt.wantErr {
				t.Errorf("provider.Create() or = %v, wantErr %v", err, tt.wantErr)
				return
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
	res.Server.ID = "f3e4a95d-1f4f-4989-97ce-f3a1fb8c04d7"
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
		fmt.Fprintf(w, string(srvRes))
	})
	th.Mux.HandleFunc("/os-volumes_boot", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "POST")
		th.TestHeader(t, r, "X-Auth-Token", client.TokenID)
		th.TestJSONRequest(t, r, expectedServer)

		w.WriteHeader(http.StatusAccepted)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintf(w, string(srvRes))
	})

	// Handle listing images.
	th.Mux.HandleFunc("/images/detail", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", client.TokenID)

		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintf(w, `
			{
				"images": [
					{
						"status": "ACTIVE",
						"updated": "2014-09-23T12:54:56Z",
						"id": "f3e4a95d-1f4f-4989-97ce-f3a1fb8c04d7",
						"OS-EXT-IMG-SIZE:size": 476704768,
						"name": "F17-x86_64-cfntools",
						"created": "2014-09-23T12:54:52Z",
						"minDisk": 0,
						"progress": 100,
						"minRam": 0
					},
					{
						"status": "ACTIVE",
						"updated": "2014-09-23T12:51:43Z",
						"id": "f90f6034-2570-4974-8351-6b49732ef2eb",
						"OS-EXT-IMG-SIZE:size": 13167616,
						"name": "cirros-0.3.2-x86_64-disk",
						"created": "2014-09-23T12:51:42Z",
						"minDisk": 0,
						"progress": 100,
						"minRam": 0
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
