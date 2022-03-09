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

package controller

import (
	"strconv"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
)

func useExternalCSI(machine *clusterv1alpha1.Machine) (bool, error) {
	kubeletFeatureGates := common.GetKubeletFlags(machine.Annotations)
	val, ok := kubeletFeatureGates[common.ExternalCloudProviderKubeletFlag]
	if !ok {
		return false, nil
	}
	return strconv.ParseBool(val)
}
