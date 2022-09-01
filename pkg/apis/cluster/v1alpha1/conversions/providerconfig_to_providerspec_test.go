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

package conversions

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/ghodss/yaml"

	testhelper "github.com/kubermatic/machine-controller/pkg/test"
)

func Test_Convert_MachineDeployment_ProviderConfig_To_ProviderSpec(t *testing.T) {
	fixtures, err := os.ReadDir("testdata/clusterv1alpha1machineDeploymentWithProviderConfig")
	if err != nil {
		t.Fatalf("failed to list fixtures: %v", err)
	}

	for _, fixture := range fixtures {
		fixtureYamlByte, err := os.ReadFile(fmt.Sprintf("testdata/clusterv1alpha1machineDeploymentWithProviderConfig/%s", fixture.Name()))
		if err != nil {
			t.Errorf("failed to read fixture file %s: %v", fixture.Name(), err)
			continue
		}
		fixtureJSONBytes, err := yaml.YAMLToJSON(fixtureYamlByte)
		if err != nil {
			t.Errorf("failed to convert yaml to json: %v", err)
			continue
		}
		convertedMachineDeployment, wasConverted, err := Convert_MachineDeployment_ProviderConfig_To_ProviderSpec(fixtureJSONBytes)
		if err != nil {
			t.Errorf("failed to convert machineDeployment from file %s: %v", fixture.Name(), err)
			continue
		}
		if !wasConverted {
			t.Errorf("expected wasConverted to be true but was %t", wasConverted)
		}
		convertedMachineDeploymentJSONBytes, err := json.Marshal(*convertedMachineDeployment)
		if err != nil {
			t.Errorf("failed to marshal converted machineDeployment %s: %v", convertedMachineDeployment.Name, err)
			continue
		}
		convertedMachineDeploymentYamlBytes, err := yaml.JSONToYAML(convertedMachineDeploymentJSONBytes)
		if err != nil {
			t.Errorf("failed to convert json to yaml: %v", err)
			continue
		}
		testhelper.CompareOutput(t, fmt.Sprintf("migrated_clusterv1alpha1machineDeploymentWithProviderConfig/%s", fixture.Name()), string(convertedMachineDeploymentYamlBytes), *update)
	}
}

func Test_Convert_MachineSet_ProviderConfig_To_ProviderSpec(t *testing.T) {
	fixtures, err := os.ReadDir("testdata/clusterv1alpha1machineSetWithProviderConfig")
	if err != nil {
		t.Fatalf("failed to list fixtures: %v", err)
	}

	for _, fixture := range fixtures {
		fixtureYamlByte, err := os.ReadFile(fmt.Sprintf("testdata/clusterv1alpha1machineSetWithProviderConfig/%s", fixture.Name()))
		if err != nil {
			t.Errorf("failed to read fixture file %s: %v", fixture.Name(), err)
			continue
		}
		fixtureJSONBytes, err := yaml.YAMLToJSON(fixtureYamlByte)
		if err != nil {
			t.Errorf("failed to convert yaml to json: %v", err)
			continue
		}
		convertedMachineSet, wasConverted, err := Convert_MachineSet_ProviderConfig_To_ProviderSpec(fixtureJSONBytes)
		if err != nil {
			t.Errorf("failed to convert machineSet from file %s: %v", fixture.Name(), err)
			continue
		}
		if !wasConverted {
			t.Errorf("expected wasConverted to be true but was %t", wasConverted)
		}

		convertedMachineSetJSONBytes, err := json.Marshal(*convertedMachineSet)
		if err != nil {
			t.Errorf("failed to marshal converted machineSet %s: %v", convertedMachineSet.Name, err)
			continue
		}
		convertedMachineSetYamlBytes, err := yaml.JSONToYAML(convertedMachineSetJSONBytes)
		if err != nil {
			t.Errorf("failed to convert json to yaml: %v", err)
			continue
		}
		testhelper.CompareOutput(t, fmt.Sprintf("migrated_clusterv1alpha1machineSetWithProviderConfig/%s", fixture.Name()), string(convertedMachineSetYamlBytes), *update)
	}
}

func Test_Convert_Machine_ProviderConfig_To_ProviderSpec(t *testing.T) {
	fixtures, err := os.ReadDir("testdata/clusterv1alpha1machineWithProviderConfig")
	if err != nil {
		t.Fatalf("failed to list fixtures: %v", err)
	}

	for _, fixture := range fixtures {
		fixtureYamlByte, err := os.ReadFile(fmt.Sprintf("testdata/clusterv1alpha1machineWithProviderConfig/%s", fixture.Name()))
		if err != nil {
			t.Errorf("failed to read fixture file %s: %v", fixture.Name(), err)
			continue
		}
		fixtureJSONBytes, err := yaml.YAMLToJSON(fixtureYamlByte)
		if err != nil {
			t.Errorf("failed to convert yaml to json: %v", err)
			continue
		}
		convertedMachine, wasConverted, err := Convert_Machine_ProviderConfig_To_ProviderSpec(fixtureJSONBytes)
		if err != nil {
			t.Errorf("failed to convert machine from file %s: %v", fixture.Name(), err)
			continue
		}
		if !wasConverted {
			t.Errorf("expected wasConverted to be true but was %t", wasConverted)
		}
		convertedMachineJSONBytes, err := json.Marshal(*convertedMachine)
		if err != nil {
			t.Errorf("failed to marshal converted machine %s: %v", convertedMachine.Name, err)
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
