//
// Google Cloud Provider for the Machine Controller
//

package gce

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/kubermatic/machine-controller/pkg/ini"

	"github.com/Masterminds/sprig"
)

// cloudConfigTemplate renders the cloud-config in gcfg format. All
// fields are optional, that's why containing the ifs and the explicit newlines.
const cloudConfigTemplate = "[global]\n" +
	"project-id = {{ .Global.ProjectID | iniEscape }}\n" +
	"local-zone = {{ .Global.LocalZone | iniEscape }}\n"

// global contains the values of the global section of the cloud configuration.
type global struct {
	ProjectID string
	LocalZone string
}

// cloudConfig contains only the section global.
type cloudConfig struct {
	Global global
}

// asString renders the cloud configuration as string.
func (cc *cloudConfig) asString() (string, error) {
	funcMap := sprig.TxtFuncMap()
	funcMap["iniEscape"] = ini.Escape

	tmpl, err := template.New("cloud-config").Funcs(funcMap).Parse(cloudConfigTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse the cloud config template: %v", err)
	}

	buf := &bytes.Buffer{}
	if err := tmpl.Execute(buf, cc); err != nil {
		return "", fmt.Errorf("failed to execute cloud config template: %v", err)
	}

	return buf.String(), nil
}
