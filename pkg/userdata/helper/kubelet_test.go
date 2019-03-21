package helper

import (
	"fmt"
	"net"
	"testing"

	testhelper "github.com/kubermatic/machine-controller/pkg/test"

	"github.com/Masterminds/semver"
)

type kubeletFlagTestCase struct {
	name          string
	version       *semver.Version
	dnsIPs        []net.IP
	hostname      string
	cloudProvider string
	external      bool
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
			out, err := KubeletSystemdUnit(test.version.String(), test.cloudProvider, test.hostname, test.dnsIPs, test.external)
			if err != nil {
				t.Error(err)
			}

			testhelper.CompareOutput(t, name, out, *update)
		})
	}
}
