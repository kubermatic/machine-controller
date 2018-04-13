// +build e2e

package provisioning

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	aws_manifest = "./testdata/machine-aws.yaml"
)

var awsScenarios = []scenarios{
	{
		name:                    "scenario 1 Ubuntu Docker 1.13",
		osName:                  "ubuntu",
		containerRuntime:        "docker",
		containerRuntimeVersion: "1.13",
		short: true,
	},

	{
		name:                    "scenario 2 Ubuntu Docker 17.03",
		osName:                  "ubuntu",
		containerRuntime:        "docker",
		containerRuntimeVersion: "17.03",
	},

	{
		name:                    "scenario 3 Ubuntu CRI-O 1.9",
		osName:                  "ubuntu",
		containerRuntime:        "cri-o",
		containerRuntimeVersion: "1.9",
	},

	{
		name:                    "scenario 4 CoreOS Docker 1.13",
		osName:                  "coreos",
		containerRuntime:        "docker",
		containerRuntimeVersion: "1.13",
	},

	{
		name:                    "scenario 5 CoreOS Docker 17.03",
		osName:                  "coreos",
		containerRuntime:        "docker",
		containerRuntimeVersion: "17.03",
	},
}

// TestAWSProvisioning - a test suite that exercises AWS provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestAWSProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	awsKeyID := os.Getenv("AWS_E2E_TESTS_KEY_ID")
	awsSecret := os.Getenv("AWS_E2E_TESTS_SECRET")
	if len(awsKeyID) == 0 || len(awsSecret) == 0 {
		t.Fatal("unable to run the test suite, AWS_E2E_TESTS_KEY_ID or AWS_E2E_TESTS_SECRET environment variables cannot be empty")
	}

	// act
	for _, scenario := range awsScenarios {
		scenario := scenario // capture range variable
		t.Run(scenario.name, func(t *testing.T) {
			if testing.Short() == true && scenario.short == false {
				t.SkipNow()
			}
			t.Parallel()

			nameSufix := strings.Replace(scenario.name, " ", "-", -1)
			nameSufix = strings.ToLower(nameSufix)
			nameSufix = fmt.Sprintf("aws-%s", nameSufix)
			machineName := fmt.Sprintf("machine-%s", nameSufix)
			nodeName := fmt.Sprintf("node-%s", nameSufix)
			params := fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s,<< AWS_SECRET_ACCESS_KEY >>=%s", awsKeyID, awsSecret)
			params = fmt.Sprintf("%s,<< MACHINE_NAME >>=%s,<< NODE_NAME >>=%s", params, machineName, nodeName)
			params = fmt.Sprintf("%s,<< OS_NAME >>=%s,<< CONTAINER_RUNTIME >>=%s,<< CONTAINER_RUNTIME_VERSION >>=%s", params, scenario.osName, scenario.containerRuntime, scenario.containerRuntimeVersion)

			err := runVerifyTool(aws_manifest, params)
			if err != nil {
				t.Errorf("verify tool failed due to %v", err)
			}
		})
	}
}
