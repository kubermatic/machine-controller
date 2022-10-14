/*
Copyright 2022 The Machine Controller Authors.

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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/provisioner"

	"sigs.k8s.io/yaml"
)

func newCreateCommand(rootFlags *pflag.FlagSet) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "create",
		Short:         "Create a machine",
		Long:          "",
		Args:          cobra.ExactArgs(0),
		SilenceErrors: true,
		Example:       `machine-provisioner create --machine-config ./machines.yaml`,
		Run: func(_ *cobra.Command, _ []string) {
			err := runCreateMachineCommand(opts.MachineConfig)
			if err != nil {
				logrus.Fatal(err)
			}
		},
	}

	return cmd
}

func runCreateMachineCommand(machineConfigFile string) error {
	logrus.Info("Running command to create machines")

	if len(machineConfigFile) == 0 {
		return errors.New("machine configuration path is empty")
	}

	machineConfig, err := os.ReadFile(machineConfigFile)
	if err != nil {
		return errors.New("failed to read machine configuration")
	}

	machines, err := parseYAMLToObjects(machineConfig)
	if err != nil {
		return err
	}

	out, err := provisioner.CreateMachines(context.Background(), machines)
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(out, "", "	")
	if err != nil {
		return err
	}

	err = os.WriteFile("machines.json", b, 0600)
	if err != nil {
		return err
	}

	logrus.Infof("Create task ran successfully. Output is available in %q.", provisioner.OutputFileName)
	return nil
}

func parseYAMLToObjects(machineByte []byte) ([]clusterv1alpha1.Machine, error) {
	machine := []clusterv1alpha1.Machine{}
	if err := yaml.Unmarshal(machineByte, &machine); err != nil {
		return nil, err
	}

	return machine, nil
}
