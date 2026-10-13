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

// Package mirror provides a single source of truth for container images
// that the machine-controller mirrors from upstream registries to
// quay.io/kubermatic-mirror. Providers import this package and call
// Image(key) to resolve a manifest key to its fully qualified, digest-pinned
// reference in the mirror.
//
// To add or update an image:
//  1. Edit pkg/mirror/mirror-images.yaml.
//  2. Reference the key from the provider code via mirror.Image("<key>").
//  3. On merge to main, the post-machine-controller-mirror-images
//     postsubmit pushes the image to the mirror registry.
package mirror

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

// registryPrefix is the destination registry path images are mirrored to.
// Must match the prefix hack/mirror-images.sh writes to and the validator
// at hack/ci/validate-mirror-images.sh checks against.
const registryPrefix = "quay.io/kubermatic-mirror/images"

//go:embed mirror-images.yaml
var manifestBytes []byte

type entry struct {
	Source  string `yaml:"source"`
	Version string `yaml:"version"`
}

type manifest struct {
	Images map[string]entry `yaml:"images"`
}

var images map[string]entry

func init() {
	var m manifest
	if err := yaml.Unmarshal(manifestBytes, &m); err != nil {
		panic(fmt.Sprintf("mirror: failed to parse embedded mirror-images.yaml: %v", err))
	}
	if len(m.Images) == 0 {
		panic("mirror: embedded mirror-images.yaml has no .images entries")
	}
	images = m.Images
}

// Image returns the fully qualified, digest-pinned mirror reference
// for the given manifest key. Panics if the key is missing -- this is a
// programming error and provisioning with a wrong image is a worse failure
// mode than crashing at process start.
func Image(key string) string {
	e, ok := images[key]
	if !ok {
		panic(fmt.Sprintf("mirror: mirror-images.yaml has no entry for key %q", key))
	}
	return fmt.Sprintf("%s/%s@%s", registryPrefix, key, e.Version)
}
