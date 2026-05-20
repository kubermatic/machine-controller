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

package client

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

// mirrorRegistryPrefix is the destination registry path images are mirrored to.
// Must match the prefix the mirror-images.sh script writes to and the validator
// checks against.
const mirrorRegistryPrefix = "quay.io/kubermatic-mirror/images"

//go:embed mirror-images.yaml
var mirrorImagesYAML []byte

type mirrorEntry struct {
	Source  string `yaml:"source"`
	Version string `yaml:"version"`
}

type mirrorManifest struct {
	Images map[string]mirrorEntry `yaml:"images"`
}

// mirrorImages is the parsed manifest. Populated at package init.
var mirrorImages map[string]mirrorEntry

func init() {
	var m mirrorManifest
	if err := yaml.Unmarshal(mirrorImagesYAML, &m); err != nil {
		panic(fmt.Sprintf("tinkerbell: failed to parse embedded mirror-images.yaml: %v", err))
	}

	if len(m.Images) == 0 {
		panic("tinkerbell: embedded mirror-images.yaml has no .images entries")
	}

	mirrorImages = m.Images
}

// mirrorImage returns the fully qualified, digest-pinned mirror reference
// for the given manifest key. Panics if the key is missing -- this is a
// programming error, not a runtime condition, and provisioning with a
// wrong image is a worse failure mode than crashing at process start.
func mirrorImage(key string) string {
	entry, ok := mirrorImages[key]
	if !ok {
		panic(fmt.Sprintf("tinkerbell: mirror-images.yaml has no entry for key %q", key))
	}

	return fmt.Sprintf("%s/%s@%s", mirrorRegistryPrefix, key, entry.Version)
}
