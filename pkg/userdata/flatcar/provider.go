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
	"text/template"

	"github.com/Masterminds/semver/v3"

	"github.com/kubermatic/machine-controller/pkg/apis/plugin"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/kubermatic/machine-controller/pkg/userdata/convert"
	userdatahelper "github.com/kubermatic/machine-controller/pkg/userdata/helper"
)

// Provider is a pkg/userdata/plugin.Provider implementation.
type Provider struct{}

// UserData renders user-data template to string.
func (p Provider) UserData(req plugin.UserDataRequest) (string, error) {
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

	userDataTemplate, err := getUserDataTemplate(flatcarConfig.ProvisioningUtility)
	if err != nil {
		return "", fmt.Errorf("failed to get an appropriate user-data template: %v", err)
	}

	tmpl, err := template.New("user-data").Funcs(userdatahelper.TxtFuncMap()).Parse(userDataTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse user-data template: %v", err)
	}

	kubeletVersion, err := semver.NewVersion(req.MachineSpec.Versions.Kubelet)
	if err != nil {
		return "", fmt.Errorf("invalid kubelet version: %v", err)
	}

	kubeconfigString, err := userdatahelper.StringifyKubeconfig(req.Kubeconfig)
	if err != nil {
		return "", err
	}

	kubernetesCACert, err := userdatahelper.GetCACert(req.Kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting cacert: %v", err)
	}

	if flatcarConfig.DisableAutoUpdate {
		flatcarConfig.DisableLocksmithD = true
		flatcarConfig.DisableUpdateEngine = true
	}

	crEngine := req.ContainerRuntime.Engine(kubeletVersion)
	crScript, err := crEngine.ScriptFor(providerconfigtypes.OperatingSystemFlatcar)
	if err != nil {
		return "", fmt.Errorf("failed to generate container runtime install script: %w", err)
	}

	crConfig, err := crEngine.Config()
	if err != nil {
		return "", fmt.Errorf("failed to generate container runtime config: %w", err)
	}

	data := struct {
		plugin.UserDataRequest
		ProviderSpec                   *providerconfigtypes.Config
		FlatcarConfig                  *Config
		Kubeconfig                     string
		KubernetesCACert               string
		KubeletVersion                 string
		NodeIPScript                   string
		ExtraKubeletFlags              []string
		ContainerRuntimeScript         string
		ContainerRuntimeConfigFileName string
		ContainerRuntimeConfig         string
		ContainerRuntimeName           string
	}{
		UserDataRequest:                req,
		ProviderSpec:                   pconfig,
		FlatcarConfig:                  flatcarConfig,
		Kubeconfig:                     kubeconfigString,
		KubernetesCACert:               kubernetesCACert,
		KubeletVersion:                 kubeletVersion.String(),
		NodeIPScript:                   userdatahelper.SetupNodeIPEnvScript(),
		ExtraKubeletFlags:              crEngine.KubeletFlags(),
		ContainerRuntimeScript:         crScript,
		ContainerRuntimeConfigFileName: crEngine.ConfigFileName(),
		ContainerRuntimeConfig:         crConfig,
		ContainerRuntimeName:           crEngine.String(),
	}

	b := &bytes.Buffer{}
	err = tmpl.Execute(b, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute user-data template: %v", err)
	}

	out, err := userdatahelper.CleanupTemplateOutput(b.String())
	if err != nil {
		return "", fmt.Errorf("failed to cleanup user-data template: %v", err)
	}

	if flatcarConfig.ProvisioningUtility == CloudInit {
		return out, nil
	}

	return convert.ToIgnition(out)
}

func getUserDataTemplate(pUtil ProvisioningUtility) (string, error) {
	switch pUtil {
	case Ignition, "":
		return userDataIgnitionTemplate, nil
	case CloudInit:
		return userDataCloudInitTemplate, nil
	default:
		return "", fmt.Errorf("invalid provisioning utility %s, allowed values are %s or %s",
			pUtil, Ignition, CloudInit)
	}
}

// Ignition template.
const userDataIgnitionTemplate = `passwd:
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

{{- if .HTTPProxy }}
    - name: update-engine.service
      dropins:
        - name: 50-proxy.conf
          contents: |
            [Service]
            Environment=ALL_PROXY={{ .HTTPProxy }}
{{- end }}

    - name: download-script.service
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

    - name: kubelet-healthcheck.service
      enabled: true
      dropins:
      - name: 40-download.conf
        contents: |
          [Unit]
          Requires=download-script.service
          After=download-script.service
      contents: |
{{ kubeletHealthCheckSystemdUnit | indent 10 }}

    - name: nodeip.service
      enabled: true
      contents: |
        [Unit]
        Description=Setup Kubelet Node IP Env
        Requires=network-online.target
        After=network-online.target

        [Service]
        ExecStart=/opt/bin/setup_net_env.sh
        RemainAfterExit=yes
        Type=oneshot
        [Install]
        WantedBy=multi-user.target

    - name: kubelet.service
      enabled: true
      dropins:
      - name: 10-nodeip.conf
        contents: |
          [Service]
          EnvironmentFile=/etc/kubernetes/nodeip.conf
      - name: 40-download.conf
        contents: |
          [Unit]
          Requires=download-script.service
          After=download-script.service
      contents: |
{{ kubeletSystemdUnit .ContainerRuntimeName .KubeletVersion .KubeletCloudProviderName .MachineSpec.Name .DNSIPs .ExternalCloudProvider .PauseImage .MachineSpec.Taints .ExtraKubeletFlags | indent 8 }}

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
{{ kubeletConfiguration "cluster.local" .DNSIPs .KubeletFeatureGates .KubeletConfigs | indent 10 }}

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

    - path: "/opt/bin/setup_net_env.sh"
      filesystem: root
      mode: 0755
      contents:
        inline: |
{{ .NodeIPScript | indent 10 }}

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

{{- if not .FlatcarConfig.DisableAutoUpdate }}
    - path: "/etc/polkit-1/rules.d/60-noreboot_norestart.rules"
      filesystem: root
      mode: 0644
      contents:
        inline: |
          polkit.addRule(function(action, subject) {
              if (action.id == "org.freedesktop.login1.reboot" ||
                  action.id == "org.freedesktop.login1.reboot-multiple-sessions") {
                  if (subject.user == "core") {
                      return polkit.Result.YES;
                  } else {
                      return polkit.Result.AUTH_ADMIN;
                  }
              }
          });
{{- end }}

    - path: /opt/bin/download.sh
      filesystem: root
      mode: 0755
      contents:
        inline: |
          #!/bin/bash
          set -xeuo pipefail

{{ safeDownloadBinariesScript .KubeletVersion | indent 10 }}
          mkdir -p /etc/systemd/system/containerd.service.d /etc/systemd/system/docker.service.d
          cat <<EOF | tee /etc/systemd/system/containerd.service.d/environment.conf /etc/systemd/system/docker.service.d/environment.conf
          [Service]
          Restart=always
          EnvironmentFile=-/etc/environment
          EOF
{{ .ContainerRuntimeScript | indent 10 }}
          systemctl disable download-script.service

    - path: {{ .ContainerRuntimeConfigFileName }}
      filesystem: root
      mode: 0644
      contents:
        inline: |
{{ .ContainerRuntimeConfig | indent 10 }}

    - path: /etc/crictl.yaml
      filesystem: root
      mode: 0644
      contents:
        inline: |
          runtime-endpoint: unix:///run/containerd/containerd.sock
`

// Coreos cloud-config template
const userDataCloudInitTemplate = `#cloud-config

users:
{{- if ne (len .ProviderSpec.SSHPublicKeys) 0 }}
- name: core
  ssh_authorized_keys:
  {{range .ProviderSpec.SSHPublicKeys}}- {{.}}
  {{end}}
{{- end }}

coreos:
  units:
{{- if .ProviderSpec.Network }}
  - name: static-nic.network
    content: |
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
{{- if .FlatcarConfig.DisableUpdateEngine }}
  - name: update-engine.service
    mask: true
{{- end }}
{{- if .FlatcarConfig.DisableLocksmithD }}
  - name: locksmithd.service
    mask: true
{{- end }}
{{- if .HTTPProxy }}
  - name: update-engine.service
    drop-ins:
    - name: 50-proxy.conf
      content: |
        [Service]
        Environment=ALL_PROXY={{ .HTTPProxy }}
{{- end }}
  - name: download-script.service
    enable: true
    command: start
    content: |
      [Unit]
      Requires=network-online.target
      After=network-online.target
      [Service]
      Type=oneshot
      EnvironmentFile=-/etc/environment
      ExecStart=/opt/bin/download.sh
      [Install]
      WantedBy=multi-user.target

  - name: kubelet-healthcheck.service
    enable: true
    command: start
    drop-ins:
    - name: 40-download.conf
      content: |
        [Unit]
        Requires=download-script.service
        After=download-script.service
    content: |
{{ kubeletHealthCheckSystemdUnit | indent 6 }}

  - name: nodeip.service
    enable: true
    command: start
    content: |
      [Unit]
      Description=Setup Kubelet Node IP Env
      Requires=network-online.target
      After=network-online.target

      [Service]
      ExecStart=/opt/bin/setup_net_env.sh
      RemainAfterExit=yes
      Type=oneshot
      [Install]
      WantedBy=multi-user.target

  - name: kubelet.service
    enable: true
    command: start
    drop-ins:
    - name: 10-nodeip.conf
      content: |
        [Service]
        EnvironmentFile=/etc/kubernetes/nodeip.conf
    - name: 40-download.conf
      content: |
        [Unit]
        Requires=download-script.service
        After=download-script.service
    content: |
{{ kubeletSystemdUnit .ContainerRuntimeName .KubeletVersion .KubeletCloudProviderName .MachineSpec.Name .DNSIPs .ExternalCloudProvider .PauseImage .MachineSpec.Taints .ExtraKubeletFlags | indent 6 }}

  - name: apply-sysctl-settings.service
    enable: true
    command: start
    content: |
      [Unit]
      Requires=network-online.target
      After=network-online.target
      [Service]
      Type=oneshot
      ExecStart=/opt/bin/apply_sysctl_settings.sh
      [Install]
      WantedBy=multi-user.target

write_files:
{{- if .HTTPProxy }}
- path: /etc/environment
  permissions: "0644"
  content: |
{{ proxyEnvironment .HTTPProxy .NoProxy | indent 4 }}
{{- end }}

- path: "/etc/systemd/journald.conf.d/max_disk_use.conf"
  permissions: "0644"
  content: |
{{ journalDConfig | indent 4 }}

- path: "/etc/kubernetes/kubelet.conf"
  permissions: "0644"
  content: |
{{ kubeletConfiguration "cluster.local" .DNSIPs .KubeletFeatureGates .KubeletConfigs | indent 4 }}

- path: /opt/load-kernel-modules.sh
  permissions: "0755"
  content: |
{{ kernelModulesScript | indent 4 }}

- path: /etc/sysctl.d/k8s.conf
  permissions: "0644"
  content: |
{{ kernelSettings | indent 4 }}

- path: "/opt/bin/setup_net_env.sh"
  permissions: "0755"
  content: |
{{ .NodeIPScript | indent 4 }}

- path: /etc/kubernetes/bootstrap-kubelet.conf
  permissions: "0400"
  content: |
{{ .Kubeconfig | indent 4 }}

- path: /etc/kubernetes/cloud-config
  permissions: "0400"
  content: |
{{ .CloudConfig | indent 4 }}

- path: /etc/kubernetes/pki/ca.crt
  permissions: "0644"
  content: |
{{ .KubernetesCACert | indent 4 }}

{{ if ne .CloudProviderName "aws" }}
- path: /etc/hostname
  permissions: "0600"
  content: '{{ .MachineSpec.Name }}'
{{- end }}

- path: /etc/ssh/sshd_config
  permissions: "0600"
  user: root
  content: |
    # Use most defaults for sshd configuration.
    Subsystem sftp internal-sftp
    ClientAliveInterval 180
    UseDNS no
    UsePAM yes
    PrintLastLog no # handled by PAM
    PrintMotd no # handled by PAM
    PasswordAuthentication no
    ChallengeResponseAuthentication no

{{- if not .FlatcarConfig.DisableAutoUpdate }}
- path: "/etc/polkit-1/rules.d/60-noreboot_norestart.rules"
  permissions: "0644"
  content: |
    polkit.addRule(function(action, subject) {
        if (action.id == "org.freedesktop.login1.reboot" ||
            action.id == "org.freedesktop.login1.reboot-multiple-sessions") {
            if (subject.user == "core") {
                return polkit.Result.YES;
            } else {
                return polkit.Result.AUTH_ADMIN;
            }
        }
    });
{{- end }}

- path: /opt/bin/download.sh
  permissions: "0755"
  content: |
    #!/bin/bash
    set -xeuo pipefail
{{ safeDownloadBinariesScript .KubeletVersion | indent 4 }}
{{ .ContainerRuntimeScript | indent 4 }}
    systemctl disable download-script.service

- path: /opt/bin/apply_sysctl_settings.sh
  permissions: "0755"
  user: root
  content: |
    #!/bin/bash
    set -xeuo pipefail
    sysctl --system
    systemctl disable apply-sysctl-settings.service

{{- with .ProviderSpec.CAPublicKey }}

- path: "/etc/ssh/trusted-user-ca-keys.pem"
  content: |
{{ . | indent 4 }}

- path: "/etc/ssh/sshd_config"
  content: |
{{ sshConfigAddendum | indent 4 }}
  append: true
{{- end }}

- path: {{ .ContainerRuntimeConfigFileName }}
  permissions: "0644"
  user: root
  content: |
{{ .ContainerRuntimeConfig | indent 4 }}

- path: /etc/crictl.yaml
  permissions: "0644"
  user: root
  content: |
    runtime-endpoint: unix:///run/containerd/containerd.sock
`
