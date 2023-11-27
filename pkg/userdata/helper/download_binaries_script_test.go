/*
Copyright 2019 The Machine Controller Authors.

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

package helper

import (
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/kubermatic/machine-controller/pkg/test"
)

const goldenExtension = ".golden"

func TestDownloadBinariesScript(t *testing.T) {
	for _, version := range versions {
		name := fmt.Sprintf("download_binaries_%s", version.Original())
		t.Run(name, func(t *testing.T) {
			script, err := DownloadBinariesScript(zap.NewNop().Sugar(), version.String(), true)
			if err != nil {
				t.Error(err)
			}
			goldenName := name + goldenExtension
			test.CompareOutput(t, goldenName, script, *update)
		})
	}
}

func TestSafeDownloadBinariesScript(t *testing.T) {
	name := "safe_download_binaries_v1.26.6"
	t.Run(name, func(t *testing.T) {
		script, err := SafeDownloadBinariesScript(zap.NewNop().Sugar(), "v1.26.6")
		if err != nil {
			t.Error(err)
		}
		goldenName := name + goldenExtension
		test.CompareOutput(t, goldenName, script, *update)
	})
}
