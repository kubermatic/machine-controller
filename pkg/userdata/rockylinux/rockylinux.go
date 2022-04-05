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

package rockylinux

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime"
)

// Config contains specific configuration for RockyLinux.
type Config struct {
	DistUpgradeOnBoot bool `json:"distUpgradeOnBoot"`
}

func DefaultConfig(operatingSystemSpec runtime.RawExtension) runtime.RawExtension {
	if operatingSystemSpec.Raw == nil {
		operatingSystemSpec.Raw, _ = json.Marshal(Config{})
	}

	return operatingSystemSpec
}

// LoadConfig retrieves the RockyLinux configuration from raw data.
func LoadConfig(r runtime.RawExtension) (*Config, error) {
	r = DefaultConfig(r)
	cfg := Config{}

	if err := json.Unmarshal(r.Raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Spec return the configuration as raw data.
func (cfg *Config) Spec() (*runtime.RawExtension, error) {
	ext := &runtime.RawExtension{}
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	ext.Raw = b
	return ext, nil
}
