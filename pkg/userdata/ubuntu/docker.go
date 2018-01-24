package ubuntu

import "errors"

func getDockerInstallCandidate(desiredVersion string) (pkg string, version string, err error) {
	switch desiredVersion {
	case "1.10", "1.10.3":
		pkg = "docker.io"
		version = "1.10.3-0ubuntu6"
	case "1.13", "1.13.1":
		pkg = "docker.io"
		version = "1.13.1-0ubuntu1~16.04.2"
	case "17.03.0":
		pkg = "docker-ce"
		version = "17.03.0~ce-0~ubuntu-xenial"
	case "17.03.1":
		pkg = "docker-ce"
		version = "17.03.1~ce-0~ubuntu-xenial"
	case "17.03", "17.03.2":
		pkg = "docker-ce"
		version = "17.03.2~ce-0~ubuntu-xenial"
	case "17.06.0":
		pkg = "docker-ce"
		version = "17.06.0~ce-0~ubuntu"
	case "17.06.1":
		pkg = "docker-ce"
		version = "17.06.1~ce-0~ubuntu"
	case "17.06", "17.06.2":
		pkg = "docker-ce"
		version = "17.06.2~ce-0~ubuntu"
	case "17.09.0":
		pkg = "docker-ce"
		version = "17.09.0~ce-0~ubuntu"
	case "17.09", "17.09.1":
		pkg = "docker-ce"
		version = "17.09.1~ce-0~ubuntu"
	case "17.12", "17.12.0":
		pkg = "docker-ce"
		version = "17.12.0~ce-0~ubuntu"
	default:
		err = errors.New("no install candidate found for the requested version")
	}

	return pkg, version, err
}
