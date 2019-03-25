//
// Userdata manager for plugin handling from machine controller side.
//

// Package manager provides the communication gPRC client-part
// for userdata plugins. It checks for available plugin binaries,
// starts those with accoding arguments and registers them for
// usage.
package manager

import (
	"context"
	"errors"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

var (
	// ErrPluginNotFound describes an invalid operating system for
	// a user data plugin. Here directory has to be checked if
	// correct ones are installed.
	ErrPluginNotFound = errors.New("no user data plugin for the given operating system found")

	// cancel allows a graceful shutdown via context.
	cancel func()

	// plugins contains all found and successfully started plugins.
	plugins map[providerconfig.OperatingSystem]*plugin
)

// init creates the central manager instance.
func init() {
	var ctx context.Context

	ctx, cancel = context.WithCancel(ctx.Background())
	plugins = make(map[providerconfig.OperatingSystem]*Plugin)

	for i, os := range []providerconfig.OperatingSystem{
		providerconfig.OperatingSystemCentOS,
		providerconfig.OperatingSystemCoreos,
		providerconfig.OperatingSystemUbuntu,
	} {
		// TODO Handle debug flag.
		plugin, err := newPlugin(ctx, os, true)
		if err != nil {
			// TODO Log error.
		}
		plugins[os] = plugin
	}
}

// ForOS returns the plugin for the given operating system.
func ForOS(os providerconfig.OperatingSystem) (p *Plugin, err error) {
	if p, found = providers[os]; !found {
		return nil, ErrPluginNotFound
	}
	return p, nil
}

// Stop kills and derigisters all plugins.
func Stop() {
	plugins = make(map[providerconfig.OperatingSystem]*Plugin)
	cancel()
}
