/*
Copyright 2022 The Machine Controller Authors.

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

package containerruntime

import (
	"errors"
	"fmt"
	"testing"
)

func TestContainerdRegistryMirror(t *testing.T) {
	type testCase struct {
		desc            string
		flag            string
		expectedMirrors map[string][]string
		expectedError   error
	}

	testCases := []testCase{
		{
			desc:            "no registry mirrors set",
			flag:            "",
			expectedMirrors: map[string][]string{},
			expectedError:   nil,
		},

		{
			desc: "registry mirror without name and protocol",
			flag: "registry-v1.docker.io",
			expectedMirrors: map[string][]string{
				"docker.io": {"https://registry-v1.docker.io"},
			},
			expectedError: nil,
		},
		{
			desc: "multiple registry mirrors without name, with and without protocol",
			flag: "registry-v1.docker.io,http://registry.docker-cn.com",
			expectedMirrors: map[string][]string{
				"docker.io": {
					"https://registry-v1.docker.io",
					"http://registry.docker-cn.com",
				},
			},
			expectedError: nil,
		},

		{
			desc: "registry mirror with name and without protocol",
			flag: "quay.io=my-quay-io-mirror.example.com",
			expectedMirrors: map[string][]string{
				"quay.io": {"https://my-quay-io-mirror.example.com"},
			},
			expectedError: nil,
		},
		{
			desc: "registry mirror with name and protocol",
			flag: "quay.io=http://my-quay-io-mirror.example.com",
			expectedMirrors: map[string][]string{
				"quay.io": {"http://my-quay-io-mirror.example.com"},
			},
			expectedError: nil,
		},
		{
			desc: "multiple registry mirrors with same name",
			flag: "quay.io=http://my-quay-io-mirror.example.com,quay.io=example.net",
			expectedMirrors: map[string][]string{
				"quay.io": {
					"http://my-quay-io-mirror.example.com",
					"https://example.net",
				},
			},
			expectedError: nil,
		},

		{
			desc: "complex example",
			flag: "quay.io=http://my-quay-io-mirror.example.com,quay.io=example.net," +
				"registry-v1.docker.io,http://registry.docker-cn.com," +
				"ghcr.io=http://foo/bar",
			expectedMirrors: map[string][]string{
				"quay.io": {
					"http://my-quay-io-mirror.example.com",
					"https://example.net",
				},
				"docker.io": {
					"https://registry-v1.docker.io",
					"http://registry.docker-cn.com",
				},
				"ghcr.io": {
					"http://foo/bar",
				},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			opts := Opts{
				ContainerRuntime: containerdName,
				RegistryMirrors:  tc.flag,
			}

			config, err := BuildConfig(opts)
			if tc.expectedError != nil {
				if !errors.Is(err, tc.expectedError) {
					t.Errorf("expected error %q but got %q", tc.expectedError, err)
				}
			}

			if err != nil {
				t.Errorf("expected success but got error: %q", err)
			}

			if fmt.Sprint(config.RegistryMirrors) != fmt.Sprint(tc.expectedMirrors) {
				t.Errorf("expected to get %v instead got: %v", tc.expectedMirrors, config.RegistryMirrors)
			}
		})
	}
}
