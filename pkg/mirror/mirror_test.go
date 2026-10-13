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

package mirror

import (
	"strings"
	"testing"
)

func TestManifestEmbedded(t *testing.T) {
	if len(images) == 0 {
		t.Fatal("expected embedded mirror-images.yaml to populate images map")
	}
}

func TestImageReturnsMirrorReference(t *testing.T) {
	var anyKey string
	for k := range images {
		anyKey = k
		break
	}
	if anyKey == "" {
		t.Fatal("no entries in images map")
	}
	got := Image(anyKey)
	wantPrefix := registryPrefix + "/" + anyKey + "@sha256:"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("Image(%q) = %q, want prefix %q", anyKey, got, wantPrefix)
	}
}

func TestImagePanicsOnUnknownKey(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected Image to panic on unknown key")
		}
	}()
	_ = Image("does-not-exist")
}
