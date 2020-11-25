// +build e2e

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

package provisioning

import (
	"flag"
	"fmt"
	"os"
	"testing"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
)

func init() {
	klog.InitFlags(nil)
	if err := clusterv1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("failed to add clusterv1alpha1 to scheme: %v", err)
	}
}

const (
	DOManifest                   = "./testdata/machinedeployment-digitalocean.yaml"
	AWSManifest                  = "./testdata/machinedeployment-aws.yaml"
	AWSEBSEncryptedManifest      = "./testdata/machinedeployment-aws-ebs-encryption-enabled.yaml"
	AzureManifest                = "./testdata/machinedeployment-azure.yaml"
	AzureRedhatSatelliteManifest = "./testdata/machinedeployment-azure.yaml"
	GCEManifest                  = "./testdata/machinedeployment-gce.yaml"
	HZManifest                   = "./testdata/machinedeployment-hetzner.yaml"
	PacketManifest               = "./testdata/machinedeployment-packet.yaml"
	LinodeManifest               = "./testdata/machinedeployment-linode.yaml"
	VSPhereManifest              = "./testdata/machinedeployment-vsphere.yaml"
	VSPhereDSCManifest           = "./testdata/machinedeployment-vsphere-datastore-cluster.yaml"
	VSPhereResourcePoolManifest  = "./testdata/machinedeployment-vsphere-resource-pool.yaml"
	OSManifest                   = "./testdata/machinedeployment-openstack.yaml"
	OSUpgradeManifest            = "./testdata/machinedeployment-openstack-upgrade.yml"
	invalidMachineManifest       = "./testdata/machine-invalid.yaml"
	kubevirtManifest             = "./testdata/machinedeployment-kubevirt.yaml"
	kubevirtManifestDNSConfig    = "./testdata/machinedeployment-kubevirt-dns-config.yaml"
	alibabaManifest              = "./testdata/machinedeployment-alibaba.yaml"
)

var testRunIdentifier = flag.String("identifier", "local", "The unique identifier for this test run")

func TestInvalidObjectsGetRejected(t *testing.T) {
	t.Parallel()

	tests := []scenario{
		{osName: "invalid", executor: verifyCreateMachineFails},
		{osName: "coreos", executor: verifyCreateMachineFails},
	}

	for i, test := range tests {
		testScenario(t,
			test,
			fmt.Sprintf("invalid-machine-%v", i),
			nil,
			invalidMachineManifest,
			false)
	}
}

func TestKubevirtProvisioningE2E(t *testing.T) {
	t.Parallel()

	kubevirtKubeconfig := os.Getenv("KUBEVIRT_E2E_TESTS_KUBECONFIG")

	if kubevirtKubeconfig == "" {
		t.Fatalf("Unable to run kubevirt tests, KUBEVIRT_E2E_TESTS_KUBECONFIG must be set")
	}

	selector := Not(OsSelector("sles", "flatcar", "rhel"))
	params := []string{
		fmt.Sprintf("<< KUBECONFIG >>=%s", kubevirtKubeconfig),
	}

	runScenarios(t, selector, params, kubevirtManifest, fmt.Sprintf("kubevirt-%s", *testRunIdentifier))
}

func TestKubevirtDNSConfigProvisioningE2E(t *testing.T) {
	t.Parallel()

	kubevirtKubeconfig := os.Getenv("KUBEVIRT_E2E_TESTS_KUBECONFIG")

	if kubevirtKubeconfig == "" {
		t.Fatalf("Unable to run kubevirt tests, KUBEVIRT_E2E_TESTS_KUBECONFIG must be set")
	}

	params := []string{
		fmt.Sprintf("<< KUBECONFIG >>=%s", kubevirtKubeconfig),
	}

	scenario := scenario{
		name:              "Kubevirt with dns config",
		osName:            "ubuntu",
		containerRuntime:  "docker",
		kubernetesVersion: "v1.17.0",
		executor:          verifyCreateAndDelete,
	}

	testScenario(t, scenario, *testRunIdentifier, params, kubevirtManifestDNSConfig, false)
}

func TestOpenstackProvisioningE2E(t *testing.T) {
	t.Parallel()

	osAuthURL := os.Getenv("OS_AUTH_URL")
	osDomain := os.Getenv("OS_DOMAIN")
	osPassword := os.Getenv("OS_PASSWORD")
	osRegion := os.Getenv("OS_REGION")
	osUsername := os.Getenv("OS_USERNAME")
	osTenant := os.Getenv("OS_TENANT_NAME")
	osNetwork := os.Getenv("OS_NETWORK_NAME")

	if osAuthURL == "" || osUsername == "" || osPassword == "" || osDomain == "" || osRegion == "" || osTenant == "" {
		t.Fatal("unable to run test suite, all of OS_AUTH_URL, OS_USERNAME, OS_PASSOWRD, OS_REGION, OS_TENANT and OS_DOMAIN must be set!")
	}

	params := []string{
		fmt.Sprintf("<< IDENTITY_ENDPOINT >>=%s", osAuthURL),
		fmt.Sprintf("<< USERNAME >>=%s", osUsername),
		fmt.Sprintf("<< PASSWORD >>=%s", osPassword),
		fmt.Sprintf("<< DOMAIN_NAME >>=%s", osDomain),
		fmt.Sprintf("<< REGION >>=%s", osRegion),
		fmt.Sprintf("<< TENANT_NAME >>=%s", osTenant),
		fmt.Sprintf("<< NETWORK_NAME >>=%s", osNetwork),
	}

	selector := Not(OsSelector("sles", "rhel"))
	runScenarios(t, selector, params, OSManifest, fmt.Sprintf("os-%s", *testRunIdentifier))
}

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

	selector := Not(OsSelector("sles", "rhel", "flatcar", "coreos"))
	// act
	params := []string{fmt.Sprintf("<< DIGITALOCEAN_TOKEN >>=%s", doToken)}
	runScenarios(t, selector, params, DOManifest, fmt.Sprintf("do-%s", *testRunIdentifier))
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
	selector := Not(OsSelector("coreos", "sles"))
	// act
	params := []string{fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
	}
	runScenarios(t, selector, params, AWSManifest, fmt.Sprintf("aws-%s", *testRunIdentifier))
}

// TestAWSSLESProvisioningE2E - a test suite that exercises AWS provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestAWSSLESProvisioningE2E(t *testing.T) {
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

	// We would like to test SLES image only in this test as the other images are tested in TestAWSProvisioningE2E
	selector := OsSelector("sles")
	runScenarios(t, selector, params, AWSManifest, fmt.Sprintf("aws-%s", *testRunIdentifier))
}

func TestAWSCentOS8ProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	awsKeyID := os.Getenv("AWS_E2E_TESTS_KEY_ID")
	awsSecret := os.Getenv("AWS_E2E_TESTS_SECRET")
	if len(awsKeyID) == 0 || len(awsSecret) == 0 {
		t.Fatal("unable to run the test suite, AWS_E2E_TESTS_KEY_ID or AWS_E2E_TESTS_SECRET environment variables cannot be empty")
	}

	amiID := "ami-032025b3afcbb6b34" // official "CentOS 8.2.2004 x86_64"

	params := []string{
		fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
		fmt.Sprintf("<< AMI >>=%s", amiID),
	}

	// We would like to test CentOS8 image only in this test as the other images are tested in TestAWSProvisioningE2E
	selector := OsSelector("centos")
	runScenarios(t, selector, params, AWSManifest, fmt.Sprintf("aws-%s", *testRunIdentifier))
}

// TestAWSProvisioningE2EWithEbsEncryptionEnabled - a test suite that exercises AWS provider with ebs encryption enabled
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestAWSProvisioningE2EWithEbsEncryptionEnabled(t *testing.T) {
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

	scenario := scenario{
		name:              "AWS with ebs encryption enabled",
		osName:            "ubuntu",
		containerRuntime:  "docker",
		kubernetesVersion: "v1.15.6",
		executor:          verifyCreateAndDelete,
	}
	testScenario(t, scenario, fmt.Sprintf("aws-%s", *testRunIdentifier), params, AWSEBSEncryptedManifest, false)
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

	selector := Not(OsSelector("coreos", "sles"))
	// act
	params := []string{
		fmt.Sprintf("<< AZURE_TENANT_ID >>=%s", azureTenantID),
		fmt.Sprintf("<< AZURE_SUBSCRIPTION_ID >>=%s", azureSubscriptionID),
		fmt.Sprintf("<< AZURE_CLIENT_ID >>=%s", azureClientID),
		fmt.Sprintf("<< AZURE_CLIENT_SECRET >>=%s", azureClientSecret),
	}
	runScenarios(t, selector, params, AzureManifest, fmt.Sprintf("azure-%s", *testRunIdentifier))
}

// TestAzureRedhatSatelliteProvisioningE2E - a test suite that exercises Azure provider
// by requesting rhel node and subscribe to redhat satellite server.
func TestAzureRedhatSatelliteProvisioningE2E(t *testing.T) {
	t.Parallel()
	t.Skip()

	// test data
	azureTenantID := os.Getenv("AZURE_E2E_TESTS_TENANT_ID")
	azureSubscriptionID := os.Getenv("AZURE_E2E_TESTS_SUBSCRIPTION_ID")
	azureClientID := os.Getenv("AZURE_E2E_TESTS_CLIENT_ID")
	azureClientSecret := os.Getenv("AZURE_E2E_TESTS_CLIENT_SECRET")
	if len(azureTenantID) == 0 || len(azureSubscriptionID) == 0 || len(azureClientID) == 0 || len(azureClientSecret) == 0 {
		t.Fatal("unable to run the test suite, AZURE_TENANT_ID, AZURE_SUBSCRIPTION_ID, AZURE_CLIENT_ID and AZURE_CLIENT_SECRET environment variables cannot be empty")
	}

	// act
	params := []string{
		fmt.Sprintf("<< AZURE_TENANT_ID >>=%s", azureTenantID),
		fmt.Sprintf("<< AZURE_SUBSCRIPTION_ID >>=%s", azureSubscriptionID),
		fmt.Sprintf("<< AZURE_CLIENT_ID >>=%s", azureClientID),
		fmt.Sprintf("<< AZURE_CLIENT_SECRET >>=%s", azureClientSecret),
	}

	scenario := scenario{
		name:              "Azure redhat satellite server subscription",
		osName:            "rhel",
		containerRuntime:  "docker",
		kubernetesVersion: "v1.17.0",
		executor:          verifyCreateAndDelete,
	}

	testScenario(t, scenario, *testRunIdentifier, params, AzureRedhatSatelliteManifest, false)
}

// TestGCEProvisioningE2E - a test suite that exercises Google Cloud provider
// by requesting nodes with different combination of container runtime type,
// container runtime version and the OS flavour.
func TestGCEProvisioningE2E(t *testing.T) {
	t.Parallel()

	// Test data.
	googleServiceAccount := os.Getenv("GOOGLE_SERVICE_ACCOUNT")
	if len(googleServiceAccount) == 0 {
		t.Fatal("unable to run the test suite, GOOGLE_SERVICE_ACCOUNT environment variable cannot be empty")
	}

	// Act. GCE does not support CentOS.
	selector := OsSelector("ubuntu")
	params := []string{
		fmt.Sprintf("<< GOOGLE_SERVICE_ACCOUNT >>=%s", googleServiceAccount),
	}
	runScenarios(t, selector, params, GCEManifest, fmt.Sprintf("gce-%s", *testRunIdentifier))
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
	selector := Not(OsSelector("coreos", "sles", "rhel", "flatcar"))

	// act
	params := []string{fmt.Sprintf("<< HETZNER_TOKEN >>=%s", hzToken)}
	runScenarios(t, selector, params, HZManifest, fmt.Sprintf("hz-%s", *testRunIdentifier))
}

// TestPacketProvisioning - a test suite that exercises Packet provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestPacketProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	apiKey := os.Getenv("PACKET_API_KEY")
	if len(apiKey) == 0 {
		t.Fatal("unable to run the test suite, PACKET_API_KEY environment variable cannot be empty")
	}

	projectID := os.Getenv("PACKET_PROJECT_ID")
	if len(projectID) == 0 {
		t.Fatal("unable to run the test suite, PACKET_PROJECT_ID environment variable cannot be empty")
	}

	selector := Not(OsSelector("sles", "rhel"))

	// act
	params := []string{
		fmt.Sprintf("<< PACKET_API_KEY >>=%s", apiKey),
		fmt.Sprintf("<< PACKET_PROJECT_ID >>=%s", projectID),
	}
	runScenarios(t, selector, params, PacketManifest, fmt.Sprintf("packet-%s", *testRunIdentifier))
}

func TestAlibabaProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	accessKeyID := os.Getenv("ALIBABA_ACCESS_KEY_ID")
	if len(accessKeyID) == 0 {
		t.Fatal("unable to run the test suite, ALIBABA_ACCESS_KEY_ID environment variable cannot be empty")
	}

	accessKeySecret := os.Getenv("ALIBABA_ACCESS_KEY_SECRET")
	if len(accessKeySecret) == 0 {
		t.Fatal("unable to run the test suite, ALIBABA_ACCESS_KEY_SECRET environment variable cannot be empty")
	}

	selector := Not(OsSelector("coreos", "sles", "rhel", "flatcar"))

	// act
	params := []string{
		fmt.Sprintf("<< ALIBABA_ACCESS_KEY_ID >>=%s", accessKeyID),
		fmt.Sprintf("<< ALIBABA_ACCESS_KEY_SECRET >>=%s", accessKeySecret),
	}
	runScenarios(t, selector, params, alibabaManifest, fmt.Sprintf("alibaba-%s", *testRunIdentifier))
}

// TestLinodeProvisioning - a test suite that exercises Linode provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
//
// note that tests require  a valid API Token that is read from the LINODE_E2E_TEST_TOKEN environmental variable.
func TestLinodeProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	linodeToken := os.Getenv("LINODE_E2E_TESTS_TOKEN")
	if len(linodeToken) == 0 {
		t.Fatal("unable to run the test suite, LINODE_E2E_TESTS_TOKEN environment variable cannot be empty")
	}

	// we're shimming userdata through Linode stackscripts, and Linode's coreos does not support stackscripts
	// and the stackscript hasn't been verified for use with centos
	selector := OsSelector("ubuntu")

	// act
	params := []string{fmt.Sprintf("<< LINODE_TOKEN >>=%s", linodeToken)}
	runScenarios(t, selector, params, LinodeManifest, fmt.Sprintf("linode-%s", *testRunIdentifier))
}

func getVSphereTestParams(t *testing.T) []string {
	// test data
	vsPassword := os.Getenv("VSPHERE_E2E_PASSWORD")
	vsUsername := os.Getenv("VSPHERE_E2E_USERNAME")
	vsCluster := os.Getenv("VSPHERE_E2E_CLUSTER")
	vsAddress := os.Getenv("VSPHERE_E2E_ADDRESS")

	if vsPassword == "" || vsUsername == "" || vsAddress == "" || vsCluster == "" {
		t.Fatal("unable to run the test suite, VSPHERE_E2E_PASSWORD, VSPHERE_E2E_USERNAME, VSPHERE_E2E_CLUSTER " +
			"or VSPHERE_E2E_ADDRESS environment variables cannot be empty")
	}

	// act
	params := []string{fmt.Sprintf("<< VSPHERE_PASSWORD >>=%s", vsPassword),
		fmt.Sprintf("<< VSPHERE_USERNAME >>=%s", vsUsername),
		fmt.Sprintf("<< VSPHERE_ADDRESS >>=%s", vsAddress),
		fmt.Sprintf("<< VSPHERE_CLUSTER >>=%s", vsCluster),
	}
	return params
}

// TestVsphereProvisioning - a test suite that exercises vsphere provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestVsphereProvisioningE2E(t *testing.T) {
	t.Parallel()

	selector := Not(OsSelector("sles", "rhel"))

	params := getVSphereTestParams(t)
	runScenarios(t, selector, params, VSPhereManifest, fmt.Sprintf("vs-%s", *testRunIdentifier))
}

// TestVsphereDatastoreClusterProvisioning - is the same as the TestVsphereProvisioning suite but specifies a DatastoreCluster
// instead of the Datastore in the provider specs.
func TestVsphereDatastoreClusterProvisioningE2E(t *testing.T) {
	t.Parallel()

	selector := Not(OsSelector("sles", "rhel", "flatcar", "rhel"))

	params := getVSphereTestParams(t)
	runScenarios(t, selector, params, VSPhereDSCManifest, fmt.Sprintf("vs-dsc-%s", *testRunIdentifier))
}

// TestVsphereResourcePoolProvisioning - creates a machine deployment using a
// resource pool.
func TestVsphereResourcePoolProvisioningE2E(t *testing.T) {
	t.Parallel()

	params := getVSphereTestParams(t)
	// We do not need to test all combinations.
	scenario := scenario{
		name:              "vSphere resource pool provisioning",
		osName:            "coreos",
		containerRuntime:  "docker",
		kubernetesVersion: "1.17.0",
		executor:          verifyCreateAndDelete,
	}

	testScenario(t, scenario, *testRunIdentifier, params, VSPhereResourcePoolManifest, false)
}

// TestUbuntuProvisioningWithUpgradeE2E will create an instance from an old Ubuntu 1604
// image and upgrade it prior to joining the cluster
func TestUbuntuProvisioningWithUpgradeE2E(t *testing.T) {
	t.Parallel()

	osAuthURL := os.Getenv("OS_AUTH_URL")
	osDomain := os.Getenv("OS_DOMAIN")
	osPassword := os.Getenv("OS_PASSWORD")
	osRegion := os.Getenv("OS_REGION")
	osUsername := os.Getenv("OS_USERNAME")
	osTenant := os.Getenv("OS_TENANT_NAME")
	osNetwork := os.Getenv("OS_NETWORK_NAME")

	if osAuthURL == "" || osUsername == "" || osPassword == "" || osDomain == "" || osRegion == "" || osTenant == "" {
		t.Fatal("unable to run test, all of OS_AUTH_URL, OS_USERNAME, OS_PASSOWRD, OS_REGION, OS_TENANT and OS_DOMAIN must be set!")
	}

	params := []string{
		fmt.Sprintf("<< IDENTITY_ENDPOINT >>=%s", osAuthURL),
		fmt.Sprintf("<< USERNAME >>=%s", osUsername),
		fmt.Sprintf("<< PASSWORD >>=%s", osPassword),
		fmt.Sprintf("<< DOMAIN_NAME >>=%s", osDomain),
		fmt.Sprintf("<< REGION >>=%s", osRegion),
		fmt.Sprintf("<< TENANT_NAME >>=%s", osTenant),
		fmt.Sprintf("<< NETWORK_NAME >>=%s", osNetwork),
	}
	scenario := scenario{
		name:              "Ubuntu upgrade",
		osName:            "ubuntu",
		containerRuntime:  "docker",
		kubernetesVersion: "1.16.2",
		executor:          verifyCreateAndDelete,
	}

	testScenario(t, scenario, *testRunIdentifier, params, OSUpgradeManifest, false)
}

// TestDeploymentControllerUpgradesMachineE2E verifies the machineDeployment controller correctly
// rolls over machines on changes in the machineDeployment
func TestDeploymentControllerUpgradesMachineE2E(t *testing.T) {
	t.Parallel()

	// test data
	hzToken := os.Getenv("HZ_E2E_TOKEN")
	if len(hzToken) == 0 {
		t.Fatal("unable to run the test suite, HZ_E2E_TOKEN environment variable cannot be empty")
	}

	// act
	params := []string{fmt.Sprintf("<< HETZNER_TOKEN >>=%s", hzToken)}

	scenario := scenario{
		name:              "MachineDeployment upgrade",
		osName:            "ubuntu",
		containerRuntime:  "docker",
		kubernetesVersion: "1.16.2",
		executor:          verifyCreateUpdateAndDelete,
	}
	testScenario(t, scenario, *testRunIdentifier, params, HZManifest, false)
}
