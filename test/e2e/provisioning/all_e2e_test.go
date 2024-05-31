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
	"encoding/base64"
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
		klog.Fatalf("Failed to add clusterv1alpha1 to scheme: %v", err)
	}
}

const (
	AWSManifest             = "./testdata/machinedeployment-aws.yaml"
	AWSSpotInstanceManifest = "./testdata/machinedeployment-aws-spot-instances.yaml"
	AWSManifestARM          = "./testdata/machinedeployment-aws-arm-machines.yaml"
	AWSEBSEncryptedManifest = "./testdata/machinedeployment-aws-ebs-encryption-enabled.yaml"
	OSMachineManifest       = "./testdata/machine-openstack.yaml"
	OSManifest              = "./testdata/machinedeployment-openstack.yaml"
	OSManifestProjectAuth   = "./testdata/machinedeployment-openstack-project-auth.yaml"
	OSUpgradeManifest       = "./testdata/machinedeployment-openstack-upgrade.yml"
	invalidMachineManifest  = "./testdata/machine-invalid.yaml"
)

const (
	defaultKubernetesVersion    = "1.28.7"
	awsDefaultKubernetesVersion = "1.26.12"
	defaultContainerRuntime     = "containerd"
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
		t.Fatal("Unable to run test suite, all of OS_AUTH_URL, OS_USERNAME, OS_PASSWORD, OS_REGION, and OS_TENANT OS_DOMAIN must be set!")
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
			containerRuntime:  defaultContainerRuntime,
			kubernetesVersion: versions[0].String(),
			osName:            string(providerconfigtypes.OperatingSystemUbuntu),

			executor: func(kubeConfig, manifestPath string, parameters []string, d time.Duration) error {
				if err := updateMachineControllerForCustomCA(kubeConfig); err != nil {
					return fmt.Errorf("failed to add CA: %w", err)
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
		return fmt.Errorf("Error building kubeconfig: %w", err)
	}

	client, err := ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
	if err != nil {
		return fmt.Errorf("failed to create Client: %w", err)
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
		return fmt.Errorf("failed to create ca-bundle ConfigMap: %w", err)
	}

	// add CA to deployments
	deployments := []string{"machine-controller", "machine-controller-webhook"}
	for _, deployment := range deployments {
		if err := addCAToDeployment(ctx, client, deployment, ns); err != nil {
			return fmt.Errorf("failed to add CA to %s Deployment: %w", deployment, err)
		}
	}

	// wait for deployments to roll out
	for _, deployment := range deployments {
		if err := wait.PollUntilContextTimeout(ctx, 3*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			d := &appsv1.Deployment{}
			key := types.NamespacedName{Namespace: ns, Name: deployment}

			if err := client.Get(ctx, key, d); err != nil {
				return false, fmt.Errorf("failed to get Deployment: %w", err)
			}

			return d.Status.AvailableReplicas > 0, nil
		}); err != nil {
			return fmt.Errorf("%s Deployment never became ready: %w", deployment, err)
		}
	}

	return nil
}

func addCAToDeployment(ctx context.Context, client ctrlruntimeclient.Client, name string, namespace string) error {
	deployment := &appsv1.Deployment{}
	key := types.NamespacedName{Namespace: namespace, Name: name}

	if err := client.Get(ctx, key, deployment); err != nil {
		return fmt.Errorf("failed to get Deployment: %w", err)
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
		t.Fatal("Unable to run kubevirt tests, KUBEVIRT_E2E_TESTS_KUBECONFIG must be set")
	}

	selector := OsSelector("ubuntu", "flatcar")

	params := []string{
		fmt.Sprintf("<< KUBECONFIG_BASE64 >>=%s", safeBase64Encoding(kubevirtKubeconfig)),
	}

	runScenarios(t, selector, params, kubevirtManifest, fmt.Sprintf("kubevirt-%s", *testRunIdentifier))
}

// safeBase64Encoding takes a value and encodes it with base64
// if it is not already encoded.
func safeBase64Encoding(value string) string {
	// If there was no error, the original value was already encoded.
	if _, err := base64.StdEncoding.DecodeString(value); err == nil {
		return value
	}

	return base64.StdEncoding.EncodeToString([]byte(value))
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
		t.Fatal("Unable to run test suite, all of OS_AUTH_URL, OS_USERNAME, OS_PASSWORD, OS_REGION, and OS_TENANT OS_DOMAIN must be set!")
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

	// In-tree cloud provider is not supported from Kubernetes v1.26.
	selector := And(Not(VersionSelector("1.27.11", "1.28.7", "1.29.2")))
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
		t.Fatal("Unable to run test suite, all of OS_AUTH_URL, OS_USERNAME, OS_PASSWORD, OS_REGION, and OS_TENANT OS_DOMAIN must be set!")
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
		containerRuntime:  defaultContainerRuntime,
		kubernetesVersion: defaultKubernetesVersion,
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
		t.Fatal("Unable to run the test suite, DO_E2E_TESTS_TOKEN environment variable cannot be empty")
	}

	selector := OsSelector("ubuntu")

	// act
	params := []string{fmt.Sprintf("<< DIGITALOCEAN_TOKEN >>=%s", doToken)}
	runScenarios(t, selector, params, DOManifest, fmt.Sprintf("do-%s", *testRunIdentifier))
}

// TestAWSProvisioning - a test suite that exercises AWS provider
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestAWSProvisioningE2E(t *testing.T) {
	t.Parallel()

	provisioningUtility := flatcar.Ignition
	// `OPERATING_SYSTEM_MANAGER` will be false when legacy machine-controller userdata should be used for E2E tests.
	if v := os.Getenv("OPERATING_SYSTEM_MANAGER"); v == "false" {
		provisioningUtility = flatcar.CloudInit
	}

	// test data
	awsKeyID := os.Getenv("AWS_E2E_TESTS_KEY_ID")
	awsSecret := os.Getenv("AWS_E2E_TESTS_SECRET")
	if len(awsKeyID) == 0 || len(awsSecret) == 0 {
		t.Fatal("Unable to run the test suite, AWS_E2E_TESTS_KEY_ID or AWS_E2E_TESTS_SECRET environment variables cannot be empty")
	}

	// In-tree cloud provider is not supported from Kubernetes v1.27.
	selector := Not(VersionSelector("1.27.11", "1.28.7", "1.29.2"))

	// act
	params := []string{fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
		fmt.Sprintf("<< PROVISIONING_UTILITY >>=%s", provisioningUtility),
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
		t.Fatal("Unable to run the test suite, environment variables AWS_E2E_TESTS_KEY_ID, AWS_E2E_TESTS_SECRET, AWS_E2E_ASSUME_ROLE_ARN and AWS_E2E_ASSUME_ROLE_EXTERNAL_ID cannot be empty")
	}

	// act
	params := []string{fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
		fmt.Sprintf("<< PROVISIONING_UTILITY >>=%s", flatcar.Ignition),
	}

	scenario := scenario{
		name:              "AWS with AssumeRole",
		osName:            "ubuntu",
		containerRuntime:  defaultContainerRuntime,
		kubernetesVersion: defaultKubernetesVersion,
		executor:          verifyCreateAndDelete,
	}
	testScenario(t, scenario, *testRunIdentifier, params, AWSManifest, false)
}

// TestAWSSpotInstanceProvisioning - a test suite that exercises AWS provider
// by requesting spot nodes with different combination of container runtime type, container runtime version.
func TestAWSSpotInstanceProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	awsKeyID := os.Getenv("AWS_E2E_TESTS_KEY_ID")
	awsSecret := os.Getenv("AWS_E2E_TESTS_SECRET")
	if len(awsKeyID) == 0 || len(awsSecret) == 0 {
		t.Fatal("Unable to run the test suite, AWS_E2E_TESTS_KEY_ID or AWS_E2E_TESTS_SECRET environment variables cannot be empty")
	}
	// Since we are only testing the spot instance functionality, testing it against a single OS is sufficient.
	// In-tree cloud provider is not supported from Kubernetes v1.27.
	selector := And(OsSelector("ubuntu"), Not(VersionSelector("1.27.11", "1.28.7", "1.29.2")))

	// act
	params := []string{fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
		fmt.Sprintf("<< PROVISIONING_UTILITY >>=%s", flatcar.Ignition),
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
		t.Fatal("Unable to run the test suite, AWS_E2E_TESTS_KEY_ID or AWS_E2E_TESTS_SECRET environment variables cannot be empty")
	}
	// In-tree cloud provider is not supported from Kubernetes v1.27.
	selector := And(OsSelector("ubuntu"), Not(VersionSelector("1.27.11", "1.28.7", "1.29.2")))

	// act
	params := []string{fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
		fmt.Sprintf("<< PROVISIONING_UTILITY >>=%s", flatcar.Ignition),
	}
	runScenarios(t, selector, params, AWSManifestARM, fmt.Sprintf("aws-%s", *testRunIdentifier))
}

func TestAWSFlatcarCoreOSCloudInit8ProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	awsKeyID := os.Getenv("AWS_E2E_TESTS_KEY_ID")
	awsSecret := os.Getenv("AWS_E2E_TESTS_SECRET")
	if len(awsKeyID) == 0 || len(awsSecret) == 0 {
		t.Fatal("Unable to run the test suite, AWS_E2E_TESTS_KEY_ID or AWS_E2E_TESTS_SECRET environment variables cannot be empty")
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

// TestAWSEbsEncryptionEnabledProvisioningE2E - a test suite that exercises AWS provider with ebs encryption enabled
// by requesting nodes with different combination of container runtime type, container runtime version and the OS flavour.
func TestAWSEbsEncryptionEnabledProvisioningE2E(t *testing.T) {
	t.Parallel()

	// test data
	awsKeyID := os.Getenv("AWS_E2E_TESTS_KEY_ID")
	awsSecret := os.Getenv("AWS_E2E_TESTS_SECRET")
	if len(awsKeyID) == 0 || len(awsSecret) == 0 {
		t.Fatal("Unable to run the test suite, AWS_E2E_TESTS_KEY_ID or AWS_E2E_TESTS_SECRET environment variables cannot be empty")
	}

	// act
	params := []string{fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s", awsKeyID),
		fmt.Sprintf("<< AWS_SECRET_ACCESS_KEY >>=%s", awsSecret),
	}

	scenario := scenario{
		name:              "AWS with ebs encryption enabled",
		osName:            "ubuntu",
		containerRuntime:  defaultContainerRuntime,
		kubernetesVersion: awsDefaultKubernetesVersion,
		executor:          verifyCreateAndDelete,
	}
	testScenario(t, scenario, fmt.Sprintf("aws-%s", *testRunIdentifier), params, AWSEBSEncryptedManifest, false)
}

// TestUbuntuProvisioningWithUpgradeE2E will create an instance from an old Ubuntu 1604
// image and upgrade it prior to joining the cluster.
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
		t.Fatal("Unable to run test suite, all of OS_AUTH_URL, OS_USERNAME, OS_PASSWORD, OS_REGION, and OS_TENANT OS_DOMAIN must be set!")
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
		containerRuntime:  defaultContainerRuntime,
		kubernetesVersion: defaultKubernetesVersion,
		executor:          verifyCreateAndDelete,
	}

	testScenario(t, scenario, *testRunIdentifier, params, OSUpgradeManifest, false)
}
