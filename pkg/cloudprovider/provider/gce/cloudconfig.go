//
// Google Cloud Provider for the Machine Controller
//

package gce

//-----
// Imports
//-----

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/kubermatic/machine-controller/pkg/ini"

	"github.com/Masterminds/sprig"
)

//-----
// Constants
//-----

// cloudConfigTemplate renders the cloud-config in gcfg format. All
// fields are optional, that's why containing the ifs and the explicit newlines.
// TODO(mue) Check for mandatory fields and better ways to drop empty fields,
// e.g. by filtering afterwards.
const cloudConfigTemplate = "[global]\n" +
	"{{if .Global.TokenURL}}token-url = {{ .Global.TokenURL | iniEscape }}\n{{end}}" +
	"{{if .Global.TokenBody}}token-body = {{ .Global.TokenBody | iniEscape }}\n{{end}}" +
	"{{if .Global.ProjectID}}project-id = {{ .Global.ProjectID | iniEscape }}\n{{end}}" +
	"{{if .Global.NetworkProjectID}}network-project-id = {{ .Global.NetworkProjectID | iniEscape }}\n{{end}}" +
	"{{if .Global.NetworkName}}network-name = {{ .Global.NetworkName | iniEscape }}\n{{end}}" +
	"{{if .Global.SubnetworkName}}subnetwork-name = {{ .Global.SubnetworkName | iniEscape }}\n{{end}}" +
	"{{if .Global.SecondaryRangeName}}secondary-range-name = {{ .Global.SecondaryRangeName | iniEscape }}\n{{end}}" +
	"{{if .Global.NodeTags}}node-tags = {{ StringsJoin .Global.NodeTags \", \" }}\n{{end}}" +
	"{{if .Global.NodeInstancePrefix}}node-instance-prefix = {{ .Global.NodeInstancePrefix | iniEscape }}\n{{end}}" +
	"{{if .Global.Regional}}regional = {{ .Global.Regional }}\n{{end}}" +
	"{{if .Global.Multizone}}multizone = {{ .Global.Multizone }}\n{{end}}" +
	"{{if .Global.APIEndpoint}}api-endpoint = {{ .Global.APIEndpoint | iniEscape }}\n{{end}}" +
	"{{if .Global.ContainerAPIEndpoint}}container-api-endpoint = {{ .Global.ContainerAPIEndpoint | iniEscape }}\n{{end}}" +
	"{{if .Global.LocalZone}}local-zone = {{ .Global.LocalZone | iniEscape }}\n{{end}}" +
	"{{if .Global.AlphaFeatures}}alpha-features = {{ StringsJoin .Global.AlphaFeatures \", \" }}\n{{end}}"

//-----
// Cloud Configuration
//-----

// global contains the values of the global section of the cloud configuration.
type global struct {
	TokenURL             string
	TokenBody            string
	ProjectID            string
	NetworkProjectID     string
	NetworkName          string
	SubnetworkName       string
	SecondaryRangeName   string
	NodeTags             []string
	NodeInstancePrefix   string
	Regional             bool
	Multizone            bool
	APIEndpoint          string
	ContainerAPIEndpoint string
	LocalZone            string
	AlphaFeatures        []string
}

// cloudConfig contains only the section global.
type cloudConfig struct {
	Global global
}

// asString renders the cloud configuration as string.
func (cc *cloudConfig) asString() (string, error) {
	funcMap := sprig.TxtFuncMap()
	funcMap["iniEscape"] = ini.Escape
	funcMap["StringsJoin"] = strings.Join

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
