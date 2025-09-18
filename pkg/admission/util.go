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

package admission

import (
	"encoding/json"
	"fmt"

	vcdtypes "k8c.io/machine-controller/sdk/cloudprovider/vmwareclouddirector"
	providerconfigtypes "k8c.io/machine-controller/sdk/providerconfig"
)

const cloudProviderPacket = "packet"

func migrateToEquinixMetal(providerConfig *providerconfigtypes.Config) (err error) {
	providerConfig.CloudProvider = providerconfigtypes.CloudProviderEquinixMetal

	// Field .spec.providerSpec.cloudProviderSpec.apiKey has been replaced with .spec.providerSpec.cloudProviderSpec.token
	// We first need to perform in-place replacement for this field
	rawConfig := map[string]interface{}{}
	if err := json.Unmarshal(providerConfig.CloudProviderSpec.Raw, &rawConfig); err != nil {
		return fmt.Errorf("failed to unmarshal providerConfig.CloudProviderSpec.Raw: %w", err)
	}
	// NB: We have to set the token only if apiKey existed, otherwise, migrated
	// machines will not create at all (authentication errors).
	apiKey, ok := rawConfig["apiKey"]
	if ok {
		rawConfig["token"] = apiKey
		delete(rawConfig, "apiKey")
	}

	// Update original object
	providerConfig.CloudProviderSpec.Raw, err = json.Marshal(rawConfig)
	if err != nil {
		return fmt.Errorf("failed to json marshal providerConfig.CloudProviderSpec.Raw: %w", err)
	}
	return nil
}

func migrateVMwareCloudDirector(providerConfig *providerconfigtypes.Config) (err error) {
	config, err := vcdtypes.GetConfig(*providerConfig)
	if err != nil {
		return fmt.Errorf("failed to get vcd config: %w", err)

	}

	if config.Network.Value != "" {
		config.Networks = append([]providerconfigtypes.ConfigVarString{config.Network}, config.Networks...)
		config.Network.Value = ""
		p := &providerconfigtypes.ConfigVarString{Value: ""}
		config.Network = *p
	}

	config.Networks = Deduplicate(config.Networks)

	cloudProviderSpecRaw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal cloudProviderConfig: %w", err)
	}

	providerConfig.CloudProviderSpec.Raw = cloudProviderSpecRaw
	return nil
}

func Deduplicate[T comparable](slice []T) []T {
	seen := make(map[T]struct{})
	result := []T{}

	for _, val := range slice {
		if _, exists := seen[val]; !exists {
			seen[val] = struct{}{}
			result = append(result, val)
		}
	}

	return result
}
