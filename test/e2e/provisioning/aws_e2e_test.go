// +build e2e

package provisioning

import (
	"fmt"
	"os"
	"testing"
)

const (
	aws_manifest = "./testdata/machine-aws.yaml"
)

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
	params := fmt.Sprintf("<< AWS_ACCESS_KEY_ID >>=%s,<< AWS_SECRET_ACCESS_KEY >>=%s", awsKeyID, awsSecret)
	runScenarios(t, scenarios, params, aws_manifest, "aws")
}
