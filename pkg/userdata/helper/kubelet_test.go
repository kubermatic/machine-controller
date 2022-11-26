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

	"github.com/Masterminds/semver/v3"

	testhelper "github.com/kubermatic/machine-controller/pkg/test"

	corev1 "k8s.io/api/core/v1"
)

type kubeletFlagTestCase struct {
	name             string
	containerRuntime string
	version          *semver.Version
	dnsIPs           []net.IP
	hostname         string
	cloudProvider    string
	external         bool
	pauseImage       string
	initialTaints    []corev1.Taint
	extraFlags       []string
}

func TestKubeletSystemdUnit(t *testing.T) {
	var tests []kubeletFlagTestCase
	for _, version := range versions {
		tests = append(tests,
			kubeletFlagTestCase{
				name:     fmt.Sprintf("version-%s", version.Original()),
				version:  version,
				dnsIPs:   []net.IP{net.ParseIP("10.10.10.10")},
				hostname: "some-test-node",
			},
			kubeletFlagTestCase{
				name:     fmt.Sprintf("version-%s-external", version.Original()),
				version:  version,
				dnsIPs:   []net.IP{net.ParseIP("10.10.10.10")},
				hostname: "some-test-node",
				external: true,
			},
		)
	}
	tests = append(tests, []kubeletFlagTestCase{
		{
			name:    "multiple-dns-servers",
			version: semver.MustParse("v1.23.5"),
			dnsIPs: []net.IP{
				net.ParseIP("10.10.10.10"),
				net.ParseIP("10.10.10.11"),
				net.ParseIP("10.10.10.12"),
			},
			hostname: "some-test-node",
		},
		{
			name:          "cloud-provider-set",
			version:       semver.MustParse("v1.23.5"),
			dnsIPs:        []net.IP{net.ParseIP("10.10.10.10")},
			hostname:      "some-test-node",
			cloudProvider: "aws",
		},
		{
			name:          "pause-image-set",
			version:       semver.MustParse("v1.23.5"),
			dnsIPs:        []net.IP{net.ParseIP("10.10.10.10")},
			hostname:      "some-test-node",
			cloudProvider: "aws",
			pauseImage:    "192.168.100.100:5000/kubernetes/pause:v3.1",
		},
		{
			name:          "taints-set",
			version:       semver.MustParse("v1.23.5"),
			dnsIPs:        []net.IP{net.ParseIP("10.10.10.10")},
			hostname:      "some-test-node",
			cloudProvider: "aws",
			initialTaints: []corev1.Taint{
				{
					Key:    "key1",
					Value:  "value1",
					Effect: corev1.TaintEffectNoSchedule,
				},
				{
					Key:    "key2",
					Value:  "value2",
					Effect: corev1.TaintEffectNoExecute,
				},
			},
		},
	}...)

	for _, test := range tests {
		name := fmt.Sprintf("kublet_systemd_unit_%s", test.name)
		t.Run(name, func(t *testing.T) {
			out, err := KubeletSystemdUnit(
				defaultTo(test.containerRuntime, "docker"),
				test.version.String(),
				test.cloudProvider,
				test.hostname,
				test.dnsIPs,
				test.external,
				test.pauseImage,
				test.initialTaints,
				test.extraFlags,
				true,
			)
			if err != nil {
				t.Error(err)
			}
			goldenName := name + ".golden"
			testhelper.CompareOutput(t, goldenName, out, *update)
		})
	}
}

func defaultTo(in string, defaultValue string) string {
	if in == "" {
		return defaultValue
	}

	return in
}
