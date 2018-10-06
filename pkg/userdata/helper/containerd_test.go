package helper

import (
	"fmt"
	"testing"

	"github.com/kubermatic/machine-controller/pkg/test"
)

func TestContainerdConfig(t *testing.T) {
	for _, version := range versions {
		name := fmt.Sprintf("containerd_config_%s", version.Original())
		t.Run(name, func(t *testing.T) {
			script, err := ContainerdConfig(version.String())
			if err != nil {
				t.Error(err)
			}

			test.CompareOutput(t, name, script, *update)
		})
	}
}

func TestContainerdSystemdUnitConfig(t *testing.T) {
	for _, version := range versions {
		name := fmt.Sprintf("containerd_systemd_unit_%s", version.Original())
		t.Run(name, func(t *testing.T) {
			script, err := ContainerdSystemdUnit(version.String())
			if err != nil {
				t.Error(err)
			}

			test.CompareOutput(t, name, script, *update)
		})
	}
}
