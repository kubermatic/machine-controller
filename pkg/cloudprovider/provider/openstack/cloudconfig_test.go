package openstack

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
					AuthURL:    "https://127.0.0.1:8443",
					Username:   "admin",
					Password:   "password",
					DomainName: "Default",
					TenantName: "Test",
					Region:     "eu-central1",
				},
				BlockStorage: BlockStorageOpts{
					BSVersion:       "v2",
					IgnoreVolumeAZ:  true,
					TrustDevicePath: true,
				},
				LoadBalancer: LoadBalancerOpts{
					ManageSecurityGroups: true,
				},
				Version: "1.10.0",
			},
		},
		{
			name: "config-with-special-chars",
			config: &CloudConfig{
				Global: GlobalOpts{
					AuthURL:    "https://127.0.0.1:8443",
					Username:   "admin",
					Password:   `.)\^x[tt0L@};p<KJ|f.VQ]7r9u;"ZF|`,
					DomainName: "Default",
					TenantName: "Test",
					Region:     "eu-central1",
				},
				BlockStorage: BlockStorageOpts{
					BSVersion:       "v2",
					IgnoreVolumeAZ:  true,
					TrustDevicePath: true,
				},
				LoadBalancer: LoadBalancerOpts{
					ManageSecurityGroups: true,
				},
				Version: "1.12.0",
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
				t.Logf("\n%s", s)
				t.Fatalf("failed to load string into config object: %v", err)
			}

			testhelper.CompareOutput(t, test.name, s, *update)
		})
	}
}
