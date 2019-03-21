//
// Userdata manager for plugin handling from machine controller side.
//

// Package manager provides the communication gPRC client-part
// for userdata plugins. It checks for available plugin binaries,
// starts those with accoding arguments and registers them for
// usage.
package manager

import (
	"errors"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

const (
	// basePort is the first dynamic port for a found plugin.
	// TODO Range has to be defined.
	basePort = 60000

	// pluginPrefix has to be the prefix of all plugin filenames.
	pluginPrefix = "machine-controller-userdata-"
)

var (
	// ErrPluginNotFound describes an invalid operating system for
	// a user data plugin. Here directory has to be checked if
	// correct ones are installed.
	ErrPluginNotFound = errors.New("no user data plugin for the given operating system found")

	// plugins contains all found and successfully started plugins.
	plugins map[providerconfig.OperatingSystem]*plugin
)

// init creates the central manager instance.
func init() {
	plugins = make(map[providerconfig.OperatingSystem]*plugin)
	files, err := ioutil.ReadDir(".")
	if err != nil {
		// TODO Only logging or exit the machine controller?
	}
	for i, file := range files {
		if strings.HasPrefix(file.Name(), pluginPrefix) {
			p, err := newPlugin(file.Name(), basePort+i)
			if err != nil {
				// TODO Log and skip.
			}
			plugins[p.OperatingSystem()] = p
		}
	}
}

// ForOS returns the plugin for the given operating system.
func ForOS(os providerconfig.OperatingSystem) (p *Plugin, err error) {
	if p, found = providers[os]; !found {
		return nil, ErrPluginNotFound
	}
	return p, nil
}
