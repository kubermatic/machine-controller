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
	"strings"
	"testing"
)

func TestMirrorImagesEmbedded(t *testing.T) {
	if len(mirrorImages) == 0 {
		t.Fatal("expected embedded mirror-images.yaml to populate mirrorImages")
	}
}

func TestMirrorImageResolvesKnownKeys(t *testing.T) {
	required := []string{
		"alpine",
		"tinkerbell-actions/image2disk",
		"tinkerbell/actions/cexec-pinned",
		"tinkerbell-actions/writefile",
		"tinkerbell-actions/cexec",
		"tinkerbell/actions/cexec-latest-resolved",
		"jacobweinstock/waitdaemon",
	}

	for _, key := range required {
		got := mirrorImage(key)
		if !strings.HasPrefix(got, mirrorRegistryPrefix+"/"+key+"@sha256:") {
			t.Errorf("mirrorImage(%q) = %q, want prefix %q/%s@sha256:",
				key, got, mirrorRegistryPrefix, key)
		}
	}
}

func TestMirrorImagePanicsOnUnknownKey(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected mirrorImage to panic on unknown key")
		}
	}()
	_ = mirrorImage("does-not-exist")
}
