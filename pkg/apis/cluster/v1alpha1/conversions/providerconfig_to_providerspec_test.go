package conversions

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	testhelper "github.com/kubermatic/machine-controller/pkg/test"

	"github.com/ghodss/yaml"
)

func Test_Convert_ProviderConfig_To_ProviderSpec(t *testing.T) {
	fixtures, err := ioutil.ReadDir("testdata/clusterv1alpha1machineWithProviderConfig")
	if err != nil {
		t.Fatalf("failed to list fixtures: %v", err)
	}

	for _, fixture := range fixtures {
		fixtureYamlByte, err := ioutil.ReadFile(fmt.Sprintf("testdata/clusterv1alpha1machineWithProviderConfig/%s", fixture.Name()))
		if err != nil {
			t.Errorf("failed to read fixture file %s: %v", fixture.Name(), err)
			continue
		}
		fixtureJSONBytes, err := yaml.YAMLToJSON(fixtureYamlByte)
		if err != nil {
			t.Errorf("failed to convert yaml to json: %v", err)
			continue
		}
		convertedMachine, _, err := Convert_ProviderConfig_To_ProviderSpec(fixtureJSONBytes)
		if err != nil {
			t.Errorf("failed to convert machine from file %s: %v", fixture.Name(), err)
			continue
		}
		convertedMachineJSONBytes, err := json.Marshal(*convertedMachine)
		if err != nil {
			t.Errorf("faile to marshal converted machine %s: %v", convertedMachine.Name, err)
			continue
		}
		convertedMachineYamlBytes, err := yaml.JSONToYAML(convertedMachineJSONBytes)
		if err != nil {
			t.Errorf("failed to convert json to yaml: %v", err)
			continue
		}

		testhelper.CompareOutput(t, fmt.Sprintf("migrated_clusterv1alpha1machineWithProviderConfig/%s", fixture.Name()), string(convertedMachineYamlBytes), *update)
	}
}
