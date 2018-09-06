package openstack

import (
	"github.com/go-test/deep"
	"github.com/pmezard/go-difflib/difflib"
	"gopkg.in/gcfg.v1"
	"testing"
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
			},
		},
		{
			name:     "config with special chars",
			expected: expectedSpecialCharactersConfig,
			config: &CloudConfig{
				Global: GlobalOpts{
					AuthURL:    "https://127.0.0.1:8443",
					Username:   "admin",
					Password:   "\"f;o'ob\\a`r=#",
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
auth-url    = https://127.0.0.1:8443
username    = admin
password    = password
tenant-name = Test
domain-name = Default
region      = eu-central1

[LoadBalancer]
manage-security-groups = true

[BlockStorage]
ignore-volume-az  = true
trust-device-path = true
bs-version        = v2

`
	expectedSpecialCharactersConfig = `[Global]
auth-url    = https://127.0.0.1:8443
username    = admin
password    = """\"f;o'ob\\a` + "`" + `r=#"""
tenant-name = Test
domain-name = Default
region      = eu-central1

[LoadBalancer]
manage-security-groups = true

[BlockStorage]
ignore-volume-az  = true
trust-device-path = true
bs-version        = v2

`
)
