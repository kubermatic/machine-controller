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

package types

import (
	"flag"
	"testing"

	"gopkg.in/gcfg.v1"

	testhelper "github.com/kubermatic/machine-controller/pkg/test"
)

var update = flag.Bool("update", false, "update testdata files")

func TestCloudConfigToString(t *testing.T) {
	tests := []struct {
		name   string
		config *CloudConfig
	}{
		{
			name: "simple-config",
			config: &CloudConfig{
				Global: GlobalOpts{
					User:         "admin",
					Password:     "password",
					InsecureFlag: true,
				},
				Workspace: WorkspaceOpts{
					VCenterIP:        "https://127.0.0.1:8443",
					ResourcePoolPath: "/some-resource-pool",
					DefaultDatastore: "Datastore",
					Folder:           "some-folder",
					Datacenter:       "Datacenter",
				},
				Disk: DiskOpts{
					SCSIControllerType: "pvscsi",
				},
				VirtualCenter: map[string]*VirtualCenterConfig{},
			},
		},
		{
			name: "2-virtual-centers",
			config: &CloudConfig{
				Global: GlobalOpts{
					User:         "admin",
					Password:     "password",
					InsecureFlag: true,
				},
				Workspace: WorkspaceOpts{
					VCenterIP:        "https://127.0.0.1:8443",
					ResourcePoolPath: "/some-resource-pool",
					DefaultDatastore: "Datastore",
					Folder:           "some-folder",
					Datacenter:       "Datacenter",
				},
				Disk: DiskOpts{
					SCSIControllerType: "pvscsi",
				},
				VirtualCenter: map[string]*VirtualCenterConfig{
					"vc1": {
						User:        "1-some-user",
						Password:    "1-some-password",
						VCenterPort: "443",
						Datacenters: "1-foo",
					},
					"vc2": {
						User:        "2-some-user",
						Password:    "2-some-password",
						VCenterPort: "443",
						Datacenters: "2-foo",
					},
				},
			},
		},
		{
			name: "3-dual-stack",
			config: &CloudConfig{
				Global: GlobalOpts{
					User:         "admin",
					Password:     "password",
					InsecureFlag: true,
					IPFamily:     "ipv4,ipv6",
				},
				Workspace: WorkspaceOpts{
					VCenterIP:        "https://127.0.0.1:8443",
					ResourcePoolPath: "/some-resource-pool",
					DefaultDatastore: "Datastore",
					Folder:           "some-folder",
					Datacenter:       "Datacenter",
				},
				Disk: DiskOpts{
					SCSIControllerType: "pvscsi",
				},
				VirtualCenter: map[string]*VirtualCenterConfig{
					"vc1": {
						User:        "1-some-user",
						Password:    "1-some-password",
						VCenterPort: "443",
						Datacenters: "1-foo",
						IPFamily:    "ipv4,ipv6",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, err := test.config.String()
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("\n%s", s)

			nc := &CloudConfig{}
			if err := gcfg.ReadStringInto(nc, s); err != nil {
				t.Fatalf("failed to load string into config object: %v", err)
			}
			goldenName := test.name + ".golden"
			testhelper.CompareOutput(t, goldenName, s, *update)
		})
	}
}
