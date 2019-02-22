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
	"strings"
	"testing"
)

//-----
// Tests
//-----

func TestCloudConfigAsString(t *testing.T) {
	tests := []struct {
		name     string
		config   *cloudConfig
		contents []string
	}{
		{
			name: "minimum test",
			config: &cloudConfig{
				Global: global{
					ProjectID: "my-project-id",
					LocalZone: "my-zone",
				},
			},
			contents: []string{
				`project-id = "my-project-id"`,
				`local-zone = "my-zone"`,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, err := test.config.asString()
			if err != nil {
				t.Fatalf("failed to convert to string: %v", err)
			}
			for _, c := range test.contents {
				if !strings.Contains(s, c) {
					t.Fatalf("output does not contain %q, instead %s", c, s)
				}
			}
		})
	}

}
