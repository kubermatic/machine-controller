package ubuntu

import (
	"testing"

	"github.com/go-test/deep"
	"github.com/kubermatic/machine-controller/pkg/errors"
)

func TestGetCRIOInstallCandidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		version string
		resErr  error
		resPkg  string
		resVer  string
	}{
		{
			name:    "no version found",
			version: "foo-does-not-exist",
			resErr:  errors.VersionNotAvailableErr,
			resPkg:  "",
			resVer:  "",
		},
		{
			name:    "get patch version",
			version: "1.9.1",
			resErr:  nil,
			resPkg:  "cri-o",
			resVer:  "1.9.1-1~ubuntu16.04.2~ppa1",
		},
		{
			name:    "get minor version",
			version: "1.9",
			resErr:  nil,
			resPkg:  "cri-o",
			resVer:  "1.9.1-1~ubuntu16.04.2~ppa1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pkg, version, err := getCRIOInstallCandidate(test.version)
			if diff := deep.Equal(err, test.resErr); diff != nil {
				t.Errorf("expected to get %v instead got: %v", test.resErr, err)
			}
			if err != nil {
				return
			}
			if pkg != test.resPkg {
				t.Errorf("expected to get %v instead got: %v", test.resPkg, pkg)
			}
			if version != test.resVer {
				t.Errorf("expected to get %v instead got: %v", test.resVer, version)
			}
		})
	}

}
