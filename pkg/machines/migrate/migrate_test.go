package migrate

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/ghodss/yaml"

	machinev1alpha1downstream "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"

	kyaml "k8s.io/apimachinery/pkg/util/yaml"
)

var update = flag.Bool("update", false, "update .golden files")

func getDownstreamTestMachines() (machines []machinev1alpha1downstream.Machine, err error) {
	files, err := ioutil.ReadDir("testdata/downstreammachines")
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		newMachine := &machinev1alpha1downstream.Machine{}
		fileContent, err := ioutil.ReadFile(fmt.Sprintf("testdata/downstreammachines/%s", file.Name()))
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
	machines, err := getDownstreamTestMachines()
	if err != nil {
		t.Fatalf("Error getting downstream machines: %v", err)
	}

	for _, machine := range machines {
		machine, err := migrateMachine(machine)
		fixtureFilePath := fmt.Sprintf("testdata/migrated/%s.yaml", machine.Name)
		if err != nil {
			t.Errorf("Failed to migrate machine: %v", err)
		}
		machineRaw, err := yaml.Marshal(machine)
		if err != nil {
			t.Errorf("Failed to marshal machine: %v", err)
		}
		if *update {
			if err = ioutil.WriteFile(fixtureFilePath, machineRaw, 0644); err != nil {
				t.Fatalf("Failed to write updated test fixture: %v", err)
			}
		}
		expected, err := ioutil.ReadFile(fixtureFilePath)
		if err != nil {
			t.Fatalf("Failed to read fixture: %v", err)
		}
		if string(machineRaw) != string(expected) {
			t.Errorf("Converted machine did not mach fixture: converted:\n%s\nfixture:\n%s",
				string(machineRaw), string(expected))
		}
	}
}
