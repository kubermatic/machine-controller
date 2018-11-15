package aws

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
					Zone:                        "some-zone",
					VPC:                         "some-vpc",
					SubnetID:                    "some-subnet",
					KubernetesClusterID:         "some-tag",
					DisableSecurityGroupIngress: true,
					DisableStrictZoneCheck:      true,
					ElbSecurityGroup:            "some-sg",
					KubernetesClusterTag:        "some-tag",
					RoleARN:                     "some-arn",
					RouteTableID:                "some-rt",
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
