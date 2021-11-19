/*
Copyright 2021 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"text/template"

	"github.com/Masterminds/sprig/v3"

	"github.com/kubermatic/machine-controller/pkg/apis/plugin"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/kubermatic/machine-controller/pkg/userdata/convert"
	"github.com/kubermatic/machine-controller/pkg/userdata/flatcar"

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func getOSMBootstrapUserdata(ctx context.Context, client ctrlruntimeclient.Client, req plugin.UserDataRequest, secretName string) (string, error) {

	var clusterName string
	for key := range req.Kubeconfig.Clusters {
		clusterName = key
	}

	token, err := util.ExtractAPIServerToken(ctx, client)
	if err != nil {
		return "", fmt.Errorf("failed to fetch api-server token: %v", err)
	}

	// Retrieve provider config from machine
	pconfig, err := providerconfigtypes.GetConfig(req.MachineSpec.ProviderSpec)
	if err != nil {
		return "", fmt.Errorf("failed to get providerSpec: %v", err)
	}

	// Ignition configuration is used for flatcar
	if useIgnition(pconfig) {
		return getOSMBootstrapUserDataForIgnition(ctx, req, pconfig.SSHPublicKeys, token, secretName, clusterName)
	}
	// cloud-init is used for all other operating systems
	return getOSMBootstrapUserDataForCloudInit(ctx, req, pconfig, token, secretName, clusterName)
}

// getOSMBootstrapUserDataForIgnition returns the userdata for the ignition bootstrap config
func getOSMBootstrapUserDataForIgnition(ctx context.Context, req plugin.UserDataRequest, sshPublicKeys []string, token, secretName, clusterName string) (string, error) {
	data := struct {
		Token      string
		SecretName string
		ServerURL  string
	}{
		Token:      token,
		SecretName: secretName,
		ServerURL:  req.Kubeconfig.Clusters[clusterName].Server,
	}
	bsScript, err := template.New("bootstrap-script").Parse(ignitionBootstrapBinContentTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse bootstrapBinContentTemplate template for ignition: %v", err)
	}
	script := &bytes.Buffer{}
	err = bsScript.Execute(script, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute bootstrapBinContentTemplate template for ignition: %v", err)
	}
	bsIgnitionConfig, err := template.New("bootstrap-ignition-config").Funcs(sprig.TxtFuncMap()).Parse(ignitionTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse bootstrap-ignition-config template: %v", err)
	}

	ignitionConfig := &bytes.Buffer{}
	err = bsIgnitionConfig.Execute(ignitionConfig, struct {
		Script        string
		Service       string
		SSHPublicKeys []string
		plugin.UserDataRequest
	}{
		Script:        script.String(),
		Service:       bootstrapServiceContentTemplate,
		SSHPublicKeys: sshPublicKeys,
		UserDataRequest: req,
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute ignitionTemplate template: %v", err)
	}

	return convert.ToIgnition(ignitionConfig.String())
}

// getOSMBootstrapUserDataForCloudInit returns the userdata for the cloud-init bootstrap script
func getOSMBootstrapUserDataForCloudInit(ctx context.Context, req plugin.UserDataRequest, pconfig *providerconfigtypes.Config, token, secretName, clusterName string) (string, error) {
	data := struct {
		Token       string
		SecretName  string
		ServerURL   string
		MachineName string
	}{
		Token:       token,
		SecretName:  secretName,
		ServerURL:   req.Kubeconfig.Clusters[clusterName].Server,
		MachineName: req.MachineSpec.Name,
	}

	var (
		bsScript *template.Template
		err      error
	)

	switch pconfig.OperatingSystem {
	case providerconfigtypes.OperatingSystemUbuntu:
		bsScript, err = template.New("bootstrap-cloud-init").Parse(bootstrapAptBinContentTemplate)
		if err != nil {
			return "", fmt.Errorf("failed to parse bootstrapAptBinContentTemplate template: %v", err)
		}
	case providerconfigtypes.OperatingSystemCentOS:
		bsScript, err = template.New("bootstrap-cloud-init").Parse(bootstrapYumBinContentTemplate)
		if err != nil {
			return "", fmt.Errorf("failed to parse bootstrapYumBinContentTemplate template: %v", err)
		}
	}

	script := &bytes.Buffer{}
	err = bsScript.Execute(script, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute bootstrap script template: %v", err)
	}
	bsCloudInit, err := template.New("bootstrap-cloud-init").Parse(cloudInitTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse download-binaries template: %v", err)
	}

	cloudInit := &bytes.Buffer{}
	err = bsCloudInit.Execute(cloudInit, struct {
		Script  string
		Service string
		plugin.UserDataRequest
		ProviderSpec *providerconfigtypes.Config
	}{
		Script:          base64.StdEncoding.EncodeToString(script.Bytes()),
		Service:         base64.StdEncoding.EncodeToString([]byte(bootstrapServiceContentTemplate)),
		UserDataRequest: req,
		ProviderSpec:    pconfig,
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute cloudInitTemplate template: %v", err)
	}
	return cloudInit.String(), nil
}

// cleanupTemplateOutput postprocesses the output of the template processing. Those
// may exist due to the working of template functions like those of the sprig package
// or template condition.
func cleanupTemplateOutput(output string) (string, error) {
	// Valid YAML files are not allowed to have empty lines containing spaces or tabs.
	// So far only cleanup.
	woBlankLines := regexp.MustCompile(`(?m)^[ \t]+$`).ReplaceAllString(output, "")
	return woBlankLines, nil
}

func useIgnition(p *providerconfigtypes.Config) bool {
	if p.OperatingSystem == providerconfigtypes.OperatingSystemFlatcar {
		config, err := flatcar.LoadConfig(p.OperatingSystemSpec)
		if err != nil {
			return false
		}
		return config.ProvisioningUtility == flatcar.Ignition
	}
	return false
}

const (
	bootstrapAptBinContentTemplate = `#!/bin/bash
set -xeuo pipefail
apt update && apt install -y curl jq
curl -s -k -v --header 'Authorization: Bearer {{ .Token }}'	{{ .ServerURL }}/api/v1/namespaces/cloud-init-settings/secrets/{{ .SecretName }} | jq '.data["cloud-config"]' -r| base64 -d > /etc/cloud/cloud.cfg.d/{{ .SecretName }}.cfg
cloud-init clean
cloud-init --file /etc/cloud/cloud.cfg.d/{{ .SecretName }}.cfg init
systemctl daemon-reload
systemctl restart setup.service
systemctl restart kubelet.service
systemctl restart kubelet-healthcheck.service
	`

	bootstrapYumBinContentTemplate = `#!/bin/bash
set -xeuo pipefail
yum install epel-release -y
yum install -y curl jq
curl -s -k -v --header 'Authorization: Bearer {{ .Token }}'	{{ .ServerURL }}/api/v1/namespaces/cloud-init-settings/secrets/{{ .SecretName }} | jq '.data["cloud-config"]' -r| base64 -d > /etc/cloud/cloud.cfg.d/{{ .SecretName }}.cfg
cloud-init clean
cloud-init --file /etc/cloud/cloud.cfg.d/{{ .SecretName }}.cfg init
systemctl daemon-reload
systemctl restart setup.service
systemctl restart kubelet.service
systemctl restart kubelet-healthcheck.service
	`

	bootstrapServiceContentTemplate = `[Install]
WantedBy=multi-user.target

[Unit]
Requires=network-online.target
After=network-online.target
[Service]
Type=oneshot
RemainAfterExit=true
ExecStart=/opt/bin/bootstrap
	`

	cloudInitTemplate = `#cloud-config
{{ if ne .CloudProviderName "aws" }}
hostname: {{ .MachineSpec.Name }}
{{- /* Never set the hostname on AWS nodes. Kubernetes(kube-proxy) requires the hostname to be the private dns name */}}
{{ end }}
ssh_pwauth: no

{{- if .ProviderSpec.SSHPublicKeys }}
ssh_authorized_keys:
{{- range .ProviderSpec.SSHPublicKeys }}
- "{{ . }}"
{{- end }}
{{- end }}

write_files:
- path: /opt/bin/bootstrap
  permissions: '0755'
  encoding: b64
  content: |
    {{ .Script }}
- path: /etc/systemd/system/bootstrap.service
  permissions: '0644'
  encoding: b64
  content: |
    {{ .Service }}
runcmd:
- systemctl restart bootstrap.service
- systemctl daemon-reload
`

	ignitionBootstrapBinContentTemplate = `#!/bin/bash
set -xeuo pipefail
apt update && apt install -y curl jq
curl -s -k -v --header 'Authorization: Bearer {{ .Token }}'	{{ .ServerURL }}/api/v1/namespaces/cloud-init-settings/secrets/{{ .SecretName }} | jq '.data["cloud-config"]' -r| base64 -d > /usr/share/oem/config.ign
touch /boot/flatcar/first_boot
systemctl disable bootstrap.service
rm /etc/systemd/system/bootstrap.service
rm /etc/machine-id
reboot
`

	ignitionTemplate = `passwd:
{{- if ne (len .SSHPublicKeys) 0 }}
  users:
    - name: core
      ssh_authorized_keys:
        {{range .SSHPublicKeys }}- {{.}}
        {{end}}
{{- end }}
storage:
  files:
  - path: /opt/bin/bootstrap
    mode: 0755
    filesystem: root
    contents:
      inline: |
{{ .Script | indent 10}}
{{ if ne .CloudProviderName "aws" }}
{{- /* Never set the hostname on AWS nodes. Kubernetes(kube-proxy) requires the hostname to be the private dns name */}}
  - path: /etc/hostname
    mode: 0600
    filesystem: root
    contents:
      inline: '{{ .MachineSpec.Name }}'
{{ end }}
systemd:
  units:
  - name: bootstrap.service
    enabled: true
    contents: |
{{ .Service | indent 10 }}
`
)
