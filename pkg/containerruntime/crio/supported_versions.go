package crio

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver"
)

// GetOfficiallySupportedVersions returns the officially supported cri-o version for the given kubernetes version
func GetOfficiallySupportedVersions(kubernetesVersion string) ([]string, error) {
	v, err := semver.NewVersion(kubernetesVersion)
	if err != nil {
		return nil, err
	}

	majorMinorString := fmt.Sprintf("%d.%d", v.Major(), v.Minor())
	switch majorMinorString {
	case "1.8":
		return []string{"1.8"}, nil
	case "1.9":
		return []string{"1.9"}, nil
	default:
		return nil, errors.New("no supported versions available")
	}
}
