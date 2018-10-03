package docker

import (
	"github.com/Masterminds/semver"
)

// GetVersionsForKubelet returns the officially supported docker version for the given kubernetes version
// The returned versions are sorted from highest to lowest
func GetVersionsForKubelet(v *semver.Version) []string {
	// Following the changelog from the kubernetes versions
	if v.Minor() <= 11 {
		// https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG-1.9.md#external-dependencies
		// https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG-1.10.md#external-dependencies
		// https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG-1.11.md#external-dependencies
		return []string{
			"17.03.3",
			"17.03.2",
			"17.03.1",
			"17.03.0",
			"1.13.1",
			"1.12.6",
			"1.11.2",
		}
	}

	// 12+ should be fine with the versions for v1.12
	// https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG-1.12.md#external-dependencies
	return []string{
		"18.06.1",
		"18.06.0",
		"17.09.1",
		"17.09.0",
		"17.06.2",
		"17.06.1",
		"17.06.0",
		"17.03.3",
		"17.03.2",
		"17.03.1",
		"17.03.0",
		"1.13.1",
		"1.12.6",
		"1.11.2",
	}
}
