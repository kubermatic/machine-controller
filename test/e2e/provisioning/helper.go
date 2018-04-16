package provisioning

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type scenario struct {
	// name holds short description of the test scenario, it is also used to create machines and nodes names
	// so please don't put "strange" characters there
	name                    string
	osName                  string
	containerRuntime        string
	containerRuntimeVersion string
}

func runScenarios(t *testing.T, testCases []scenario, testParams string, manifestPath string, cloudProvider string) {
	for _, testCase := range testCases {
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
	homeDir := os.Getenv("HOME")
	kubeConfigDir := filepath.Join(homeDir, ".kube/config")
	args := []string{
		"-input",
		manifestPath,
		"-parameters",
		params,
		"-kubeconfig",
		kubeConfigDir,
		"-machineReadyTimeout",
		"40m",
		"-logtostderr",
		"true",
	}

	cmd := exec.Command("../../tools/verify/verify", args...)
	var out bytes.Buffer
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("verify tool failed with the error=%v, output=\n%v", err, out.String())
	}
	return nil
}
