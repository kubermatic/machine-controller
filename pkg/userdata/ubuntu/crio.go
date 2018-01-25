package ubuntu

import "errors"

func getCRIOInstallCandidate(desiredVersion string) (pkg string, version string, err error) {
	switch desiredVersion {
	case "1.9", "1.9.0":
		pkg = "cri-o"
		version = "1.9.0-1~ubuntu16.04.2~ppa1"
	default:
		err = errors.New("no install candidate found for the requested version")
	}

	return pkg, version, err
}
