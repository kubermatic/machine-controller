package docker

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver"
)

// GetOfficiallySupportedVersions returns the officially supported docker version for the given kubernetes version
func GetOfficiallySupportedVersions(kubernetesVersion string) ([]string, error) {
	v, err := semver.NewVersion(kubernetesVersion)
	if err != nil {
		return nil, err
	}

	majorMinorString := fmt.Sprintf("%d.%d", v.Major(), v.Minor())
	switch majorMinorString {
	case "1.8", "1.9":
		return []string{"1.11.2", "1.12.6", "1.13.1", "17.03.2"}, nil
	default:
		return nil, errors.New("no supported versions available")
	}
}
