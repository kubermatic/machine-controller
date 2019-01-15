package conversions

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
)

func Test_Convert_ProviderConfig_To_ProviderSpec(t *testing.T) {
	fixtures, err := ioutil.ReadDir("testdata/clusterv1alpha1machineWithProviderConfig")
	if err != nil {
		t.Fatalf("failed to list fixtures: %v", err)
	}

	for _, fixture := range fixtures {
		bt, err := ioutil.ReadFile(fmt.Sprintf("testdata/clusterv1alpha1machineWithProviderConfig/%s", fixture.Name()))
		if err != nil {
			t.Errorf("failed to read fixture file %s: %v", fixture.Name(), err)
			continue
		}
		convertedMachine, err := Convert_ProviderConfig_To_ProviderSpec(bt)
		if err != nil {
			t.Errorf("failed to convert machine from file %s: %v", fixture.Name(), err)
			continue
		}
		convertedMachineBytes, err := json.Marshal(*convertedMachine)
		if err != nil {
			t.Errorf("faile to marshal converted machine %s: %v", convertedMachine.Name, err)
			continue
		}

		resultFixturePath := fmt.Sprintf("testdata/migrated_clusterv1alpha1machineWithProviderConfig/%s", fixture.Name())
		if *update {
			if err := ioutil.WriteFile(resultFixturePath, convertedMachineBytes, 0644); err != nil {
				t.Errorf("failed to update fixture for machine %s: %v", convertedMachine.Name, err)
				continue
			}
		}

		resultFixtureContent, err := ioutil.ReadFile(resultFixturePath)
		if err != nil {
			t.Errorf("failed to read result fixture for machine %s: %v", convertedMachine.Name, err)
			continue
		}

		if string(convertedMachineBytes) != string(resultFixtureContent) {
			t.Errorf("Converted Machine does not match fixture, converted machine:\n---\n%s\n---\nFixture:\n---\n%s\n---", string(convertedMachineBytes), string(resultFixtureContent))
		}
	}
}
