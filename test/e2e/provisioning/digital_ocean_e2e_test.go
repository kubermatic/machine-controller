// +build e2e

package provisioning

import (
	"fmt"
	"os"
	"testing"
)

const (
	do_manifest = "./testdata/machine-digitalocean.yaml"
)

// TestDigitalOceanProvisioning - a test suite that exercises digital ocean provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
//
// note that tests require  a valid API Token that is read from the DO_E2E_TEST_TOKEN environmental variable.
func TestDigitalOceanProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	doToken := os.Getenv("DO_E2E_TESTS_TOKEN")
	if len(doToken) == 0 {
		t.Fatal("unable to run the test suite, DO_E2E_TESTS_TOKEN environement varialbe cannot be empty")
	}

	// act
	params := fmt.Sprintf("<< DIGITALOCEAN_TOKEN >>=%s", doToken)
	runScenarios(t, scenarios, params, do_manifest, "do")
}
