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

//
// UserData plugin manager.
//

// Package manager provides the instantiation and
// running of the plugins on machine controller side.
package manager

import (
	"errors"
	"flag"

	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	"k8s.io/klog"
)

var (
	// ErrLocatingPlugins is returned when a new manager cannot locate
	// the plugins for the supported operating systems.
	ErrLocatingPlugins = errors.New("one or more user data plugins not found")

	// ErrPluginNotFound describes an invalid operating system for
	// a user data plugin. Here directory has to be checked if
	// correct ones are installed.
	ErrPluginNotFound = errors.New("no user data plugin for the given operating system found")

	// supportedOS contains a list of operating systems the machine
	// controller supports.
	supportedOS = []providerconfigtypes.OperatingSystem{
		providerconfigtypes.OperatingSystemAmazonLinux2,
		providerconfigtypes.OperatingSystemCentOS,
		providerconfigtypes.OperatingSystemFlatcar,
		providerconfigtypes.OperatingSystemRHEL,
		providerconfigtypes.OperatingSystemUbuntu,
		providerconfigtypes.OperatingSystemRockyLinux,
	}
)

// Manager inits and manages the userdata plugins.
type Manager struct {
	debug   bool
	plugins map[providerconfigtypes.OperatingSystem]*Plugin
}

// New returns an initialised plugin manager.
func New() (*Manager, error) {
	m := &Manager{
		plugins: make(map[providerconfigtypes.OperatingSystem]*Plugin),
	}
	flag.BoolVar(&m.debug, "plugin-debug", false, "Switch for enabling the plugin debugging")
	m.locatePlugins()
	if len(m.plugins) < len(supportedOS) {
		return nil, ErrLocatingPlugins
	}
	return m, nil
}

// ForOS returns the plugin for the given operating system.
func (m *Manager) ForOS(os providerconfigtypes.OperatingSystem) (p *Plugin, err error) {
	var found bool
	if p, found = m.plugins[os]; !found {
		return nil, ErrPluginNotFound
	}
	return p, nil
}

// locatePlugins tries to find the plugins and inits their wrapper.
func (m *Manager) locatePlugins() {
	for _, os := range supportedOS {
		plugin, err := newPlugin(os, m.debug)
		if err != nil {
			klog.Errorf("cannot use plugin '%v': %v", os, err)
			continue
		}
		m.plugins[os] = plugin
	}
}
