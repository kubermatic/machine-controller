// +build e2e

package provisioning

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	do_manifest = "./testdata/machine-digitalocean.yaml"
)

var digitalOceanScenarios = []scenarios{
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

// TestDigitalOceanProvisioning - a test suite that exercises digital ocean provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
//
// note that tests require  a valid API Token that is read from the DO_E2E_TEST_TOKEN environmental variable.
func TestDigitalOceanProvisioningE2E(t *testing.T) {
	if testing.Short() {
		t.Parallel()
	}

	// test data
	doToken := os.Getenv("DO_E2E_TESTS_TOKEN")
	if len(doToken) == 0 {
		t.Fatal("unable to run the test suite, DO_E2E_TESTS_TOKEN environement varialbe cannot be empty")
	}

	// act
	for _, scenario := range digitalOceanScenarios {
		scenario := scenario // capture range variable
		t.Run(scenario.name, func(t *testing.T) {
			if testing.Short() == true && scenario.short == false {
				t.SkipNow()
			}
			t.Parallel()

			nameSufix := strings.Replace(scenario.name, " ", "-", -1)
			nameSufix = strings.ToLower(nameSufix)
			nameSufix = fmt.Sprintf("do-%s", nameSufix)
			machineName := fmt.Sprintf("machine-%s", nameSufix)
			nodeName := fmt.Sprintf("node-%s", nameSufix)
			params := fmt.Sprintf("<< MACHINE_NAME >>=%s,<< DIGITALOCEAN_TOKEN >>=%s,<< NODE_NAME >>=%s", machineName, doToken, nodeName)
			params = fmt.Sprintf("%s,<< OS_NAME >>=%s,<< CONTAINER_RUNTIME >>=%s,<< CONTAINER_RUNTIME_VERSION >>=%s", params, scenario.osName, scenario.containerRuntime, scenario.containerRuntimeVersion)

			err := runVerifyTool(do_manifest, params)
			if err != nil {
				t.Errorf("verify tool failed due to %v", err)
			}
		})
	}
}
