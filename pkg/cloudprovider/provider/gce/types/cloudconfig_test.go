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

//
// Google Cloud Provider for the Machine Controller
//
// Unit Tests
//

package types

import (
	"testing"
)

func TestCloudConfigAsString(t *testing.T) {
	tests := []struct {
		name     string
		config   *CloudConfig
		contents string
	}{
		{
			name: "minimum test",
			config: &CloudConfig{
				Global: GlobalOpts{
					ProjectID:      "my-project-id",
					LocalZone:      "my-zone",
					NetworkName:    "my-cool-network",
					SubnetworkName: "my-cool-subnetwork",
					TokenURL:       "nil",
					MultiZone:      true,
					Regional:       true,
					NodeTags:       []string{"tag1", "tag2"},
				},
			},
			contents: "[global]\n" +
				"project-id = \"my-project-id\"\n" +
				"local-zone = \"my-zone\"\n" +
				"network-name = \"my-cool-network\"\n" +
				"subnetwork-name = \"my-cool-subnetwork\"\n" +
				"token-url = \"nil\"\n" +
				"multizone = true\n" +
				"regional = true\n" +
				"node-tags = \"tag1\"\n" +
				"node-tags = \"tag2\"\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, err := test.config.AsString()
			if err != nil {
				t.Fatalf("failed to convert to string: %v", err)
			}
			if s != test.contents {
				t.Fatalf("output is not as expected: %s", s)
			}
		})
	}
}
