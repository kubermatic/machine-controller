//
// Userdata manager for plugin handling from machine controller side.
//

// Package manager provides the communication gPRC client-part
// for userdata plugins. It checks for available plugin binaries,
// starts those with accoding arguments and registers them for
// usage.
package manager

import (
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
)

// Manager is the responsible component for plugin discovery
// and instantiation.
type Manager struct {
	providers map[providerconfig.OperatingSystem]*Plugin
}

// New creates a new manager instance.
func New() *Manager {
	m := &Manager{}
	return m
}
