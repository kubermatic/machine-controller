package helper

import (
	"fmt"
	"testing"

	"github.com/kubermatic/machine-controller/pkg/test"
)

func TestDownloadBinariesScript(t *testing.T) {
	for _, version := range Versions {
		name := fmt.Sprintf("download_binaries_%s", version.Original())
		t.Run(name, func(t *testing.T) {
			script, err := DownloadBinariesScript(version.String(), true)
			if err != nil {
				t.Error(err)
			}

			test.CompareOutput(t, name, script, *update)
		})
	}
}
