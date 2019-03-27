//
// UserData operating system configurations.
//

package os

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime"
)

// CentOSConfig contains os configuration for CentOS.
type CentOSConfig struct {
	DistUpgradeOnBoot bool `json:"distUpgradeOnBoot"`
}

// LoadCentOSConfig retrieves the CentOS configuration from raw data.
func LoadCentOSConfig(r runtime.RawExtension) (*CentOSConfig, error) {
	cfg := CentOSConfig{}
	if len(r.Raw) == 0 {
		return &cfg, nil
	}
	if err := json.Unmarshal(r.Raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// CoreOSConfig contains os configuration for CoreOS.
type CoreOSConfig struct {
	DisableAutoUpdate bool `json:"disableAutoUpdate"`
}

// LoadCoreOSConfig retrieves the CoreOS configuration from raw data.
func LoadCoreOSConfig(r runtime.RawExtension) (*CoreOSConfig, error) {
	cfg := CoreOSConfig{}
	if len(r.Raw) == 0 {
		return &cfg, nil
	}
	if err := json.Unmarshal(r.Raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UbuntuConfig contains os configuration for Ubuntu.
type UbuntuConfig struct {
	DistUpgradeOnBoot bool `json:"distUpgradeOnBoot"`
}

// LoadUbuntuConfig retrieves the Ubuntu configuration from raw data.
func LoadUbuntuConfig(r runtime.RawExtension) (*UbuntuConfig, error) {
	cfg := UbuntuConfig{}
	if len(r.Raw) == 0 {
		return &cfg, nil
	}
	if err := json.Unmarshal(r.Raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
