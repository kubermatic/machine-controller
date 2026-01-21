/*
Copyright 2026 The Machine Controller Authors.

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

package version

import (
	"runtime/debug"
)

// BuildInfoReader is a function type for reading build info, allowing dependency injection for testing.
type BuildInfoReader func() (*debug.BuildInfo, bool)

// Info holds version information extracted from build metadata.
type Info struct {
	ModuleVersion string // Module version from build info
	Revision      string // Git commit hash
	Dirty         bool   // Whether working directory had uncommitted changes
	readBuildInfo BuildInfoReader
}

type Option func(*Info)

func WithReadBuildInfoFunc(f BuildInfoReader) Option {
	return func(i *Info) {
		i.readBuildInfo = f
	}
}

// Get retrieves version information from build metadata using the default debug.ReadBuildInfo.
func Get(opts ...Option) Info {
	info := Info{
		Revision:      "unknown",
		readBuildInfo: debug.ReadBuildInfo,
	}

	for _, opt := range opts {
		opt(&info)
	}

	bi, ok := info.readBuildInfo()
	if !ok {
		return info
	}

	// Save the main module version
	if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		info.ModuleVersion = bi.Main.Version
	}

	// Extract VCS info from build settings
	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.Revision = setting.Value
		case "vcs.modified":
			info.Dirty = setting.Value == "true"
		}
	}

	return info
}

// String returns a formatted version string based on the available build version information.
func (i Info) String() string {
	// Use module version if available
	if i.ModuleVersion != "" {
		return i.ModuleVersion
	}

	// Fall back to VCS revision
	if i.Revision == "unknown" {
		return "dev"
	}

	version := i.Revision
	if i.Dirty {
		version += "-dirty"
	}

	return version
}
