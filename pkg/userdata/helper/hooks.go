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

package helper

import (
	"bytes"
	"fmt"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"io/ioutil"
	"os"
	"text/template"
)

const (
	preHookDownloadBinariesFile = "/hooks/pre-hook-download-binaries.template"
)

type Hooks struct {
	ProviderConfig   *providerconfigtypes.Config
	MachineSpec      clusterv1alpha1.MachineSpec
}

func NewHooks(pconfig *providerconfigtypes.Config, machineSpec clusterv1alpha1.MachineSpec) *Hooks {
	return &Hooks{
		ProviderConfig:   pconfig,
		MachineSpec:      machineSpec,
	}
}

// PreHookDownloadBinariesScript returns the pre hook script for download binaries.
func (h *Hooks) PreHookDownloadBinariesScript() (string, error) {
	if _, err := os.Stat(preHookDownloadBinariesFile); err == nil {

		preHookDownloadBinariesTpl, err := ioutil.ReadFile(preHookDownloadBinariesFile) // just pass the file name
		if err != nil {
			return "", fmt.Errorf("failed to read pre hook template file: %v", err)
		}

		tmpl, err := template.New("pre-hook-download-binaries").Funcs(TxtFuncMap()).Parse(string(preHookDownloadBinariesTpl))
		if err != nil {
			return "", fmt.Errorf("failed to parse download-binaries template: %v", err)
		}

		data := struct {
			ProviderConfig   *providerconfigtypes.Config
			MachineSpec      clusterv1alpha1.MachineSpec
		}{
			ProviderConfig:   h.ProviderConfig,
			MachineSpec:      h.MachineSpec,
		}

		b := &bytes.Buffer{}
		err = tmpl.Execute(b, data)
		if err != nil {
			return "", fmt.Errorf("failed to execute download-binaries template: %v", err)
		}

		return b.String(), nil
	} else {
		return "", nil
	}
}
