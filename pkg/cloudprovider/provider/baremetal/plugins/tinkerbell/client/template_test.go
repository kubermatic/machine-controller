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

	"k8c.io/machine-controller/pkg/mirror"
)

// tinkerbellImageKeys lists every manifest key Tinkerbell's template.go
// references. The test asserts each one resolves through pkg/mirror -- if
// a key disappears from the manifest, the production init() panic would
// surface only when ProvisionServer runs. Catch it here instead.
var tinkerbellImageKeys = []string{
	"alpine",
	"tinkerbell-actions/image2disk",
	"tinkerbell/actions/cexec-pinned",
	"tinkerbell-actions/writefile",
	"tinkerbell-actions/cexec",
	"tinkerbell/actions/cexec-latest-resolved",
	"jacobweinstock/waitdaemon",
}

func TestTinkerbellImagesResolve(t *testing.T) {
	for _, key := range tinkerbellImageKeys {
		got := mirror.Image(key)
		want := "quay.io/kubermatic-mirror/images/" + key + "@sha256:"
		if !strings.HasPrefix(got, want) {
			t.Errorf("mirror.Image(%q) = %q, want prefix %q", key, got, want)
		}
	}
}
