// +build e2e

package provisioning

import (
	"fmt"
	"os"
	"testing"
)

const (
	do_manifest  = "./testdata/machine-digitalocean.yaml"
	aws_manifest = "./testdata/machine-aws.yaml"
	hz_manifest  = "./testdata/machine-hetzner.yaml"
	vs_manifest  = "./testdata/machine-vsphere.yaml"
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

var awsScenarios = []scenario{
	scenarios[0], /* scenario 1 Ubuntu Docker 1.13 */
	scenarios[1], /* scenario 2 Ubuntu Docker 17.03 */
	scenarios[2], /* scenario 3 Ubuntu CRI-O 1.9 */
	scenarios[5], /* scenario 6 CoreOS Docker 1.13 */
	scenarios[6], /* scenario 7 CoreOS Docker 17.03 */
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
	params := fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s,<< AWS_SECRET_ACCESS_KEY >>=%s", awsKeyID, awsSecret)
	runScenarios(t, awsScenarios, params, aws_manifest, "aws")
}

var hzScenarios = []scenario{
	scenarios[0], /* scenario 1 Ubuntu Docker 1.13 */
	scenarios[1], /* scenario 2 Ubuntu Docker 17.03 */
	scenarios[2], /* scenario 3 Ubuntu CRI-O 1.9 */
	scenarios[3], /* scenario 4 CentOS Docker 1.12 */
	scenarios[4], /* scenario 5 CentOS Docker 1.13 */
}

// TestHetznerProvisioning - a test suite that exercises Hetzner provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestHetznerProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	hzToken := os.Getenv("HZ_E2E_TOKEN")
	if len(hzToken) == 0 {
		t.Fatal("unable to run the test suite, HZ_E2E_TOKEN environment variable cannot be empty")
	}

	// act
	params := fmt.Sprintf("<< HETZNER_TOKEN >>=%s", hzToken)
	runScenarios(t, hzScenarios, params, hz_manifest, "hz")
}

var vsphereScenarios = []scenario{
	scenarios[0], /* scenario 1 Ubuntu Docker 1.13 */
	scenarios[1], /* scenario 2 Ubuntu Docker 17.03 */
	scenarios[2], /* scenario 3 Ubuntu CRI-O 1.9 */
}

// TestVsphereProvisioning - a test suite that exercises vsphere provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestVsphereProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	vsPassword := os.Getenv("VSPHERE_E2E_PASSWORD")
	vsUsername := os.Getenv("VSPHERE_E2E_USERNAME")
	vsAddress := os.Getenv("VSPHERE_E2E_ADDRESS")
	if len(vsPassword) == 0 || len(vsUsername) == 0 || len(vsAddress) == 0 {
		t.Fatal("unable to run the test suite, VSPHERE_E2E_PASSWORD, VSPHERE_E2E_USERNAME or VSPHERE_E2E_ADDRESS environment variables cannot be empty")
	}

	// act
	params := fmt.Sprintf("<< VSPHERE_PASSWORD >>=%s,<< VSPHERE_USERNAME >>=%s,<< VSPHERE_ADDRESS >>=%s", vsPassword, vsUsername, vsAddress)
	runScenarios(t, vsphereScenarios, params, vs_manifest, "vs")
}
