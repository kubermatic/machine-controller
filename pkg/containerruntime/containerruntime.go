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
	"github.com/Masterminds/semver/v3"

	"github.com/kubermatic/machine-controller/pkg/providerconfig/types"
)

const (
	dockerName     = "docker"
	containerdName = "containerd"
)

type Engine interface {
	KubeletFlags() []string
	ScriptFor(os types.OperatingSystem) (string, error)
	ConfigFileName() string
	Config() (string, error)
	String() string
}

type Opt func(*Config)

func WithInsecureRegistries(registries []string) Opt {
	return func(cfg *Config) {
		cfg.InsecureRegistries = registries
	}
}

func WithRegistryMirrors(mirrors map[string][]string) Opt {
	return func(cfg *Config) {
		cfg.RegistryMirrors = mirrors
	}
}

func WithRegistryCredentials(auth map[string]AuthConfig) Opt {
	return func(cfg *Config) {
		cfg.RegistryCredentials = auth
	}
}

func WithSandboxImage(image string) Opt {
	return func(cfg *Config) {
		cfg.SandboxImage = image
	}
}

func Get(containerRuntimeName string, opts ...Opt) Config {
	cfg := Config{}

	switch containerRuntimeName {
	case dockerName:
		cfg.Docker = &Docker{}
		cfg.Containerd = nil
	case containerdName:
		cfg.Containerd = &Containerd{}
		cfg.Docker = nil
	default:
		cfg.Docker = &Docker{}
		cfg.Containerd = nil
	}

	for _, o := range opts {
		o(&cfg)
	}

	return cfg
}

type Config struct {
	Docker              *Docker               `json:",omitempty"`
	Containerd          *Containerd           `json:",omitempty"`
	InsecureRegistries  []string              `json:",omitempty"`
	RegistryMirrors     map[string][]string   `json:",omitempty"`
	RegistryCredentials map[string]AuthConfig `json:",omitempty"`
	SandboxImage        string                `json:",omitempty"`
}

func (cfg Config) String() string {
	switch {
	case cfg.Containerd != nil:
		return containerdName
	case cfg.Docker != nil:
		return dockerName
	}

	return dockerName
}

func (cfg Config) Engine(kubeletVersion *semver.Version) Engine {
	docker := &Docker{
		insecureRegistries: cfg.InsecureRegistries,
		registryMirrors:    cfg.RegistryMirrors["docker.io"],
	}

	containerd := &Containerd{
		insecureRegistries:  cfg.InsecureRegistries,
		registryMirrors:     cfg.RegistryMirrors,
		sandboxImage:        cfg.SandboxImage,
		registryCredentials: cfg.RegistryCredentials,
	}

	moreThan124, _ := semver.NewConstraint(">= 1.24")

	switch {
	case moreThan124.Check(kubeletVersion):
		return containerd
	case cfg.Docker != nil:
		return docker
	case cfg.Containerd != nil:
		return containerd
	}

	return docker
}
