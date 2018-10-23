package vsphere

import (
	"flag"
	"testing"

	"gopkg.in/gcfg.v1"

	testhelper "github.com/kubermatic/machine-controller/pkg/test"
)

var update = flag.Bool("update", false, "update .golden files")

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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, err := CloudConfigToString(test.config)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("\n%s", s)

			nc := &CloudConfig{}
			if err := gcfg.ReadStringInto(nc, s); err != nil {
				t.Fatalf("failed to load string into config object: %v", err)
			}

			testhelper.CompareOutput(t, test.name, s, *update)
		})
	}
}
