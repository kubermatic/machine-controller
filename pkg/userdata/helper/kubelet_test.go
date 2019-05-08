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
	"net"
	"testing"

	testhelper "github.com/kubermatic/machine-controller/pkg/test"

	"github.com/Masterminds/semver"
)

type kubeletFlagTestCase struct {
	name           string
	version        *semver.Version
	dnsIPs         []net.IP
	hostname       string
	cloudProvider  string
	external       bool
	hasCloudConfig bool
}

func TestKubeletSystemdUnit(t *testing.T) {
	var tests []kubeletFlagTestCase
	for _, version := range versions {
		tests = append(tests,
			kubeletFlagTestCase{
				name:           fmt.Sprintf("version-%s", version.Original()),
				version:        version,
				dnsIPs:         []net.IP{net.ParseIP("10.10.10.10")},
				hostname:       "some-test-node",
				hasCloudConfig: true,
			},
			kubeletFlagTestCase{
				name:           fmt.Sprintf("version-%s-external", version.Original()),
				version:        version,
				dnsIPs:         []net.IP{net.ParseIP("10.10.10.10")},
				hostname:       "some-test-node",
				external:       true,
				hasCloudConfig: true,
			},
		)
	}
	tests = append(tests, []kubeletFlagTestCase{
		{
			name:    "multiple-dns-servers",
			version: semver.MustParse("v1.10.1"),
			dnsIPs: []net.IP{
				net.ParseIP("10.10.10.10"),
				net.ParseIP("10.10.10.11"),
				net.ParseIP("10.10.10.12"),
			},
			hostname: "some-test-node",
		},
		{
			name:          "cloud-provider-set",
			version:       semver.MustParse("v1.10.1"),
			dnsIPs:        []net.IP{net.ParseIP("10.10.10.10")},
			hostname:      "some-test-node",
			cloudProvider: "aws",
		},
	}...)

	for _, test := range tests {
		name := fmt.Sprintf("kublet_systemd_unit_%s", test.name)
		t.Run(name, func(t *testing.T) {
			out, err := KubeletSystemdUnit(test.version.String(), test.cloudProvider, test.hostname, test.dnsIPs, test.external, test.hasCloudConfig)
			if err != nil {
				t.Fatal(err)
			}
			goldenName := name + ".golden"
			testhelper.CompareOutput(t, goldenName, out, *update)
		})
	}
}
