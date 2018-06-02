package migrate

import (
	"bytes"
	"io/ioutil"
	"testing"

	machinev1alpha1downstream "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"

	"k8s.io/apimachinery/pkg/util/yaml"
)

func getDownstreamTestMachines() (machines []machinev1alpha1downstream.Machine, err error) {
	files, err := ioutil.ReadDir("testdata/downstreammachines")
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		newMachine := &machinev1alpha1downstream.Machine{}
		fileContent, err := ioutil.ReadFile(file.Name())
		if err != nil {
			return nil, err
		}
		fileReader := bytes.NewReader(fileContent)
		fileDecoder := yaml.NewYAMLToJSONDecoder(fileReader)
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
		_, err := migrateMachine(machine)
		if err != nil {
			t.Errorf("Failed to migrate machine: %v")
		}
	}
}
