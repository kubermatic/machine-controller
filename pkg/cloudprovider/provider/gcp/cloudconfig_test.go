//
// Google Cloud Platform Provider for the Machine Controller
//
// Unit Tests
//

package gcp

//-----
// Imports
//-----

import (
	"testing"
)

//-----
// Tests
//-----

func TestCloudConfigAsString(t *testing.T) {
	tests := []struct {
		name     string
		config   *cloudConfig
		contents string
	}{
		{
			name: "minimum test",
			config: &cloudConfig{
				Global: global{
					ProjectID: "my-project-id",
					LocalZone: "my-zone",
				},
			},
			contents: "[global]\n" +
				"project-id = \"my-project-id\"\n" +
				"local-zone = \"my-zone\"\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, err := test.config.asString()
			if err != nil {
				t.Fatalf("failed to convert to string: %v", err)
			}
			if s != test.contents {
				t.Fatalf("output is not as expected")
			}
		})
	}

}
