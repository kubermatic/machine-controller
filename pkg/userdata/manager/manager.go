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
)

// Manager is responsible for starting and stopping all plugins.
type Manager struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  func()
	plugins map[providerconfig.OperatingSystem]*Plugin
}

// manager is the single instance of the plugin manager.
var manager *Manager

// New creates a new manager instance or returns the already
// existing.
func New(ctx context.Context, debug bool) *Manager {
	// TODO Handle race condition.
	if manager != nil {
		return manager
	}
	managerCtx, cancel := context.WithCancel(ctx)
	m := &Manager{
		ctx:     managerCtx,
		cancel:  cancel,
		plugins: make(map[providerconfig.OperatingSystem]*Plugin),
	}
	for i, os := range []providerconfig.OperatingSystem{
		providerconfig.OperatingSystemCentOS,
		providerconfig.OperatingSystemCoreos,
		providerconfig.OperatingSystemUbuntu,
	} {
		plugin, err := newPlugin(ctx, os, debug)
		if err != nil {
			// TODO Log error.
		}
		m.plugins[os] = plugin
	}
	manager = m
	return manager
}

// ForOS returns the plugin for the given operating system.
func (m *Manager) ForOS(os providerconfig.OperatingSystem) (p *Plugin, err error) {
	if p, found = m.plugins[os]; !found {
		return nil, ErrPluginNotFound
	}
	return p, nil
}

// Stop kills and derigisters all plugins.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var serr error
	for _, p := range m.Plugins {
		if err := p.Stop(); err != nil {
			serr = err
		}
	}
	m.plugins = make(map[providerconfig.OperatingSystem]*Plugin)
	m.cancel()
	return serr
}
