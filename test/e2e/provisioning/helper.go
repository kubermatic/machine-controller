package provisioning

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

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
