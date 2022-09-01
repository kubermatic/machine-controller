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
	"bytes"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/ghodss/yaml"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"

	kyaml "k8s.io/apimachinery/pkg/util/yaml"
)

var update = flag.Bool("update", false, "update .testdata files")

func getMachinesV1Alpha1TestMachines() (machines []machinesv1alpha1.Machine, err error) {
	files, err := os.ReadDir("testdata/machinesv1alpha1machine")
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		newMachine := &machinesv1alpha1.Machine{}
		fileContent, err := os.ReadFile(fmt.Sprintf("testdata/machinesv1alpha1machine/%s", file.Name()))
		if err != nil {
			return nil, err
		}
		fileReader := bytes.NewReader(fileContent)
		fileDecoder := kyaml.NewYAMLToJSONDecoder(fileReader)
		if err = fileDecoder.Decode(newMachine); err != nil {
			return nil, err
		}
		machines = append(machines, *newMachine)
	}
	return machines, nil
}

func TestMigratingMachine(t *testing.T) {
	machines, err := getMachinesV1Alpha1TestMachines()
	if err != nil {
		t.Fatalf("Error getting machinesv1alpha1 machines: %v", err)
	}
	for _, inMachine := range machines {
		outMachine := clusterv1alpha1.Machine{}
		err := Convert_MachinesV1alpha1Machine_To_ClusterV1alpha1Machine(&inMachine, &outMachine)
		if err != nil {
			t.Errorf("Failed to migrate machine: %v", err)
		}
		fixtureFilePath := fmt.Sprintf("testdata/migrated_clusterv1alpha1machine/%s.yaml", outMachine.Name)
		outMachineRaw, err := yaml.Marshal(outMachine)
		if err != nil {
			t.Errorf("Failed to marshal machine: %v", err)
		}
		if *update {
			if err = os.WriteFile(fixtureFilePath, outMachineRaw, 0644); err != nil {
				t.Fatalf("Failed to write updated test fixture: %v", err)
			}
		}
		expected, err := os.ReadFile(fixtureFilePath)
		if err != nil {
			t.Fatalf("Failed to read fixture: %v", err)
		}
		if string(outMachineRaw) != string(expected) {
			t.Errorf("Converted machine did not mach fixture: converted:\n%s\nfixture:\n%s",
				string(outMachineRaw), string(expected))
		}
	}
}
