package vsphere

import (
	"testing"

	"github.com/go-test/deep"
	"github.com/pmezard/go-difflib/difflib"
	"gopkg.in/gcfg.v1"
)

func TestCloudConfigToString(t *testing.T) {
	tests := []struct {
		name     string
		config   *CloudConfig
		expected string
	}{
		{
			name:     "simple config",
			expected: expectedSimpleConfig,
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
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, err := CloudConfigToString(test.config)
			if err != nil {
				t.Fatal(err)
			}

			nc := &CloudConfig{}
			if err := gcfg.ReadStringInto(nc, s); err != nil {
				t.Fatalf("failed to load string into config object: %v", err)
			}

			ddiff := deep.Equal(test.config, nc)
			if ddiff != nil {
				t.Fatal(ddiff)
			}

			diff := difflib.UnifiedDiff{
				A:        difflib.SplitLines(string(test.expected)),
				B:        difflib.SplitLines(s),
				FromFile: "Expected",
				ToFile:   "Current",
				Context:  3,
			}
			diffStr, err := difflib.GetUnifiedDiffString(diff)
			if err != nil {
				t.Error(err)
			}

			if diffStr != "" {
				t.Errorf("got diff between expected and actual result: \n%s\n", diffStr)
			}
		})
	}
}

var (
	expectedSimpleConfig = `[Global]
user          = admin
password      = password
port          = 
insecure-flag = true

[Disk]
scsicontrollertype = pvscsi

[Workspace]
server            = https://127.0.0.1:8443
datacenter        = Datacenter
folder            = some-folder
default-datastore = Datastore
resourcepool-path = /some-resource-pool

`
)
