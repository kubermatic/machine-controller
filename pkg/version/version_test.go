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

package version_test

import (
	"runtime/debug"
	"testing"

	"k8c.io/machine-controller/pkg/version"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name          string
		buildInfoFunc version.BuildInfoReader
		wantRevision  string
		wantModuleVer string
		wantDirty     bool
	}{
		{
			name: "build info not available",
			buildInfoFunc: func() (*debug.BuildInfo, bool) {
				return nil, false
			},
			wantRevision:  "unknown",
			wantModuleVer: "",
			wantDirty:     false,
		},
		{
			name: "build info with module version and vcs info",
			buildInfoFunc: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Main: debug.Module{
						Version: "v1.2.3",
					},
					Settings: []debug.BuildSetting{
						{Key: "vcs.revision", Value: "abc123def456"},
						{Key: "vcs.modified", Value: "false"},
					},
				}, true
			},
			wantRevision:  "abc123def456",
			wantModuleVer: "v1.2.3",
			wantDirty:     false,
		},
		{
			name: "build info with dirty working directory",
			buildInfoFunc: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Main: debug.Module{
						Version: "v0.5.0",
					},
					Settings: []debug.BuildSetting{
						{Key: "vcs.revision", Value: "deadbeef"},
						{Key: "vcs.modified", Value: "true"},
					},
				}, true
			},
			wantRevision:  "deadbeef",
			wantModuleVer: "v0.5.0",
			wantDirty:     true,
		},
		{
			name: "build info with devel version",
			buildInfoFunc: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Main: debug.Module{
						Version: "(devel)",
					},
					Settings: []debug.BuildSetting{
						{Key: "vcs.revision", Value: "cafe1234"},
						{Key: "vcs.modified", Value: "false"},
					},
				}, true
			},
			wantRevision:  "cafe1234",
			wantModuleVer: "",
			wantDirty:     false,
		},
		{
			name: "build info with empty version",
			buildInfoFunc: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Main: debug.Module{
						Version: "",
					},
					Settings: []debug.BuildSetting{
						{Key: "vcs.revision", Value: "1a2b3c4d"},
					},
				}, true
			},
			wantRevision:  "1a2b3c4d",
			wantModuleVer: "",
			wantDirty:     false,
		},
		{
			name: "build info without vcs settings",
			buildInfoFunc: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Main: debug.Module{
						Version: "v2.0.0",
					},
					Settings: []debug.BuildSetting{},
				}, true
			},
			wantRevision:  "unknown",
			wantModuleVer: "v2.0.0",
			wantDirty:     false,
		},
		{
			name: "build info with only revision",
			buildInfoFunc: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Main: debug.Module{},
					Settings: []debug.BuildSetting{
						{Key: "vcs.revision", Value: "abcdef123456"},
					},
				}, true
			},
			wantRevision:  "abcdef123456",
			wantModuleVer: "",
			wantDirty:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := version.Get(version.WithReadBuildInfoFunc(tt.buildInfoFunc))

			if got.Revision != tt.wantRevision {
				t.Errorf("Get().Revision = %v, want %v", got.Revision, tt.wantRevision)
			}
			if got.ModuleVersion != tt.wantModuleVer {
				t.Errorf("Get().ModuleVersion = %v, want %v", got.ModuleVersion, tt.wantModuleVer)
			}
			if got.Dirty != tt.wantDirty {
				t.Errorf("Get().Dirty = %v, want %v", got.Dirty, tt.wantDirty)
			}
		})
	}
}

func TestInfo_String(t *testing.T) {
	tests := []struct {
		name string
		info version.Info
		want string
	}{
		{
			name: "module version takes precedence",
			info: version.Info{
				ModuleVersion: "v1.2.3",
				Revision:      "abc123",
				Dirty:         false,
			},
			want: "v1.2.3",
		},
		{
			name: "module version with dirty flag (dirty is ignored when module version present)",
			info: version.Info{
				ModuleVersion: "v2.0.0",
				Revision:      "deadbeef",
				Dirty:         true,
			},
			want: "v2.0.0",
		},
		{
			name: "revision without module version",
			info: version.Info{
				ModuleVersion: "",
				Revision:      "cafe1234",
				Dirty:         false,
			},
			want: "cafe1234",
		},
		{
			name: "revision with dirty flag",
			info: version.Info{
				ModuleVersion: "",
				Revision:      "abc123def",
				Dirty:         true,
			},
			want: "abc123def-dirty",
		},
		{
			name: "unknown revision returns dev",
			info: version.Info{
				ModuleVersion: "",
				Revision:      "unknown",
				Dirty:         false,
			},
			want: "dev",
		},
		{
			name: "unknown revision with dirty flag returns dev",
			info: version.Info{
				ModuleVersion: "",
				Revision:      "unknown",
				Dirty:         true,
			},
			want: "dev",
		},
		{
			name: "empty revision returns empty string",
			info: version.Info{
				ModuleVersion: "",
				Revision:      "",
				Dirty:         false,
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.String()
			if got != tt.want {
				t.Errorf("Info.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGet_WithoutOptions(t *testing.T) {
	// Test calling Get without any options to ensure it doesn't panic
	// and returns a valid Info struct
	info := version.Get()

	// Basic validation - it should have some value for Revision
	if info.Revision == "" {
		t.Error("Get() without options returned empty Revision")
	}

	// String() should not panic and return something
	str := info.String()
	if str == "" {
		t.Error("Info.String() returned empty string")
	}
}
