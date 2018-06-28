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
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
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

	for _, inMachine := range machines {
		outMachine := clusterv1alpha1.Machine{}
		err := Convert_v1alpha1_DownStreamMachine_To_v1alpha1_ClusterMachine(&inMachine, &outMachine)
		if err != nil {
			t.Errorf("Failed to migrate machine: %v", err)
		}
		fixtureFilePath := fmt.Sprintf("testdata/migrated/%s.yaml", outMachine.Name)
		outMachineRaw, err := yaml.Marshal(outMachine)
		if err != nil {
			t.Errorf("Failed to marshal machine: %v", err)
		}
		if *update {
			if err = ioutil.WriteFile(fixtureFilePath, outMachineRaw, 0644); err != nil {
				t.Fatalf("Failed to write updated test fixture: %v", err)
			}
		}
		expected, err := ioutil.ReadFile(fixtureFilePath)
		if err != nil {
			t.Fatalf("Failed to read fixture: %v", err)
		}
		if string(outMachineRaw) != string(expected) {
			t.Errorf("Converted machine did not mach fixture: converted:\n%s\nfixture:\n%s",
				string(outMachineRaw), string(expected))
		}
	}
}
