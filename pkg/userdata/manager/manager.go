//
// UserData plugin manager.
//

// Package manager provides managing, starting, stopping, and
// communication with the individual UserData plugins. It's used
// inside the machine controller.
package manager

import (
	"errors"
	"flag"
	"log"
	"sync"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

var (
	// ErrPluginNotFound describes an invalid operating system for
	// a user data plugin. Here directory has to be checked if
	// correct ones are installed.
	ErrPluginNotFound = errors.New("no user data plugin for the given operating system found")
)

var (
	// mu avoids race conditions for the global manager.
	mu sync.Mutex

	// debug contains the debug flag, default is false.
	debug bool

	// plugins contains the registered plugins.
	plugins map[providerconfig.OperatingSystem]*Plugin
)

// init  checks the plugin debug flag.
func init() {
	flag.BoolVar(&debug, "plugin-debug", false, "Switch for enabling the plugin debugging")

	loadPlugins()
}

// ForOS returns the plugin for the given operating system.
func ForOS(os providerconfig.OperatingSystem) (p *Plugin, err error) {
	mu.Lock()
	defer mu.Unlock()

	if plugins == nil {
		loadPlugins()
	}

	var found bool
	if p, found = plugins[os]; !found {
		return nil, ErrPluginNotFound
	}

	return p, nil
}

// Supports answers if the userdata manager supports the
func Supports(os providerconfig.OperatingSystem) bool {
	mu.Lock()
	defer mu.Unlock()

	_, found := plugins[os]

	return found
}

// Stop kills and derigisters all plugins.
func Stop() error {
	mu.Lock()
	defer mu.Unlock()

	var serr error
	for _, p := range plugins {
		if err := p.Stop(); err != nil {
			serr = err
		}
	}
	plugins = nil

	return serr
}

// loadPlugins lazily loads the plugins on initial usage.
func loadPlugins() {
	plugins = make(map[providerconfig.OperatingSystem]*Plugin)

	for _, os := range []providerconfig.OperatingSystem{
		providerconfig.OperatingSystemCentOS,
		providerconfig.OperatingSystemCoreos,
		providerconfig.OperatingSystemUbuntu,
	} {
		plugin, err := newPlugin(os, debug)
		if err != nil {
			log.Printf("cannot start plugin '%v': %v", os, err)
			continue
		}
		plugins[os] = plugin
	}
}
