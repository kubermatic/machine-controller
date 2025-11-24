/*
Copyright 2025 The Machine Controller Authors.

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

package azure

import (
	"fmt"

	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
	azuretypes "k8c.io/machine-controller/sdk/cloudprovider/azure"
	"k8c.io/machine-controller/sdk/providerconfig"
)

// newCloudProviderSpec creates a cloud provider specification out of the
// given ProviderSpec.
func newCloudProviderSpec(provSpec clusterv1alpha1.ProviderSpec) (*azuretypes.RawConfig, *providerconfig.Config, error) {
	// Retrieve provider configuration from machine specification.
	pConfig, err := providerconfig.GetConfig(provSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot unmarshal machine.spec.providerconfig.value: %w", err)
	}

	if pConfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, fmt.Errorf("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	// Retrieve cloud provider specification from cloud provider specification.
	cpSpec, err := azuretypes.GetConfig(*pConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot unmarshal cloud provider specification: %w", err)
	}

	return cpSpec, pConfig, nil
}
