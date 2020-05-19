/*
Copyright 2019 The Machine Controller Authors.

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

//
// UserData plugin for Flatcar.
//

package flatcar

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/semver"

	"github.com/kubermatic/machine-controller/pkg/apis/plugin"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	userdatahelper "github.com/kubermatic/machine-controller/pkg/userdata/helper"
)

// Provider is a pkg/userdata/plugin.Provider implementation.
type Provider struct{}

// UserData renders user-data template to string.
func (p Provider) UserData(req plugin.UserDataRequest) (string, error) {

	tmpl, err := template.New("user-data").Funcs(userdatahelper.TxtFuncMap()).Parse(userDataTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse user-data template: %v", err)
	}

	kubeletVersion, err := semver.NewVersion(req.MachineSpec.Versions.Kubelet)
	if err != nil {
		return "", fmt.Errorf("invalid kubelet version: %v", err)
	}

	pconfig, err := providerconfigtypes.GetConfig(req.MachineSpec.ProviderSpec)
	if err != nil {
		return "", fmt.Errorf("failed to get provider config: %v", err)
	}

	if pconfig.OverwriteCloudConfig != nil {
		req.CloudConfig = *pconfig.OverwriteCloudConfig
	}

	flatcarConfig, err := LoadConfig(pconfig.OperatingSystemSpec)
	if err != nil {
		return "", fmt.Errorf("failed to get flatcar config from provider config: %v", err)
	}

	kubeconfigString, err := userdatahelper.StringifyKubeconfig(req.Kubeconfig)
	if err != nil {
		return "", err
	}

	kubernetesCACert, err := userdatahelper.GetCACert(req.Kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting cacert: %v", err)
	}

	// We need to reconfigure rkt to allow insecure registries in case the hyperkube image comes from an insecure registry
	var insecureHyperkubeImage bool
	for _, registry := range req.InsecureRegistries {
		if strings.Contains(req.HyperkubeImage, registry) {
			insecureHyperkubeImage = true
		}
	}

	if flatcarConfig.DisableAutoUpdate {
		flatcarConfig.DisableLocksmithD = true
		flatcarConfig.DisableUpdateEngine = true
	}

	data := struct {
		plugin.UserDataRequest
		ProviderSpec           *providerconfigtypes.Config
		FlatcarConfig          *Config
		Kubeconfig             string
		KubernetesCACert       string
		KubeletVersion         string
		InsecureHyperkubeImage bool
	}{
		UserDataRequest:        req,
		ProviderSpec:           pconfig,
		FlatcarConfig:          flatcarConfig,
		Kubeconfig:             kubeconfigString,
		KubernetesCACert:       kubernetesCACert,
		KubeletVersion:         kubeletVersion.String(),
		InsecureHyperkubeImage: insecureHyperkubeImage,
	}
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute user-data template: %v", err)
	}
	return userdatahelper.CleanupTemplateOutput(b.String())
}

// UserData template.
const userDataTemplate = `passwd:
{{- if ne (len .ProviderSpec.SSHPublicKeys) 0 }}
  users:
    - name: core
      ssh_authorized_keys:
        {{range .ProviderSpec.SSHPublicKeys}}- {{.}}
        {{end}}
{{- end }}

{{- if .ProviderSpec.Network }}
networkd:
  units:
    - name: static-nic.network
      contents: |
        [Match]
        # Because of difficulty predicting specific NIC names on different cloud providers,
        # we only support static addressing on VSphere. There should be a single NIC attached
        # that we will match by name prefix 'en' which denotes ethernet devices.
        Name=en*

        [Network]
        DHCP=no
        Address={{ .ProviderSpec.Network.CIDR }}
        Gateway={{ .ProviderSpec.Network.Gateway }}
        {{range .ProviderSpec.Network.DNS.Servers}}DNS={{.}}
        {{end}}
{{- end }}

systemd:
  units:
{{- if .FlatcarConfig.DisableUpdateEngine }}
    - name: update-engine.service
      mask: true
{{- end }}
{{- if .FlatcarConfig.DisableLocksmithD }}
    - name: locksmithd.service
      mask: true
{{- end }}
    - name: docker.service
      enabled: true

{{- if .HTTPProxy }}
    - name: update-engine.service
      dropins:
        - name: 50-proxy.conf
          contents: |
            [Service]
            Environment=ALL_PROXY={{ .HTTPProxy }}
{{- end }}

    - name: download-binaries-script.service
      enabled: true
      contents: |
        [Unit]
        Requires=network-online.target
        After=network-online.target
        [Service]
        Type=oneshot
        EnvironmentFile=-/etc/environment
        ExecStart=/opt/bin/download.sh
        [Install]
        WantedBy=multi-user.target

    - name: docker-healthcheck.service
      enabled: true
      dropins:
      - name: 40-docker.conf
        contents: |
          [Unit]
          Requires=download-binaries-script.service
          After=download-binaries-script.service
      contents: |
{{ containerRuntimeHealthCheckSystemdUnit | indent 10 }}

    - name: kubelet-healthcheck.service
      enabled: true
      dropins:
      - name: 40-docker.conf
        contents: |
          [Unit]
          Requires=download-binaries-script.service
          After=download-binaries-script.service
      contents: |
{{ kubeletHealthCheckSystemdUnit | indent 10 }}

    - name: kubelet.service
      enabled: true
      contents: |
{{ kubeletSystemdUnit .KubeletVersion .CloudProviderName .MachineSpec.Name .DNSIPs .ExternalCloudProvider .PauseImage .MachineSpec.Taints | indent 8 }}

    - name: docker.service
      enabled: true
      dropins:
      - name: 10-environment.conf
        contents: |
          [Service]
          EnvironmentFile=-/etc/environment

storage:
  files:
{{- if .HTTPProxy }}
    - path: /etc/environment
      filesystem: root
      mode: 0644
      contents:
        inline: |
{{ proxyEnvironment .HTTPProxy .NoProxy | indent 10 }}
{{- end }}

    - path: "/etc/systemd/journald.conf.d/max_disk_use.conf"
      filesystem: root
      mode: 0644
      contents:
        inline: |
{{ journalDConfig | indent 10 }}
    
    - path: "/etc/kubernetes/kubelet.conf"
      filesystem: root
      mode: 0644
      contents:
        inline: |
{{ kubeletConfiguration "cluster.local" .DNSIPs | indent 10 }}

    - path: /opt/load-kernel-modules.sh
      filesystem: root
      mode: 0755
      contents:
        inline: |
{{ kernelModulesScript | indent 10 }}

    - path: /etc/sysctl.d/k8s.conf
      filesystem: root
      mode: 0644
      contents:
        inline: |
{{ kernelSettings | indent 10 }}

    - path: /proc/sys/kernel/panic_on_oops
      filesystem: root
      mode: 0644
      contents:
        inline: |
          1

    - path: /proc/sys/kernel/panic
      filesystem: root
      mode: 0644
      contents:
        inline: |
          10

    - path: /proc/sys/vm/overcommit_memory
      filesystem: root
      mode: 0644
      contents:
        inline: |
          1

    - path: /etc/kubernetes/bootstrap-kubelet.conf
      filesystem: root
      mode: 0400
      contents:
        inline: |
{{ .Kubeconfig | indent 10 }}

    - path: /etc/kubernetes/cloud-config
      filesystem: root
      mode: 0400
      contents:
        inline: |
{{ .CloudConfig | indent 10 }}

    - path: /etc/kubernetes/pki/ca.crt
      filesystem: root
      mode: 0644
      contents:
        inline: |
{{ .KubernetesCACert | indent 10 }}
{{ if ne .CloudProviderName "aws" }}
    - path: /etc/hostname
      filesystem: root
      mode: 0600
      contents:
        inline: '{{ .MachineSpec.Name }}'
{{- end }}

    - path: /etc/ssh/sshd_config
      filesystem: root
      mode: 0600
      user:
        id: 0
      group:
        id: 0
      contents:
        inline: |
          # Use most defaults for sshd configuration.
          Subsystem sftp internal-sftp
          ClientAliveInterval 180
          UseDNS no
          UsePAM yes
          PrintLastLog no # handled by PAM
          PrintMotd no # handled by PAM
          PasswordAuthentication no
          ChallengeResponseAuthentication no

    - path: /etc/docker/daemon.json
      filesystem: root
      mode: 0644
      contents:
        inline: |
{{ dockerConfig .InsecureRegistries .RegistryMirrors | indent 10 }}

    - path: /opt/bin/download.sh
      filesystem: root
      mode: 0755
      contents:
        inline: |
          #!/bin/bash
          set -xeuo pipefail
{{ safeDownloadBinariesScript .KubeletVersion | indent 10 }}`
