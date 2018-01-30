package ubuntu

import (
	"github.com/kubermatic/machine-controller/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

var crioInstallCandidates = []installCandidate{
	{
		versions:   []string{"1.9", "1.9.1"},
		pkg:        "cri-o",
		pkgVersion: "1.9.1-1~ubuntu16.04.2~ppa1",
	},
}

func getCRIOInstallCandidate(desiredVersion string) (pkg string, version string, err error) {
	for _, ic := range crioInstallCandidates {
		if sets.NewString(ic.versions...).Has(desiredVersion) {
			return ic.pkg, ic.pkgVersion, nil
		}
	}

	return "", "", errors.VersionNotAvailableErr
}
