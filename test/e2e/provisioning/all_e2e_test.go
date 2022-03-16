//go:build e2e

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
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/kubermatic/machine-controller/pkg/userdata/flatcar"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	klog.InitFlags(nil)
	if err := clusterv1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("failed to add clusterv1alpha1 to scheme: %v", err)
	}
}

const (
	DOManifest                        = "./testdata/machinedeployment-digitalocean.yaml"
	AWSManifest                       = "./testdata/machinedeployment-aws.yaml"
	AWSSpotInstanceManifest           = "./testdata/machinedeployment-aws-spot-instances.yaml"
	AWSManifestARM                    = "./testdata/machinedeployment-aws-arm-machines.yaml"
	AWSEBSEncryptedManifest           = "./testdata/machinedeployment-aws-ebs-encryption-enabled.yaml"
	AzureManifest                     = "./testdata/machinedeployment-azure.yaml"
	AzureRedhatSatelliteManifest      = "./testdata/machinedeployment-azure.yaml"
	AzureCustomImageReferenceManifest = "./testdata/machinedeployment-azure-custom-image-reference.yaml"
	EquinixMetalManifest              = "./testdata/machinedeployment-equinixmetal.yaml"
	GCEManifest                       = "./testdata/machinedeployment-gce.yaml"
	HZManifest                        = "./testdata/machinedeployment-hetzner.yaml"
	LinodeManifest                    = "./testdata/machinedeployment-linode.yaml"
	VSPhereManifest                   = "./testdata/machinedeployment-vsphere.yaml"
	VSPhereDSCManifest                = "./testdata/machinedeployment-vsphere-datastore-cluster.yaml"
	VSPhereResourcePoolManifest       = "./testdata/machinedeployment-vsphere-resource-pool.yaml"
	ScalewayManifest                  = "./testdata/machinedeployment-scaleway.yaml"
	OSMachineManifest                 = "./testdata/machine-openstack.yaml"
	OSManifest                        = "./testdata/machinedeployment-openstack.yaml"
	OSManifestProjectAuth             = "./testdata/machinedeployment-openstack-project-auth.yaml"
	OSUpgradeManifest                 = "./testdata/machinedeployment-openstack-upgrade.yml"
	invalidMachineManifest            = "./testdata/machine-invalid.yaml"
	kubevirtManifest                  = "./testdata/machinedeployment-kubevirt.yaml"
	alibabaManifest                   = "./testdata/machinedeployment-alibaba.yaml"
	anexiaManifest                    = "./testdata/machinedeployment-anexia.yaml"
	nutanixManifest                   = "./testdata/machinedeployment-nutanix.yaml"
)

var testRunIdentifier = flag.String("identifier", "local", "The unique identifier for this test run")

func TestInvalidObjectsGetRejected(t *testing.T) {
	t.Parallel()

	tests := []scenario{
		{osName: "invalid", executor: verifyCreateMachineFails},
		{osName: "flatcar", executor: verifyCreateMachineFails},
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

// TestCustomCAsAreApplied ensures that the configured CA bundle is actually
// being used by performing a negative test: It purposefully replaces the
// valid CA bundle with a bundle that contains one random self-signed cert
// and then expects openstack provisioning to _fail_.
func TestCustomCAsAreApplied(t *testing.T) {
	t.Parallel()

	osAuthURL := os.Getenv("OS_AUTH_URL")
	osDomain := os.Getenv("OS_DOMAIN")
	osPassword := os.Getenv("OS_PASSWORD")
	osRegion := os.Getenv("OS_REGION")
	osUsername := os.Getenv("OS_USERNAME")
	osTenant := os.Getenv("OS_TENANT_NAME")
	osNetwork := os.Getenv("OS_NETWORK_NAME")

	if osAuthURL == "" || osUsername == "" || osPassword == "" || osDomain == "" || osRegion == "" || osTenant == "" {
		t.Fatal("unable to run test suite, all of OS_AUTH_URL, OS_USERNAME, OS_PASSOWRD, OS_REGION, and OS_TENANT OS_DOMAIN must be set!")
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

	testScenario(
		t,
		scenario{
			name:              "ca-test",
			containerRuntime:  "docker",
			kubernetesVersion: versions[0].String(),
			osName:            string(providerconfigtypes.OperatingSystemUbuntu),

			executor: func(kubeConfig, manifestPath string, parameters []string, d time.Duration) error {
				if err := updateMachineControllerForCustomCA(kubeConfig); err != nil {
					return fmt.Errorf("failed to add CA: %v", err)
				}

				return verifyCreateMachineFails(kubeConfig, manifestPath, parameters, d)
			},
		},
		"dummy-machine",
		params,
		OSMachineManifest,
		false,
	)
}

func updateMachineControllerForCustomCA(kubeconfig string) error {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("Error building kubeconfig: %v", err)
	}

	client, err := ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
	if err != nil {
		return fmt.Errorf("failed to create Client: %v", err)
	}

	ctx := context.Background()
	ns := metav1.NamespaceSystem

	// create intentionally valid but useless CA bundle
	caBundle := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Namespace: ns,
			Name:      "ca-bundle",
		},
		Data: map[string]string{
			// this certificate was created using `make examples/ca-cert.pem`
			"ca-bundle.pem": strings.TrimSpace(`
-----BEGIN CERTIFICATE-----
MIIFezCCA2OgAwIBAgIUV9en2WQLDZ1VzPYgzblnuhrg1sQwDQYJKoZIhvcNAQEL
BQAwTTELMAkGA1UEBhMCVVMxCzAJBgNVBAgMAkNBMQ0wCwYDVQQKDARBY21lMSIw
IAYDVQQDDBlrOHMtbWFjaGluZS1jb250cm9sbGVyLWNhMB4XDTIwMDgxOTEzNDEw
MloXDTQ4MDEwNTEzNDEwMlowTTELMAkGA1UEBhMCVVMxCzAJBgNVBAgMAkNBMQ0w
CwYDVQQKDARBY21lMSIwIAYDVQQDDBlrOHMtbWFjaGluZS1jb250cm9sbGVyLWNh
MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEAtHUcIL+zTd7jmYazQCL0
auCJxbICdthFzW3Hs8FwQ3zhXiqP7bsgMgLsG5lxmRA1iyRRUQklV/Cu6XTWQkPA
Z8WqA06zhoNl7/f5tfhilJS6RP3ftlDJ9UMVb2DaG560VF+31QHZKL8Hr0KgPdz9
WgUFTpD1LpOk0wHJdjc/WzKaTFrZm3UAZRcZIkR0+5LrUudmUPYfbHWtYSYLX2vB
Y0+9oKqcpTtoFE2jGa993dtSPSE7grG3kfKb+IhwHUDXOW0xiT/uue7JAJYc6fDd
RoRdf3vSIESl9+R7lxymcW5R9YrQ26YJ6HlVr14BpT0hNVgvrpJINstYBpj5PbQV
kpIcHmrDOoZEgb+QTAtzga0mZctWWa7U1AJ8KoWejrJgNCAE4nrecFaPQ7aDjSe4
ca0/Gx1TtLPhswMFqQhihK4bxuV1iTTsk++h8rK5ii6jO6ioS+AF9Nqye+1tYuE8
JePXMMkO1pnwKeyiRGs8poJdQEXzu0xYbc/f2FZqP4b9X4TfsVC5WQIO/xhfhaOI
l0cIKTaBn5mWW5gn/ag+AnaTHZ7aX3A4zAuE/riyTFC2GWNLO5PqlTgo6c/+5ynC
x5Q6CUBIMFw4LP8DMC2bWhyJjRaCre9+3bXSXQ8XCWxAyfTjDTcIgBEv0+peGko0
wb697GGWGgiqlRpW8GBZPeUCAwEAAaNTMFEwHQYDVR0OBBYEFO2EDvPI7jRqR6rK
vKkqj8BxCCZvMB8GA1UdIwQYMBaAFO2EDvPI7jRqR6rKvKkqj8BxCCZvMA8GA1Ud
EwEB/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggIBAJMXHPxnorj4h+HePA1TaqBs
LPfrARxPGi+/mFWGtT8JpLf8cP1YT3j74sdD9oiDCxDSL+Cg5JY3IKa5U+jnS6Go
a26D4U9MOUl2hPOa/f4BEEN9+6jNvB/jg1Jd1YC1Q7hdWjBZKx1n+qMhq+bwNJZo
du0t/zmgk6sJVa7E50ILv/WmEQDCo0NFpOBSku0M35iA+maMgxq5/7EBybl2Qo2F
j6IPTxGBRbOE13I8virmYz9MdloiKX1GDUDCP3yRSSnPVveKlaGYa/lCNbSEhynb
KZHbzcro71RRAgne1cNaFIqr5oCZMSSx+hlsc/mkenr7Dg1/o6FexFc1IYO85Fs4
VC8Yb5V2oD8IDZlRVo7G74cZqEly8OYHO17zO0ib3S70aPGTUtFXEyMiirWCbVCb
L3I2dvQcO19WTQ8CfujWGtbL2lhBZvJTfa9fzrz3uYQRIeBHIWZvi8sEIQ1pmeOi
9PQkGHHJO+jfJkbOdR9cAmHUyuHH26WzZctg5CR2+f6xA8kO/8tUMEAJ9hUJa1iJ
Br0c+gPd5UmjrHLikc40/CgjmfLkaSJcnmiYP0xxYM3Rqm8ptKJM7asHDDbeBK8m
rh3NiRD903zsNpRiUXKkQs7N382SkRaBTB/rJTONM00pXEQYAivs5nIEfCQzen/Z
C8QmzsMaZhk+mVFr1sGy
-----END CERTIFICATE-----
`),
		},
	}

	if err := client.Create(ctx, caBundle); err != nil {
		return fmt.Errorf("failed to create ca-bundle ConfigMap: %v", err)
	}

	// add CA to deployments
	deployments := []string{"machine-controller", "machine-controller-webhook"}
	for _, deployment := range deployments {
		if err := addCAToDeployment(ctx, client, deployment, ns); err != nil {
			return fmt.Errorf("failed to add CA to %s Deployment: %v", deployment, err)
		}
	}

	// wait for deployments to roll out
	for _, deployment := range deployments {
		if err := wait.Poll(3*time.Second, 30*time.Second, func() (done bool, err error) {
			d := &appsv1.Deployment{}
			key := types.NamespacedName{Namespace: ns, Name: deployment}

			if err := client.Get(ctx, key, d); err != nil {
				return false, fmt.Errorf("failed to get Deployment: %v", err)
			}

			return d.Status.AvailableReplicas > 0, nil
		}); err != nil {
			return fmt.Errorf("%s Deployment never became ready: %v", deployment, err)
		}
	}

	return nil
}

func addCAToDeployment(ctx context.Context, client ctrlruntimeclient.Client, name string, namespace string) error {
	deployment := &appsv1.Deployment{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	if err := client.Get(ctx, key, deployment); err != nil {
		return fmt.Errorf("failed to get Deployment: %v", err)
	}

	caVolume := corev1.Volume{
		Name: "ca-bundle",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "ca-bundle",
				},
			},
		},
	}

	caVolumeMount := corev1.VolumeMount{
		Name:      "ca-bundle",
		ReadOnly:  true,
		MountPath: "/etc/machine-controller",
	}

	oldDeployment := deployment.DeepCopy()

	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, caVolume)

	container := deployment.Spec.Template.Spec.Containers[0]
	container.VolumeMounts = append(container.VolumeMounts, caVolumeMount)
	container.Command = append(container.Command, "-ca-bundle=/etc/machine-controller/ca-bundle.pem")

	deployment.Spec.Template.Spec.Containers[0] = container

	return client.Patch(ctx, deployment, ctrlruntimeclient.MergeFrom(oldDeployment))
}

func TestKubevirtProvisioningE2E(t *testing.T) {
	t.Parallel()

	kubevirtKubeconfig := os.Getenv("KUBEVIRT_E2E_TESTS_KUBECONFIG")

	if kubevirtKubeconfig == "" {
		t.Fatalf("Unable to run kubevirt tests, KUBEVIRT_E2E_TESTS_KUBECONFIG must be set")
	}

	selector := OsSelector("ubuntu", "centos", "flatcar")

	params := []string{
		fmt.Sprintf("<< KUBECONFIG >>=%s", kubevirtKubeconfig),
	}

	runScenarios(t, selector, params, kubevirtManifest, fmt.Sprintf("kubevirt-%s", *testRunIdentifier))
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
		t.Fatal("unable to run test suite, all of OS_AUTH_URL, OS_USERNAME, OS_PASSOWRD, OS_REGION, and OS_TENANT OS_DOMAIN must be set!")
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

	selector := Not(OsSelector("sles", "rhel", "amzn2"))
	runScenarios(t, selector, params, OSManifest, fmt.Sprintf("os-%s", *testRunIdentifier))
}

func TestOpenstackProjectAuthProvisioningE2E(t *testing.T) {
	t.Parallel()

	osAuthURL := os.Getenv("OS_AUTH_URL")
	osDomain := os.Getenv("OS_DOMAIN")
	osPassword := os.Getenv("OS_PASSWORD")
	osRegion := os.Getenv("OS_REGION")
	osUsername := os.Getenv("OS_USERNAME")

	// not a mistake: openstack has deprecated OS_TENANT_NAME in favor of OS_PROJECT_NAME, but it contains same value.
	osProject := os.Getenv("OS_TENANT_NAME")
	osNetwork := os.Getenv("OS_NETWORK_NAME")

	if osAuthURL == "" || osUsername == "" || osPassword == "" || osDomain == "" || osRegion == "" || osProject == "" {
		t.Fatal("unable to run test suite, all of OS_AUTH_URL, OS_USERNAME, OS_PASSOWRD, OS_REGION, and OS_TENANT OS_DOMAIN must be set!")
	}

	params := []string{
		fmt.Sprintf("<< IDENTITY_ENDPOINT >>=%s", osAuthURL),
		fmt.Sprintf("<< USERNAME >>=%s", osUsername),
		fmt.Sprintf("<< PASSWORD >>=%s", osPassword),
		fmt.Sprintf("<< DOMAIN_NAME >>=%s", osDomain),
		fmt.Sprintf("<< REGION >>=%s", osRegion),
		fmt.Sprintf("<< PROJECT_NAME >>=%s", osProject),
		fmt.Sprintf("<< NETWORK_NAME >>=%s", osNetwork),
	}

	scenario := scenario{
		name:              "MachineDeploy with project auth vars",
		osName:            "ubuntu",
		containerRuntime:  "containerd",
		kubernetesVersion: "1.21.8",
		executor:          verifyCreateAndDelete,
	}
	testScenario(t, scenario, *testRunIdentifier, params, OSManifestProjectAuth, false)
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
		t.Fatal("unable to run the test suite, DO_E2E_TESTS_TOKEN environment variable cannot be empty")
	}

	selector := OsSelector("ubuntu", "centos")

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
	selector := Not(OsSelector("sles"))
	// act
	params := []string{fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
		fmt.Sprintf("<< PROVISIONING_UTILITY >>=%s", flatcar.CloudInit),
	}
	runScenarios(t, selector, params, AWSManifest, fmt.Sprintf("aws-%s", *testRunIdentifier))
}

// TestAWSAssumeRoleProvisioning - a test suite that exercises AWS provider
// by requesting nodes using an assumed role.
func TestAWSAssumeRoleProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	awsKeyID := os.Getenv("AWS_E2E_TESTS_KEY_ID")
	awsSecret := os.Getenv("AWS_E2E_TESTS_SECRET")
	awsAssumeRoleARN := os.Getenv("AWS_ASSUME_ROLE_ARN")
	awsAssumeRoleExternalID := os.Getenv("AWS_ASSUME_ROLE_EXTERNAL_ID")
	if len(awsKeyID) == 0 || len(awsSecret) == 0 || len(awsAssumeRoleARN) == 0 || len(awsAssumeRoleExternalID) == 0 {
		t.Fatal("unable to run the test suite, environment variables AWS_E2E_TESTS_KEY_ID, AWS_E2E_TESTS_SECRET, AWS_E2E_ASSUME_ROLE_ARN and AWS_E2E_ASSUME_ROLE_EXTERNAL_ID cannot be empty")
	}

	// act
	params := []string{fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
		fmt.Sprintf("<< PROVISIONING_UTILITY >>=%s", flatcar.CloudInit),
	}

	scenario := scenario{
		name:              "AWS with AssumeRole",
		osName:            "ubuntu",
		containerRuntime:  "docker",
		kubernetesVersion: "1.22.5",
		executor:          verifyCreateAndDelete,
	}
	testScenario(t, scenario, *testRunIdentifier, params, AWSManifest, false)
}

// TestAWSSpotInstanceProvisioning - a test suite that exercises AWS provider
// by requesting spot nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestAWSSpotInstanceProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	awsKeyID := os.Getenv("AWS_E2E_TESTS_KEY_ID")
	awsSecret := os.Getenv("AWS_E2E_TESTS_SECRET")
	if len(awsKeyID) == 0 || len(awsSecret) == 0 {
		t.Fatal("unable to run the test suite, AWS_E2E_TESTS_KEY_ID or AWS_E2E_TESTS_SECRET environment variables cannot be empty")
	}
	selector := Not(OsSelector("sles"))
	// act
	params := []string{fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
		fmt.Sprintf("<< PROVISIONING_UTILITY >>=%s", flatcar.CloudInit),
	}
	runScenarios(t, selector, params, AWSSpotInstanceManifest, fmt.Sprintf("aws-%s", *testRunIdentifier))
}

// TestAWSARMProvisioningE2E - a test suite that exercises AWS provider for arm machines
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestAWSARMProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	awsKeyID := os.Getenv("AWS_E2E_TESTS_KEY_ID")
	awsSecret := os.Getenv("AWS_E2E_TESTS_SECRET")
	if len(awsKeyID) == 0 || len(awsSecret) == 0 {
		t.Fatal("unable to run the test suite, AWS_E2E_TESTS_KEY_ID or AWS_E2E_TESTS_SECRET environment variables cannot be empty")
	}
	selector := OsSelector("ubuntu")
	// act
	params := []string{fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
		fmt.Sprintf("<< PROVISIONING_UTILITY >>=%s", flatcar.Ignition),
	}
	runScenarios(t, selector, params, AWSManifestARM, fmt.Sprintf("aws-%s", *testRunIdentifier))
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

func TestAWSFlatcarCoreOSCloudInit8ProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	awsKeyID := os.Getenv("AWS_E2E_TESTS_KEY_ID")
	awsSecret := os.Getenv("AWS_E2E_TESTS_SECRET")
	if len(awsKeyID) == 0 || len(awsSecret) == 0 {
		t.Fatal("unable to run the test suite, AWS_E2E_TESTS_KEY_ID or AWS_E2E_TESTS_SECRET environment variables cannot be empty")
	}

	params := []string{
		fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
		fmt.Sprintf("<< PROVISIONING_UTILITY >>=%s", flatcar.CloudInit),
	}

	// We would like to test flatcar with CoreOS-cloud-init
	selector := OsSelector("flatcar")
	runScenarios(t, selector, params, AWSManifest, fmt.Sprintf("aws-%s", *testRunIdentifier))
}

func TestAWSFlatcarContainerdProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	awsKeyID := os.Getenv("AWS_E2E_TESTS_KEY_ID")
	awsSecret := os.Getenv("AWS_E2E_TESTS_SECRET")
	if len(awsKeyID) == 0 || len(awsSecret) == 0 {
		t.Fatal("unable to run the test suite, AWS_E2E_TESTS_KEY_ID or AWS_E2E_TESTS_SECRET environment variables cannot be empty")
	}

	params := []string{
		fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
		fmt.Sprintf("<< PROVISIONING_UTILITY >>=%s", flatcar.CloudInit),
	}

	scenario := scenario{
		name:              "flatcar with containerd in AWS",
		osName:            "flatcar",
		containerRuntime:  "containerd",
		kubernetesVersion: "1.22.5",
		executor:          verifyCreateAndDelete,
	}
	testScenario(t, scenario, *testRunIdentifier, params, AWSManifest, false)
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

// TestAWSEbsEncryptionEnabledProvisioningE2E - a test suite that exercises AWS provider with ebs encryption enabled
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestAWSEbsEncryptionEnabledProvisioningE2E(t *testing.T) {
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
		containerRuntime:  "containerd",
		kubernetesVersion: "v1.21.8",
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

	selector := Not(OsSelector("sles", "amzn2"))
	// act
	params := []string{
		fmt.Sprintf("<< AZURE_TENANT_ID >>=%s", azureTenantID),
		fmt.Sprintf("<< AZURE_SUBSCRIPTION_ID >>=%s", azureSubscriptionID),
		fmt.Sprintf("<< AZURE_CLIENT_ID >>=%s", azureClientID),
		fmt.Sprintf("<< AZURE_CLIENT_SECRET >>=%s", azureClientSecret),
	}
	runScenarios(t, selector, params, AzureManifest, fmt.Sprintf("azure-%s", *testRunIdentifier))
}

// TestAzureCustomImageReferenceProvisioningE2E - a test suite that exercises Azure provider
// by requesting nodes with different combination of container runtime type, container runtime version and custom Image reference.
func TestAzureCustomImageReferenceProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	azureTenantID := os.Getenv("AZURE_E2E_TESTS_TENANT_ID")
	azureSubscriptionID := os.Getenv("AZURE_E2E_TESTS_SUBSCRIPTION_ID")
	azureClientID := os.Getenv("AZURE_E2E_TESTS_CLIENT_ID")
	azureClientSecret := os.Getenv("AZURE_E2E_TESTS_CLIENT_SECRET")
	if len(azureTenantID) == 0 || len(azureSubscriptionID) == 0 || len(azureClientID) == 0 || len(azureClientSecret) == 0 {
		t.Fatal("unable to run the test suite, AZURE_TENANT_ID, AZURE_SUBSCRIPTION_ID, AZURE_CLIENT_ID and AZURE_CLIENT_SECRET environment variables cannot be empty")
	}

	selector := OsSelector("ubuntu")
	// act
	params := []string{
		fmt.Sprintf("<< AZURE_TENANT_ID >>=%s", azureTenantID),
		fmt.Sprintf("<< AZURE_SUBSCRIPTION_ID >>=%s", azureSubscriptionID),
		fmt.Sprintf("<< AZURE_CLIENT_ID >>=%s", azureClientID),
		fmt.Sprintf("<< AZURE_CLIENT_SECRET >>=%s", azureClientSecret),
	}
	runScenarios(t, selector, params, AzureCustomImageReferenceManifest, fmt.Sprintf("azure-%s", *testRunIdentifier))
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
		kubernetesVersion: "1.21.8",
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

	selector := OsSelector("ubuntu", "centos")

	// act
	params := []string{fmt.Sprintf("<< HETZNER_TOKEN >>=%s", hzToken)}
	runScenarios(t, selector, params, HZManifest, fmt.Sprintf("hz-%s", *testRunIdentifier))
}

// TestEquinixMetalProvisioningE2E - a test suite that exercises Equinix Metal provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestEquinixMetalProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	token := os.Getenv("METAL_AUTH_TOKEN")
	if len(token) == 0 {
		t.Fatal("unable to run the test suite, METAL_AUTH_TOKEN environment variable cannot be empty")
	}

	projectID := os.Getenv("METAL_PROJECT_ID")
	if len(projectID) == 0 {
		t.Fatal("unable to run the test suite, METAL_PROJECT_ID environment variable cannot be empty")
	}

	selector := Not(OsSelector("sles", "rhel", "amzn2"))

	// act
	params := []string{
		fmt.Sprintf("<< METAL_AUTH_TOKEN >>=%s", token),
		fmt.Sprintf("<< METAL_PROJECT_ID >>=%s", projectID),
	}
	runScenarios(t, selector, params, EquinixMetalManifest, fmt.Sprintf("equinixmetal-%s", *testRunIdentifier))
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

	selector := Not(OsSelector("sles", "rhel", "flatcar"))

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

	// we're shimming userdata through Linode stackscripts and the stackscript hasn't been verified for use with centos
	selector := OsSelector("ubuntu")

	// act
	params := []string{fmt.Sprintf("<< LINODE_TOKEN >>=%s", linodeToken)}
	runScenarios(t, selector, params, LinodeManifest, fmt.Sprintf("linode-%s", *testRunIdentifier))
}

func getVSphereTestParams(t *testing.T) []string {
	// test data
	vsPassword := os.Getenv("VSPHERE_E2E_PASSWORD")
	vsUsername := os.Getenv("VSPHERE_E2E_USERNAME")
	vsAddress := os.Getenv("VSPHERE_E2E_ADDRESS")

	if vsPassword == "" || vsUsername == "" || vsAddress == "" {
		t.Fatal("unable to run the test suite, VSPHERE_E2E_PASSWORD, VSPHERE_E2E_USERNAME" +
			"or VSPHERE_E2E_ADDRESS environment variables cannot be empty")
	}

	// act
	params := []string{fmt.Sprintf("<< VSPHERE_PASSWORD >>=%s", vsPassword),
		fmt.Sprintf("<< VSPHERE_USERNAME >>=%s", vsUsername),
		fmt.Sprintf("<< VSPHERE_ADDRESS >>=%s", vsAddress),
	}
	return params
}

// TestVsphereProvisioning - a test suite that exercises vsphere provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestVsphereProvisioningE2E(t *testing.T) {
	t.Parallel()

	selector := Not(OsSelector("sles", "amzn2"))
	params := getVSphereTestParams(t)

	runScenarios(t, selector, params, VSPhereManifest, fmt.Sprintf("vs-%s", *testRunIdentifier))
}

// TestVsphereDatastoreClusterProvisioning - is the same as the TestVsphereProvisioning suite but specifies a DatastoreCluster
// instead of the Datastore in the provider specs.
func TestVsphereDatastoreClusterProvisioningE2E(t *testing.T) {
	t.Parallel()

	selector := OsSelector("ubuntu", "centos", "rhel", "flatcar")

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
		osName:            "flatcar",
		containerRuntime:  "docker",
		kubernetesVersion: "1.22.5",
		executor:          verifyCreateAndDelete,
	}

	testScenario(t, scenario, *testRunIdentifier, params, VSPhereResourcePoolManifest, false)
}

// TestScalewayProvisioning - a test suite that exercises scaleway provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
//
// note that tests require the following environment variable:
// - SCW_ACCESS_KEY -> the Scaleway Access Key
// - SCW_SECRET_KEY -> the Scaleway Secret Key
// - SCW_DEFAULT_PROJECT_ID -> the Scaleway Project ID
func TestScalewayProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	scwAccessKey := os.Getenv("SCW_ACCESS_KEY")
	if len(scwAccessKey) == 0 {
		t.Fatal("unable to run the test suite, SCW_E2E_TEST_ACCESS_KEY environment variable cannot be empty")
	}

	scwSecretKey := os.Getenv("SCW_SECRET_KEY")
	if len(scwSecretKey) == 0 {
		t.Fatal("unable to run the test suite, SCW_E2E_TEST_SECRET_KEY environment variable cannot be empty")
	}

	scwProjectID := os.Getenv("SCW_DEFAULT_PROJECT_ID")
	if len(scwProjectID) == 0 {
		t.Fatal("unable to run the test suite, SCW_E2E_TEST_PROJECT_ID environment variable cannot be empty")
	}

	selector := Not(OsSelector("sles", "rhel", "flatcar"))
	// act
	params := []string{
		fmt.Sprintf("<< SCW_ACCESS_KEY >>=%s", scwAccessKey),
		fmt.Sprintf("<< SCW_SECRET_KEY >>=%s", scwSecretKey),
		fmt.Sprintf("<< SCW_DEFAULT_PROJECT_ID >>=%s", scwProjectID),
	}
	runScenarios(t, selector, params, ScalewayManifest, fmt.Sprintf("scw-%s", *testRunIdentifier))
}

func getNutanixTestParams(t *testing.T) []string {
	// test data
	password := os.Getenv("NUTANIX_E2E_PASSWORD")
	username := os.Getenv("NUTANIX_E2E_USERNAME")
	cluster := os.Getenv("NUTANIX_E2E_CLUSTER_NAME")
	project := os.Getenv("NUTANIX_E2E_PROJECT_NAME")
	subnet := os.Getenv("NUTANIX_E2E_SUBNET_NAME")
	endpoint := os.Getenv("NUTANIX_E2E_ENDPOINT")

	if password == "" || username == "" || endpoint == "" || cluster == "" || project == "" || subnet == "" {
		t.Fatal("unable to run the test suite, NUTANIX_E2E_PASSWORD, NUTANIX_E2E_USERNAME, NUTANIX_E2E_CLUSTER_NAME, " +
			"NUTANIX_E2E_ENDPOINT, NUTANIX_E2E_PROJECT_NAME or NUTANIX_E2E_SUBNET_NAME environment variables cannot be empty")
	}

	// a proxy URL will be passed in our e2e test environment so
	// a HTTP proxy can be used to access the Nutanix API in a different
	// network segment.
	proxyURL := os.Getenv("NUTANIX_E2E_PROXY_URL")

	// set up parameters
	params := []string{fmt.Sprintf("<< NUTANIX_PASSWORD >>=%s", password),
		fmt.Sprintf("<< NUTANIX_USERNAME >>=%s", username),
		fmt.Sprintf("<< NUTANIX_ENDPOINT >>=%s", endpoint),
		fmt.Sprintf("<< NUTANIX_CLUSTER >>=%s", cluster),
		fmt.Sprintf("<< NUTANIX_PROJECT >>=%s", project),
		fmt.Sprintf("<< NUTANIX_SUBNET >>=%s", subnet),
		fmt.Sprintf("<< NUTANIX_PROXY_URL >>=%s", proxyURL),
	}
	return params
}

// TestNutanixProvisioningE2E tests provisioning on Nutanix as cloud provider
func TestNutanixProvisioningE2E(t *testing.T) {
	t.Parallel()

	// exclude migrateUID test case because it's a no-op for Nutanix and runs from a different
	// location, thus possibly blocking access a HTTP proxy if it is configured
	selector := And(OsSelector("ubuntu", "centos"), Not(NameSelector("migrateUID")))
	params := getNutanixTestParams(t)
	runScenarios(t, selector, params, nutanixManifest, fmt.Sprintf("nx-%s", *testRunIdentifier))
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
		t.Fatal("unable to run test suite, all of OS_AUTH_URL, OS_USERNAME, OS_PASSOWRD, OS_REGION, and OS_TENANT OS_DOMAIN must be set!")
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
		kubernetesVersion: "1.22.5",
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
		kubernetesVersion: "1.21.8",
		executor:          verifyCreateUpdateAndDelete,
	}
	testScenario(t, scenario, *testRunIdentifier, params, HZManifest, false)
}

func TestAnexiaProvisioningE2E(t *testing.T) {
	t.Parallel()

	token := os.Getenv("ANEXIA_TOKEN")
	if token == "" {
		t.Fatal("unable to run the test suite, ANEXIA_TOKEN environment variable cannot be empty")
	}

	selector := OsSelector("flatcar")
	params := []string{
		fmt.Sprintf("<< ANEXIA_TOKEN >>=%s", token),
	}

	runScenarios(t, selector, params, anexiaManifest, fmt.Sprintf("anexia-%s", *testRunIdentifier))
}
