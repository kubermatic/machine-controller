package provisioning

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type scenarios struct {
	// name holds short description of the test scenario, it is also used to create machines and nodes names
	// so please don't put "strange" characters there
	name                    string
	osName                  string
	containerRuntime        string
	containerRuntimeVersion string
	// if the -short flag was provided and this variable is set
	// the test scenario will be run otherwise it will be skipped.
	short bool
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
