/*
Copyright 2020 The Machine Controller Authors.

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

package containerruntime

import (
	"github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

const (
	containerdName = "containerd"
)

type Engine interface {
	KubeletFlags() []string
	ScriptFor(os types.OperatingSystem) (string, error)
	ConfigFileName() string
	Config() (string, error)
	AuthConfigFileName() string
	AuthConfig() (string, error)
	String() string
}

type Opt func(*Config)

func withInsecureRegistries(registries []string) Opt {
	return func(cfg *Config) {
		cfg.InsecureRegistries = registries
	}
}

func withRegistryMirrors(mirrors map[string][]string) Opt {
	return func(cfg *Config) {
		cfg.RegistryMirrors = mirrors
	}
}

func withSandboxImage(image string) Opt {
	return func(cfg *Config) {
		cfg.SandboxImage = image
	}
}

func withContainerdVersion(version string) Opt {
	return func(cfg *Config) {
		cfg.ContainerdVersion = version
	}
}

func get(_ string, opts ...Opt) Config {
	cfg := Config{}
	cfg.Containerd = &Containerd{}

	for _, o := range opts {
		o(&cfg)
	}

	return cfg
}

type Config struct {
	Containerd           *Containerd           `json:",omitempty"`
	InsecureRegistries   []string              `json:",omitempty"`
	RegistryMirrors      map[string][]string   `json:",omitempty"`
	RegistryCredentials  map[string]AuthConfig `json:",omitempty"`
	SandboxImage         string                `json:",omitempty"`
	ContainerLogMaxFiles string                `json:",omitempty"`
	ContainerLogMaxSize  string                `json:",omitempty"`
	ContainerdVersion    string                `json:",omitempty"`
}

// AuthConfig is a COPY of github.com/containerd/containerd/pkg/cri/config.AuthConfig.
// AuthConfig contains the config related to authentication to a specific registry.
type AuthConfig struct {
	// Username is the username to login the registry.
	Username string `toml:"username,omitempty" json:"username,omitempty"`
	// Password is the password to login the registry.
	Password string `toml:"password,omitempty" json:"password,omitempty"`
	// Auth is a base64 encoded string from the concatenation of the username,
	// a colon, and the password.
	Auth string `toml:"auth,omitempty" json:"auth,omitempty"`
	// IdentityToken is used to authenticate the user and get
	// an access token for the registry.
	IdentityToken string `toml:"identitytoken,omitempty" json:"identitytoken,omitempty"`
}

func (cfg Config) String() string {
	return containerdName
}

func (cfg Config) Engine() Engine {
	containerd := &Containerd{
		insecureRegistries:  cfg.InsecureRegistries,
		registryMirrors:     cfg.RegistryMirrors,
		sandboxImage:        cfg.SandboxImage,
		registryCredentials: cfg.RegistryCredentials,
		version:             cfg.ContainerdVersion,
	}
	return containerd
}
