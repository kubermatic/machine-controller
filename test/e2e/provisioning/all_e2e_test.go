// +build e2e

package provisioning

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

const (
	do_manifest    = "./testdata/machine-digitalocean.yaml"
	aws_manifest   = "./testdata/machine-aws.yaml"
	azure_manifest = "./testdata/machine-azure.yaml"
	hz_manifest    = "./testdata/machine-hetzner.yaml"
	vs_manifest    = "./testdata/machine-vsphere.yaml"
)

var testRunIdentifier = flag.String("identifier", "local", "The unique identifier for this test run")

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
	params := []string{fmt.Sprintf("<< DIGITALOCEAN_TOKEN >>=%s", doToken)}
	runScenarios(t, nil, params, do_manifest, fmt.Sprintf("do-%s", *testRunIdentifier))
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
	params := []string{fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
	}
	runScenarios(t, nil, params, aws_manifest, fmt.Sprintf("aws-%s", *testRunIdentifier))
}

// TestAzureProvisioningE2E - a test suite that exercises Azure provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestAzureProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	azureTenantID := os.Getenv("AZURE_E2E_TESTS_TENANT_ID")
	azureSubscriptionID := os.Getenv("AZURE_E2E_TESTS_SUBSCRIPTION_ID")
	azureClientID := os.Getenv("AZURE_E2E_TESTS_CLIENT_ID")
	azureClientSecret := os.Getenv("AZURE_E2E_TESTS_CLIENT_SECRET")
	if len(azureTenantID) == 0 || len(azureSubscriptionID) == 0 || len(azureClientID) == 0 || len(azureClientSecret) == 0 {
		t.Fatal("unable to run the test suite, AZURE_TENANT_ID, AZURE_SUBSCRIPTION_ID, AZURE_CLIENT_ID and AZURE_CLIENT_SECRET environment variables cannot be empty")
	}

	// TODO fix centos
	// cri-o doesn't work on Azure for some reason
	excludeSelector := &scenarioSelector{osName: []string{"centos"}, containerRuntime: []string{"cri-o"}}

	// act
	params := []string{
		fmt.Sprintf("<< AZURE_TENANT_ID >>=%s", azureTenantID),
		fmt.Sprintf("<< AZURE_SUBSCRIPTION_ID >>=%s", azureSubscriptionID),
		fmt.Sprintf("<< AZURE_CLIENT_ID >>=%s", azureClientID),
		fmt.Sprintf("<< AZURE_CLIENT_SECRET >>=%s", azureClientSecret),
	}
	runScenarios(t, excludeSelector, params, azure_manifest, fmt.Sprintf("azure-%s", *testRunIdentifier))
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

	// Hetzner does not support coreos
	excludeSelector := &scenarioSelector{osName: []string{"coreos"}}

	// act
	params := []string{fmt.Sprintf("<< HETZNER_TOKEN >>=%s", hzToken)}
	runScenarios(t, excludeSelector, params, hz_manifest, fmt.Sprintf("hz-%s", *testRunIdentifier))
}

// TestVsphereProvisioning - a test suite that exercises vsphere provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestVsphereProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	vsPassword := os.Getenv("VSPHERE_E2E_PASSWORD")
	vsUsername := os.Getenv("VSPHERE_E2E_USERNAME")
	vsCluster := os.Getenv("VSPHERE_E2E_CLUSTER")
	vsAddress := os.Getenv("VSPHERE_E2E_ADDRESS")
	if len(vsPassword) == 0 || len(vsUsername) == 0 || len(vsAddress) == 0 || len(vsCluster) == 0 {
		t.Fatal("unable to run the test suite, VSPHERE_E2E_PASSWORD, VSPHERE_E2E_USERNAME, VSPHERE_E2E_CLUSTER or VSPHERE_E2E_ADDRESS environment variables cannot be empty")
	}

	// Vsphere only supports Ubuntu
	excludeSelector := &scenarioSelector{osName: []string{"centos"}}

	// act
	params := []string{fmt.Sprintf("<< VSPHERE_PASSWORD >>=%s", vsPassword),
		fmt.Sprintf("<< VSPHERE_USERNAME >>=%s", vsUsername),
		fmt.Sprintf("<< VSPHERE_ADDRESS >>=%s", vsAddress),
		fmt.Sprintf("<< VSPHERE_CLUSTER >>=%s", vsCluster),
	}
	runScenarios(t, excludeSelector, params, vs_manifest, fmt.Sprintf("vs-%s", *testRunIdentifier))
}
