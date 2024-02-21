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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"

	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

var (
	scenarios = buildScenarios()

	versions = []*semver.Version{
		semver.MustParse("v1.26.12"),
		semver.MustParse("v1.27.9"),
		semver.MustParse("v1.28.5"),
		semver.MustParse("v1.29.0"),
	}

	operatingSystems = []providerconfigtypes.OperatingSystem{
		providerconfigtypes.OperatingSystemUbuntu,
		providerconfigtypes.OperatingSystemCentOS,
		providerconfigtypes.OperatingSystemAmazonLinux2,
		providerconfigtypes.OperatingSystemRHEL,
		providerconfigtypes.OperatingSystemFlatcar,
		providerconfigtypes.OperatingSystemRockyLinux,
	}

	openStackImages = map[string]string{
		string(providerconfigtypes.OperatingSystemUbuntu):     "kubermatic-ubuntu",
		string(providerconfigtypes.OperatingSystemCentOS):     "machine-controller-e2e-centos",
		string(providerconfigtypes.OperatingSystemRHEL):       "machine-controller-e2e-rhel-8-5",
		string(providerconfigtypes.OperatingSystemFlatcar):    "kubermatic-e2e-flatcar",
		string(providerconfigtypes.OperatingSystemRockyLinux): "machine-controller-e2e-rockylinux",
	}

	openNebulaImages = map[string]string{
		string(providerconfigtypes.OperatingSystemFlatcar):    "machine-controller-e2e-flatcar",
		string(providerconfigtypes.OperatingSystemRockyLinux): "machine-controller-e2e-rockylinux",
	}

	vSphereOSImageTemplates = map[string]string{
		string(providerconfigtypes.OperatingSystemCentOS):     "kkp-centos-7",
		string(providerconfigtypes.OperatingSystemFlatcar):    "kkp-flatcar-3139.2.0",
		string(providerconfigtypes.OperatingSystemRHEL):       "kkp-rhel-8.6",
		string(providerconfigtypes.OperatingSystemRockyLinux): "kkp-rockylinux-8",
		string(providerconfigtypes.OperatingSystemUbuntu):     "kkp-ubuntu-22.04",
	}

	kubevirtImages = map[string]string{
		string(providerconfigtypes.OperatingSystemCentOS):     "centos",
		string(providerconfigtypes.OperatingSystemFlatcar):    "flatcar",
		string(providerconfigtypes.OperatingSystemRHEL):       "rhel",
		string(providerconfigtypes.OperatingSystemRockyLinux): "rockylinux",
		string(providerconfigtypes.OperatingSystemUbuntu):     "ubuntu-22.04",
	}
)

type scenario struct {
	// name holds short description of the test scenario, it is also used to create machines and nodes names
	// so please don't put "strange" characters there
	name              string
	osName            string
	containerRuntime  string
	kubernetesVersion string
	executor          scenarioExecutor
}

// Selector allows to exclude or include the test scenarios.
type Selector interface {
	// Match returns `true` if the scenario should be run, `false` otherwise.
	Match(testCase scenario) bool
}

// Not returns the negation of the selector.
func Not(s Selector) Selector {
	return &not{s}
}

// Ensures that not implements Selector interface.
var _ Selector = &not{}

type not struct {
	s Selector
}

func (n *not) Match(tc scenario) bool {
	return !n.s.Match(tc)
}

// OsSelector is used to match test scenarios by OS name.
func OsSelector(osName ...string) Selector {
	return &osSelector{osName}
}

// Ensures that osSelector implements Selector interface.
var _ Selector = &osSelector{}

type osSelector struct {
	osName []string
}

func (os *osSelector) Match(testCase scenario) bool {
	for _, selectorOSName := range os.osName {
		if testCase.osName == selectorOSName {
			return true
		}
	}
	return false
}

// And is used to match against two selectors.
func And(s1 Selector, s2 Selector) Selector {
	return &and{s1, s2}
}

type and struct {
	s1 Selector
	s2 Selector
}

var _ Selector = &and{}

func (a *and) Match(tc scenario) bool {
	return a.s1.Match(tc) && a.s2.Match(tc)
}

// NameSelector is used to match against a test case name.
func NameSelector(tcName string) Selector {
	return &name{tcName}
}

type name struct {
	name string
}

var _ Selector = &name{}

func (n *name) Match(tc scenario) bool {
	return tc.name == n.name
}

// VersionSelector is used to match against the kubernetes version used for a test case.
func VersionSelector(v ...string) Selector {
	return &version{v}
}

type version struct {
	versions []string
}

var _ Selector = &version{}

func (v *version) Match(testCase scenario) bool {
	for _, version := range v.versions {
		if testCase.kubernetesVersion == version {
			return true
		}
	}
	return false
}

func runScenarios(st *testing.T, selector Selector, testParams []string, manifestPath string, cloudProvider string) {
	for _, testCase := range scenarios {
		if selector != nil && !selector.Match(testCase) {
			fmt.Printf("Skipping test %s\n", testCase.name)
			continue
		}

		st.Run(testCase.name, func(it *testing.T) {
			testScenario(it, testCase, cloudProvider, testParams, manifestPath, true)
		})
	}
}

// scenarioExecutor represents an executor for a given scenario
// args: kubeConfig, maifestPath, scenarioParams, timeout
type scenarioExecutor func(string, string, []string, time.Duration) error

func testScenario(t *testing.T, testCase scenario, cloudProvider string, testParams []string, manifestPath string, parallelize bool) {
	if parallelize {
		t.Parallel()
	}

	kubernetesCompliantName := fmt.Sprintf("%s-%s", testCase.name, cloudProvider)
	kubernetesCompliantName = strings.Replace(kubernetesCompliantName, " ", "-", -1)
	kubernetesCompliantName = strings.Replace(kubernetesCompliantName, ".", "-", -1)
	kubernetesCompliantName = strings.ToLower(kubernetesCompliantName)

	scenarioParams := append([]string(nil), testParams...)
	scenarioParams = append(scenarioParams, fmt.Sprintf("<< MACHINE_NAME >>=%s", kubernetesCompliantName))
	scenarioParams = append(scenarioParams, fmt.Sprintf("<< OS_NAME >>=%s", testCase.osName))
	scenarioParams = append(scenarioParams, fmt.Sprintf("<< CONTAINER_RUNTIME >>=%s", testCase.containerRuntime))
	scenarioParams = append(scenarioParams, fmt.Sprintf("<< KUBERNETES_VERSION >>=%s", testCase.kubernetesVersion))
	scenarioParams = append(scenarioParams, fmt.Sprintf("<< YOUR_PUBLIC_KEY >>=%s", os.Getenv("E2E_SSH_PUBKEY")))

	if testCase.osName == string(providerconfigtypes.OperatingSystemRHEL) {
		rhelSubscriptionManagerUser := os.Getenv("RHEL_SUBSCRIPTION_MANAGER_USER")
		rhelSubscriptionManagerPassword := os.Getenv("RHEL_SUBSCRIPTION_MANAGER_PASSWORD")
		rhsmOfflineToken := os.Getenv("REDHAT_SUBSCRIPTIONS_OFFLINE_TOKEN")

		if rhelSubscriptionManagerUser == "" || rhelSubscriptionManagerPassword == "" || rhsmOfflineToken == "" {
			t.Fatal("Unable to run e2e tests, RHEL_SUBSCRIPTION_MANAGER_USER, RHEL_SUBSCRIPTION_MANAGER_PASSWORD, and " +
				"REDHAT_SUBSCRIPTIONS_OFFLINE_TOKEN must be set when rhel is used as an os")
		}

		scenarioParams = append(scenarioParams, fmt.Sprintf("<< RHEL_SUBSCRIPTION_MANAGER_USER >>=%s", rhelSubscriptionManagerUser))
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< RHEL_SUBSCRIPTION_MANAGER_PASSWORD >>=%s", rhelSubscriptionManagerPassword))
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< REDHAT_SUBSCRIPTIONS_OFFLINE_TOKEN >>=%s", rhsmOfflineToken))
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< DISK_SIZE >>=%v", 50))
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< OS_DISK_SIZE >>=%v", 0))
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< DATA_DISK_SIZE >>=%v", 0))
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< CUSTOM-IMAGE >>=%v", "rhel-8-1-custom"))
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< MAX_PRICE >>=%s", "0.08"))
	} else {
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< OS_DISK_SIZE >>=%v", 30))
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< DATA_DISK_SIZE >>=%v", 30))
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< DISK_SIZE >>=%v", 25))
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< CUSTOM-IMAGE >>=%v", ""))
		scenarioParams = append(scenarioParams, fmt.Sprintf("<< MAX_PRICE >>=%s", "0.023"))
	}

	if strings.Contains(cloudProvider, string(providerconfigtypes.CloudProviderEquinixMetal)) {
		switch testCase.osName {
		case string(providerconfigtypes.OperatingSystemCentOS):
			scenarioParams = append(scenarioParams, fmt.Sprintf("<< INSTANCE_TYPE >>=%s", "m3.small.x86"))
			scenarioParams = append(scenarioParams, fmt.Sprintf("<< METRO_CODE >>=%s", "AM"))
		case string(providerconfigtypes.OperatingSystemFlatcar):
			scenarioParams = append(scenarioParams, fmt.Sprintf("<< INSTANCE_TYPE >>=%s", "c3.small.x86"))
			scenarioParams = append(scenarioParams, fmt.Sprintf("<< METRO_CODE >>=%s", "NY"))
		case string(providerconfigtypes.OperatingSystemRockyLinux):
			scenarioParams = append(scenarioParams, fmt.Sprintf("<< INSTANCE_TYPE >>=%s", "m3.small.x86"))
			scenarioParams = append(scenarioParams, fmt.Sprintf("<< METRO_CODE >>=%s", "AM"))
		case string(providerconfigtypes.OperatingSystemUbuntu):
			scenarioParams = append(scenarioParams, fmt.Sprintf("<< INSTANCE_TYPE >>=%s", "m3.small.x86"))
			scenarioParams = append(scenarioParams, fmt.Sprintf("<< METRO_CODE >>=%s", "TY"))
		}
	}

	// only used by assume role scenario, otherwise empty (disabled)
	scenarioParams = append(scenarioParams, fmt.Sprintf("<< AWS_ASSUME_ROLE_ARN >>=%s", os.Getenv("AWS_ASSUME_ROLE_ARN")))
	scenarioParams = append(scenarioParams, fmt.Sprintf("<< AWS_ASSUME_ROLE_EXTERNAL_ID >>=%s", os.Getenv("AWS_ASSUME_ROLE_EXTERNAL_ID")))

	// only used by OpenStack scenarios
	scenarioParams = append(scenarioParams, fmt.Sprintf("<< OS_IMAGE >>=%s", openStackImages[testCase.osName]))

	// only used by OpenNebula scenarios
	scenarioParams = append(scenarioParams, fmt.Sprintf("<< ONE_IMAGE >>=%s", openNebulaImages[testCase.osName]))

	// only use by vSphere scenarios
	scenarioParams = append(scenarioParams, fmt.Sprintf("<< OS_Image_Template >>=%s", vSphereOSImageTemplates[testCase.osName]))

	// only use by KubeVirt scenarios
	scenarioParams = append(scenarioParams, fmt.Sprintf("<< KUBEVIRT_OS_IMAGE >>=%s", kubevirtImages[testCase.osName]))

	// default kubeconfig to the hardcoded path at which `make e2e-cluster` creates its new kubeconfig
	gopath := os.Getenv("GOPATH")
	projectDir := filepath.Join(gopath, "src/github.com/kubermatic/machine-controller")
	kubeConfig := filepath.Join(projectDir, ".kubeconfig")
	_, err := os.Stat(kubeConfig)
	if err != nil {
		if os.IsNotExist(err) {
			// it doesn't exist, fall back to $KUBECONFIG
			kubeConfig = os.Getenv("KUBECONFIG")
		} else {
			t.Fatal(err)
		}
	}

	// the golang test runtime waits for individual subtests to complete before reporting the status.
	// if one of them is blocking/waiting and the global timeout is reached the status will not be reported/visible.
	//
	// we decided to keep this time lower that the global timeout to prevent the following:
	// the global timeout is set to 20 minutes and the verify tool waits up to 60 hours for a machine to show up.
	// thus one faulty scenario prevents from showing the results for the whole group, which is confusing because it looks like all tests are broken.
	if err := testCase.executor(kubeConfig, manifestPath, scenarioParams, 35*time.Minute); err != nil {
		t.Errorf("verify failed due to error=%v", err)
	}
}

func buildScenarios() []scenario {
	var all []scenario
	for _, version := range versions {
		for _, operatingSystem := range operatingSystems {
			s := scenario{
				name:              fmt.Sprintf("%s-%s", operatingSystem, version),
				containerRuntime:  "containerd",
				kubernetesVersion: version.String(),
				osName:            string(operatingSystem),
				executor:          verifyCreateAndDelete,
			}
			all = append(all, s)
		}
	}

	all = append(all, scenario{
		name:             "migrateUID",
		containerRuntime: "containerd",
		osName:           "ubuntu",
		executor:         verifyMigrateUID,
	})
	return all
}
