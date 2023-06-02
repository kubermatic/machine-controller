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
					NodeIPFamilies:              []string{"ipv4", "ipv6"},
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
			goldenName := test.name + ".golden"
			testhelper.CompareOutput(t, goldenName, s, *update)
		})
	}
}
