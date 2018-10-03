package ubuntu

import (
	"testing"

	"github.com/Masterminds/semver"
)

func TestGetDockerInstallCandidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		kubeletVersion string
		resPkg         string
		resVer         string
	}{
		{
			name:           "fallback version",
			kubeletVersion: "v1.9.3",
			resPkg:         "docker.io",
			resVer:         "17.12.1-0ubuntu1",
		},
		{
			name:           "v1.12.0 gets 18.06.0~ce~3-0~ubuntu",
			kubeletVersion: "1.12.0",
			resPkg:         "docker-ce",
			resVer:         "18.06.0~ce~3-0~ubuntu",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pkg, version := getDockerInstallCandidate(semver.MustParse(test.kubeletVersion))
			if pkg != test.resPkg {
				t.Errorf("expected to get %v instead got: %v", test.resPkg, pkg)
			}
			if version != test.resVer {
				t.Errorf("expected to get %v instead got: %v", test.resVer, version)
			}
		})
	}

}
