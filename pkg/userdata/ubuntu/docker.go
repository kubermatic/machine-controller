package ubuntu

import (
	"github.com/Masterminds/semver"
	"github.com/golang/glog"
	"github.com/kubermatic/machine-controller/pkg/containerruntime/docker"
	"k8s.io/apimachinery/pkg/util/sets"
)

type installCandidate struct {
	versions   []string
	pkgVersion string
	pkg        string
}

var fallbackInstallCandidate = installCandidate{
	versions:   []string{"17.12", "17.12.1"},
	pkg:        "docker.io",
	pkgVersion: "17.12.1-0ubuntu1",
}

var dockerInstallCandidates = []installCandidate{
	{
		versions:   []string{"17.12", "17.12.1"},
		pkg:        "docker.io",
		pkgVersion: "17.12.1-0ubuntu1",
	},
	{
		versions:   []string{"18.03", "18.03.1"},
		pkg:        "docker-ce",
		pkgVersion: "18.03.1~ce~3-0~ubuntu",
	},
	{
		versions:   []string{"18.06.0"},
		pkg:        "docker-ce",
		pkgVersion: "18.06.0~ce~3-0~ubuntu",
	},
}

func getDockerInstallCandidate(kubeletVersion *semver.Version) (pkg string, version string) {
	supportedKubeletVersions := docker.GetVersionsForKubelet(kubeletVersion)

	for _, supportedVersion := range supportedKubeletVersions {
		for _, ic := range dockerInstallCandidates {
			if sets.NewString(ic.versions...).Has(supportedVersion) {
				return ic.pkg, ic.pkgVersion
			}
		}
	}

	glog.V(2).Infof("No install candidate for docker found which fits to supported kubelet versions %v. Falling back to apt package %s=%s", fallbackInstallCandidate.pkg, fallbackInstallCandidate.pkgVersion, supportedKubeletVersions)
	return fallbackInstallCandidate.pkg, fallbackInstallCandidate.pkgVersion
}
