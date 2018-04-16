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
	runScenarios(t, scenarios, params, aws_manifest, "aws")
}

var hzScenarios = []scenario{
	scenarios[0],
	scenarios[1],
	scenarios[2],

	{
		name:                    "scenario 4 CentOS Docker 1.12",
		osName:                  "centos",
		containerRuntime:        "docker",
		containerRuntimeVersion: "1.12",
	},

	{
		name:                    "scenario 5 CentOS Docker 1.13",
		osName:                  "centos",
		containerRuntime:        "docker",
		containerRuntimeVersion: "1.13",
	},
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
	scenarios[0],
	scenarios[1],
	scenarios[2],
}

// TestVsphereProvisioning - a test suite that exercises vsphere provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestVsphereProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	vsPassword := os.Getenv("VSPHERE_PASSWORD")
	vsUsername := os.Getenv("VSPHERE_USERNAME")
	vsAddress := os.Getenv("VSPHERE_ADDRESS")
	if len(vsPassword) == 0 || len(vsUsername) == 0 || len(vsAddress) == 0 {
		t.Fatal("unable to run the test suite, VSPHERE_PASSWORD, VSPHERE_USERNAME or VSPHERE_ADDRESS environment variables cannot be empty")
	}

	// act
	params := fmt.Sprintf("<< VSPHERE_PASSWORD >>=%s,<< VSPHERE_USERNAME >>=%s,<< VSPHERE_ADDRESS >>=%s", vsPassword, vsUsername, vsAddress)
	runScenarios(t, vsphereScenarios, params, vs_manifest, "vs")
}
