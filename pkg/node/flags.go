/*
Copyright 2021 The Machine Controller Authors.

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

package node

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"
)

func NewFlags(flagset *flag.FlagSet) *Flags {
	settings := Flags{
		FlagSet: flagset,
	}

	settings.BoolVar(&settings.externalCloudProvider, "external-cloud-provider", false, "[DEPRECATED replaced by -node-external-cloud-provider] when set, kubelets will receive --cloud-provider=external flag")
	settings.BoolVar(&settings.externalCloudProvider, "node-external-cloud-provider", false, "when set, kubelets will receive --cloud-provider=external flag")
	settings.StringVar(&settings.kubeletFeatureGates, "node-kubelet-feature-gates", "RotateKubeletServerCertificate=true", "Feature gates to set on the kubelet")

	return &settings
}

type Flags struct {
	externalCloudProvider bool
	kubeletFeatureGates   string

	*flag.FlagSet
}

func (flags *Flags) UpdateNodeSettings(ns *machinecontroller.NodeSettings) error {
	kubeletFeatureGates, err := parseKubeletFeatureGates(flags.kubeletFeatureGates)
	if err != nil {
		return fmt.Errorf("invalid kubelet feature gates specified: %w", err)
	}

	ns.KubeletFeatureGates = kubeletFeatureGates
	ns.ExternalCloudProvider = flags.externalCloudProvider

	return nil
}

func parseKubeletFeatureGates(s string) (map[string]bool, error) {
	featureGates := map[string]bool{}
	sFeatureGates := strings.Split(s, ",")

	for _, featureGate := range sFeatureGates {
		sFeatureGate := strings.Split(featureGate, "=")
		if len(sFeatureGate) != 2 {
			return nil, fmt.Errorf("invalid kubelet feature gate: %q", featureGate)
		}

		featureGateEnabled, err := strconv.ParseBool(sFeatureGate[1])
		if err != nil {
			return nil, fmt.Errorf("failed to parse kubelet feature gate: %q", featureGate)
		}

		featureGates[sFeatureGate[0]] = featureGateEnabled
	}

	if _, ok := featureGates["RotateKubeletServerCertificate"]; !ok {
		featureGates["RotateKubeletServerCertificate"] = true
	}

	return featureGates, nil
}
