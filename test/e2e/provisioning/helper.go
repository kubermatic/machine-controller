package provisioning

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var scenarios = []scenario{
	{
		name:              "Ubuntu Docker Kubernetes v1.8.13",
		osName:            "ubuntu",
		containerRuntime:  "docker",
		kubernetesVersion: "1.8.13",
	},
	{
		name:              "Ubuntu Docker Kubernetes v1.9.10",
		osName:            "ubuntu",
		containerRuntime:  "docker",
		kubernetesVersion: "1.9.8",
	},
	{
		name:              "Ubuntu Docker Kubernetes v1.10.6",
		osName:            "ubuntu",
		containerRuntime:  "docker",
		kubernetesVersion: "1.10.5",
	},
	{
		name:              "Ubuntu Docker Kubernetes v1.11.1",
		osName:            "ubuntu",
		containerRuntime:  "docker",
		kubernetesVersion: "1.11.0",
	},
	{
		name:              "CentOS Docker Kubernetes v1.8.13",
		osName:            "centos",
		containerRuntime:  "docker",
		kubernetesVersion: "1.8.13",
	},
	{
		name:              "CentOS Docker Kubernetes v1.9.10",
		osName:            "centos",
		containerRuntime:  "docker",
		kubernetesVersion: "1.9.8",
	},
	{
		name:              "CentOS Docker Kubernetes v1.10.6",
		osName:            "centos",
		containerRuntime:  "docker",
		kubernetesVersion: "1.10.5",
	},
	{
		name:              "CentOS Docker Kubernetes v1.11.1",
		osName:            "centos",
		containerRuntime:  "docker",
		kubernetesVersion: "1.11.0",
	},

	{
		name:              "Coreos Docker Kubernetes v1.8.13",
		osName:            "coreos",
		containerRuntime:  "docker",
		kubernetesVersion: "1.8.13",
	},
	{
		name:              "Coreos Docker Kubernetes v1.9.10",
		osName:            "coreos",
		containerRuntime:  "docker",
		kubernetesVersion: "1.9.8",
	},
	{
		name:              "Coreos Docker Kubernetes v1.10.6",
		osName:            "coreos",
		containerRuntime:  "docker",
		kubernetesVersion: "1.10.5",
	},
	{
		name:              "Coreos Docker Kubernetes v1.11.1",
		osName:            "coreos",
		containerRuntime:  "docker",
		kubernetesVersion: "1.11.0",
	},
}

type scenario struct {
	// name holds short description of the test scenario, it is also used to create machines and nodes names
	// so please don't put "strange" characters there
	name              string
	osName            string
	containerRuntime  string
	kubernetesVersion string
}

type scenarioSelector struct {
	osName                  []string
	containerRuntime        []string
	containerRuntimeVersion []string
}

func doesSenarioSelectorMatch(selector *scenarioSelector, testCase scenario) bool {
	for _, selectorOSName := range selector.osName {
		if testCase.osName == selectorOSName {
			return true
		}
	}

	for _, selectorContainerRuntime := range selector.containerRuntime {
		if testCase.containerRuntime == selectorContainerRuntime {
			return true
		}
	}

	for _, selectorContainerRuntimeVersion := range selector.containerRuntimeVersion {
		if testCase.containerRuntime == selectorContainerRuntimeVersion {
			return true
		}
	}

	return false
}

func runScenarios(st *testing.T, excludeSelector *scenarioSelector, testParams []string, manifestPath string, cloudProvider string) {
	for _, testCase := range scenarios {
		if excludeSelector != nil && doesSenarioSelectorMatch(excludeSelector, testCase) {
			continue
		}

		st.Run(testCase.name, func(it *testing.T) {
			testScenario(it, testCase, cloudProvider, testParams, manifestPath)
		})
	}
}

func testScenario(t *testing.T, testCase scenario, cloudProvider string, testParams []string, manifestPath string) {
	t.Parallel()

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

	gopath := os.Getenv("GOPATH")
	projectDir := filepath.Join(gopath, "src/github.com/kubermatic/machine-controller")

	kubeConfig := filepath.Join(projectDir, ".kubeconfig")

	// the golang test runtime waits for individual subtests to complete before reporting the status.
	// if one of them is blocking/waiting and the global timeout is reached the status will not be reported/visible.
	//
	// we decided to keep this time lower that the global timeout to prevent the following:
	// the global timeout is set to 20 minutes and the verify tool waits up to 60 hours for a machine to show up.
	// thus one faulty scenario prevents from showing the results for the whole group, which is confusing because it looks like all tests are broken.
	err := verify(kubeConfig, manifestPath, scenarioParams, 25*time.Minute)
	if err != nil {
		t.Errorf("verify failed due to error=%v", err)
	}
}
