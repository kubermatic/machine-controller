package provisioning

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var scenarios = []scenario{
	{
		name:                    "scenario 1 Ubuntu Docker 1.13",
		osName:                  "ubuntu",
		containerRuntime:        "docker",
		containerRuntimeVersion: "1.13",
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

	{
		name:                    "scenario 6 CoreOS Docker 1.13",
		osName:                  "coreos",
		containerRuntime:        "docker",
		containerRuntimeVersion: "1.13",
	},

	{
		name:                    "scenario 7 CoreOS Docker 17.03",
		osName:                  "coreos",
		containerRuntime:        "docker",
		containerRuntimeVersion: "17.03",
	},
}

type scenario struct {
	// name holds short description of the test scenario, it is also used to create machines and nodes names
	// so please don't put "strange" characters there
	name                    string
	osName                  string
	containerRuntime        string
	containerRuntimeVersion string
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

func runScenarios(t *testing.T, excludeSelector *scenarioSelector, testParams string, manifestPath string, cloudProvider string) {
	for _, testCase := range scenarios {
		if excludeSelector != nil && doesSenarioSelectorMatch(excludeSelector, testCase) {
			continue
		}

		testCase := testCase // capture range variable
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			nameSufix := strings.Replace(testCase.name, " ", "-", -1)
			nameSufix = strings.ToLower(nameSufix)
			nameSufix = fmt.Sprintf("%s-%s", cloudProvider, nameSufix)
			machineName := fmt.Sprintf("machine-%s", nameSufix)
			nodeName := fmt.Sprintf("node-%s", nameSufix)
			params := testParams
			params = fmt.Sprintf("%s,<< MACHINE_NAME >>=%s,<< NODE_NAME >>=%s", params, machineName, nodeName)
			params = fmt.Sprintf("%s,<< OS_NAME >>=%s,<< CONTAINER_RUNTIME >>=%s,<< CONTAINER_RUNTIME_VERSION >>=%s", params, testCase.osName, testCase.containerRuntime, testCase.containerRuntimeVersion)

			err := runVerifyTool(manifestPath, params)
			if err != nil {
				t.Errorf("verify tool failed due to %v", err)
			}
		})
	}

}

func runVerifyTool(manifestPath string, params string) error {
	gopath := os.Getenv("GOPATH")
	projectDir := filepath.Join(gopath, "src/github.com/kubermatic/machine-controller")

	kubeConfig := filepath.Join(projectDir, ".kubeconfig")
	args := []string{
		"-input",
		manifestPath,
		"-parameters",
		params,
		"-kubeconfig",
		kubeConfig,
		"-machineReadyTimeout",
		"40m",
		"-logtostderr",
		"true",
	}

	cmd := exec.Command(filepath.Join(projectDir, "test/tools/verify/verify"), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("verify tool failed with the error=%v, output=\n%v", err, string(out))
	}
	return nil
}
