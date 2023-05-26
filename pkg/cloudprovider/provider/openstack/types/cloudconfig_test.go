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
	"time"

	"gopkg.in/gcfg.v1"

	"github.com/kubermatic/machine-controller/pkg/ini"
	testhelper "github.com/kubermatic/machine-controller/pkg/test"

	"k8s.io/utils/pointer"
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
					AuthURL:     "https://127.0.0.1:8443",
					Username:    "admin",
					Password:    "password",
					DomainName:  "Default",
					ProjectName: "Test",
					Region:      "eu-central1",
				},
				BlockStorage: BlockStorageOpts{
					BSVersion:             "v2",
					IgnoreVolumeAZ:        true,
					TrustDevicePath:       true,
					NodeVolumeAttachLimit: 25,
				},
				LoadBalancer: LoadBalancerOpts{
					ManageSecurityGroups: true,
				},
				Version: "1.10.0",
			},
		},
		{
			name: "use-octavia-explicitly-enabled",
			config: &CloudConfig{
				Global: GlobalOpts{
					AuthURL:     "https://127.0.0.1:8443",
					Username:    "admin",
					Password:    "password",
					DomainName:  "Default",
					ProjectName: "Test",
					Region:      "eu-central1",
				},
				BlockStorage: BlockStorageOpts{
					BSVersion:             "v2",
					IgnoreVolumeAZ:        true,
					TrustDevicePath:       true,
					NodeVolumeAttachLimit: 25,
				},
				LoadBalancer: LoadBalancerOpts{
					ManageSecurityGroups: true,
					UseOctavia:           pointer.Bool(true),
				},
				Version: "1.10.0",
			},
		},
		{
			name: "use-octavia-explicitly-disabled",
			config: &CloudConfig{
				Global: GlobalOpts{
					AuthURL:     "https://127.0.0.1:8443",
					Username:    "admin",
					Password:    "password",
					DomainName:  "Default",
					ProjectName: "Test",
					Region:      "eu-central1",
				},
				BlockStorage: BlockStorageOpts{
					BSVersion:             "v2",
					IgnoreVolumeAZ:        true,
					TrustDevicePath:       true,
					NodeVolumeAttachLimit: 25,
				},
				LoadBalancer: LoadBalancerOpts{
					ManageSecurityGroups: true,
					UseOctavia:           pointer.Bool(false),
				},
				Version: "1.10.0",
			},
		},
		{
			name: "config-with-special-chars",
			config: &CloudConfig{
				Global: GlobalOpts{
					AuthURL:     "https://127.0.0.1:8443",
					Username:    "admin",
					Password:    `.)\^x[tt0L@};p<KJ|f.VQ]7r9u;"ZF|`,
					DomainName:  "Default",
					ProjectName: "Test",
					Region:      "eu-central1",
				},
				BlockStorage: BlockStorageOpts{
					BSVersion:             "v2",
					IgnoreVolumeAZ:        true,
					TrustDevicePath:       true,
					NodeVolumeAttachLimit: 25,
				},
				LoadBalancer: LoadBalancerOpts{
					ManageSecurityGroups: true,
					CreateMonitor:        true,
					FloatingNetworkID:    "ext-net",
					LBMethod:             "",
					LBProvider:           "",
					LBVersion:            "",
					MonitorDelay:         ini.Duration{Duration: 30 * time.Second},
					MonitorMaxRetries:    5,
					MonitorTimeout:       ini.Duration{Duration: 30 * time.Second},
					SubnetID:             "some-subnet-id",
				},
				Version: "1.12.0",
			},
		},
		{
			name: "bs-defaulting-config",
			config: &CloudConfig{
				Global: GlobalOpts{
					AuthURL: "https://127.0.0.1:8443",
				},
				BlockStorage: BlockStorageOpts{},
				LoadBalancer: LoadBalancerOpts{},
				Version:      "1.10.0",
			},
		},
		{
			name: "use-application-credentials",
			config: &CloudConfig{
				Global: GlobalOpts{
					AuthURL:                     "https://127.0.0.1:8443",
					ApplicationCredentialID:     "app-cred-id",
					ApplicationCredentialSecret: "app-cred-secret",
					DomainName:                  "Default",
					Region:                      "eu-central1",
				},
				BlockStorage: BlockStorageOpts{},
				LoadBalancer: LoadBalancerOpts{},
				Version:      "1.10.0",
			},
		},
		{
			name: "use-application-credentials-ignore-userpass",
			config: &CloudConfig{
				Global: GlobalOpts{
					AuthURL:                     "https://127.0.0.1:8443",
					ApplicationCredentialID:     "app-cred-id",
					ApplicationCredentialSecret: "app-cred-secret",
					Username:                    "admin",
					Password:                    "password",
					DomainName:                  "Default",
					Region:                      "eu-central1",
				},
				BlockStorage: BlockStorageOpts{},
				LoadBalancer: LoadBalancerOpts{},
				Version:      "1.10.0",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, err := CloudConfigToString(test.config)
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("Marshaled config: %s\n", s)
			nc := &CloudConfig{}
			if err := gcfg.ReadStringInto(nc, s); err != nil {
				t.Logf("\n%s", s)
				t.Fatalf("failed to load string into config object: %v", err)
			}
			goldenName := test.name + ".golden"
			testhelper.CompareOutput(t, goldenName, s, *update)
		})
	}
}
