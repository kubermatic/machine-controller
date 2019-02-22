//
// Google Cloud Platform Provider for the Machine Controller
//

package gcp

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

const cloudConfigTemplate = `[global]
token-url = {{ .Global.TokenURL | iniEscape }}
token-body = {{ .Global.TokenBody | iniEscape }}
project-id = {{ .Global.ProjectID | iniEscape }}
network-project-id = {{ .Global.NetworkProjectID | iniEscape }}
network-name = {{ .Global.NetworkName | iniEscape }}
subnetwork-name = {{ .Global.SubnetworkName | iniEscape }}
secondary-range-name = {{ .Global.SecondaryRangeName | iniEscape }}
node-tags = {{ StringsJoin .Global.NodeTags ", " }}
node-instance-prefix = {{ .Global.NodeInstancePrefix | iniEscape }}
regional = {{ .Global.Regional }}
multizone = {{ .Global.Multizone }}
api-endpoint = {{ .Global.APIEndpoint | iniEscape }}
container-api-endpoint = {{ .Global.ContainerAPIEndpoint | iniEscape }}
local-zone = {{ .Global.LocalZone | iniEscape }}
alpha-features = {{ StringsJoin .Global.AlphaFeatures ", " }}
`

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
