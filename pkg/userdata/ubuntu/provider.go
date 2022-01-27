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
// UserData plugin for Ubuntu.
//

package ubuntu

import (
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/semver/v3"

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
		return "", fmt.Errorf("failed to get providerSpec: %v", err)
	}

	if pconfig.OverwriteCloudConfig != nil {
		req.CloudConfig = *pconfig.OverwriteCloudConfig
	}

	if pconfig.Network != nil {
		return "", errors.New("static IP config is not supported with Ubuntu")
	}

	ubuntuConfig, err := LoadConfig(pconfig.OperatingSystemSpec)
	if err != nil {
		return "", fmt.Errorf("failed to get ubuntu config from provider config: %v", err)
	}

	serverAddr, err := userdatahelper.GetServerAddressFromKubeconfig(req.Kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting server address from kubeconfig: %v", err)
	}

	kubeconfigString, err := userdatahelper.StringifyKubeconfig(req.Kubeconfig)
	if err != nil {
		return "", err
	}

	kubernetesCACert, err := userdatahelper.GetCACert(req.Kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting cacert: %v", err)
	}

	crEngine := req.ContainerRuntime.Engine(kubeletVersion)
	crScript, err := crEngine.ScriptFor(providerconfigtypes.OperatingSystemUbuntu)
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
		OSConfig                       *Config
		ServerAddr                     string
		KubeletVersion                 string
		Kubeconfig                     string
		KubernetesCACert               string
		NodeIPScript                   string
		ExtraKubeletFlags              []string
		ContainerRuntimeScript         string
		ContainerRuntimeConfigFileName string
		ContainerRuntimeConfig         string
		ContainerRuntimeName           string
	}{
		UserDataRequest:                req,
		ProviderSpec:                   pconfig,
		OSConfig:                       ubuntuConfig,
		ServerAddr:                     serverAddr,
		KubeletVersion:                 kubeletVersion.String(),
		Kubeconfig:                     kubeconfigString,
		KubernetesCACert:               kubernetesCACert,
		NodeIPScript:                   userdatahelper.SetupNodeIPEnvScript(),
		ExtraKubeletFlags:              crEngine.KubeletFlags(),
		ContainerRuntimeScript:         crScript,
		ContainerRuntimeConfigFileName: crEngine.ConfigFileName(),
		ContainerRuntimeConfig:         crConfig,
		ContainerRuntimeName:           crEngine.String(),
	}

	var buf strings.Builder
	if err = tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute user-data template: %w", err)
	}

	return userdatahelper.CleanupTemplateOutput(buf.String())
}

// UserData template.
const userDataTemplate = `#cloud-config
{{ if ne .CloudProviderName "aws" }}
hostname: {{ .MachineSpec.Name }}
{{- /* Never set the hostname on AWS nodes. Kubernetes(kube-proxy) requires the hostname to be the private dns name */}}
{{ end }}

{{- if .OSConfig.DistUpgradeOnBoot }}
package_upgrade: true
package_reboot_if_required: true
{{- end }}

ssh_pwauth: false

{{- if .ProviderSpec.SSHPublicKeys }}
ssh_authorized_keys:
{{- range .ProviderSpec.SSHPublicKeys }}
- "{{ . }}"
{{- end }}
{{- end }}

write_files:
{{- if .HTTPProxy }}
- path: "/etc/environment"
  content: |
    PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games"
{{ proxyEnvironment .HTTPProxy .NoProxy | indent 4 }}
{{- end }}

- path: "/etc/systemd/journald.conf.d/max_disk_use.conf"
  content: |
{{ journalDConfig | indent 4 }}

- path: "/opt/load-kernel-modules.sh"
  permissions: "0755"
  content: |
{{ kernelModulesScript | indent 4 }}

- path: "/etc/sysctl.d/k8s.conf"
  content: |
{{ kernelSettings | indent 4 }}

- path: "/etc/default/grub.d/60-swap-accounting.cfg"
  content: |
    # Added by kubermatic machine-controller
    # Enable cgroups memory and swap accounting
    GRUB_CMDLINE_LINUX="cgroup_enable=memory swapaccount=1"

- path: "/opt/bin/setup"
  permissions: "0755"
  content: |
    #!/bin/bash
    set -xeuo pipefail
    if systemctl is-active ufw; then systemctl stop ufw; fi
    systemctl mask ufw

{{- /* As we added some modules and don't want to reboot, restart the service */}}
    systemctl restart systemd-modules-load.service
    sysctl --system

{{- /* Make sure we always disable swap - Otherwise the kubelet won't start'. */}}
    sed -i.orig '/.*swap.*/d' /etc/fstab
    swapoff -a

    apt-get update

    DEBIAN_FRONTEND=noninteractive apt-get -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install -y \
      curl \
      ca-certificates \
      ceph-common \
      cifs-utils \
      conntrack \
      e2fsprogs \
      ebtables \
      ethtool \
      glusterfs-client \
      iptables \
      jq \
      kmod \
      openssh-client \
      nfs-common \
      socat \
      util-linux \
      {{- if eq .CloudProviderName "vsphere" }}
      open-vm-tools \
      {{- end }}
      {{- if eq .CloudProviderName "nutanix" }}
      open-iscsi \
      {{- end }}
      ipvsadm

    {{- /* iscsid service is required on Nutanix machines for CSI driver to attach volumes. */}}
    {{- if eq .CloudProviderName "nutanix" }}
    systemctl enable --now iscsid
    {{ end }}

    # Update grub to include kernel command options to enable swap accounting.
    # Exclude alibaba cloud until this is fixed https://github.com/kubermatic/machine-controller/issues/682
    {{ if eq .CloudProviderName "alibaba" }}
    if grep -v -q swapaccount=1 /proc/cmdline
    then
      echo "Reboot system if not alibaba cloud"
      update-grub
      touch /var/run/reboot-required
    fi
    {{ end }}
{{ .ContainerRuntimeScript | indent 4 }}

{{ safeDownloadBinariesScript .KubeletVersion | indent 4 }}
    # set kubelet nodeip environment variable
    /opt/bin/setup_net_env.sh

    systemctl enable --now kubelet
    systemctl enable --now --no-block kubelet-healthcheck.service

- path: "/opt/bin/supervise.sh"
  permissions: "0755"
  content: |
    #!/bin/bash
    set -xeuo pipefail
    while ! "$@"; do
      sleep 1
    done

- path: "/etc/systemd/system/kubelet.service"
  content: |
{{ kubeletSystemdUnit .ContainerRuntimeName .KubeletVersion .KubeletCloudProviderName .MachineSpec.Name .DNSIPs .ExternalCloudProvider .PauseImage .MachineSpec.Taints .ExtraKubeletFlags | indent 4 }}

- path: "/etc/systemd/system/kubelet.service.d/extras.conf"
  content: |
    [Service]
    Environment="KUBELET_EXTRA_ARGS=--resolv-conf=/run/systemd/resolve/resolv.conf"

- path: "/etc/kubernetes/cloud-config"
  permissions: "0600"
  content: |
{{ .CloudConfig | indent 4 }}

- path: "/opt/bin/setup_net_env.sh"
  permissions: "0755"
  content: |
{{ .NodeIPScript | indent 4 }}

- path: "/etc/kubernetes/bootstrap-kubelet.conf"
  permissions: "0600"
  content: |
{{ .Kubeconfig | indent 4 }}

- path: "/etc/kubernetes/pki/ca.crt"
  content: |
{{ .KubernetesCACert | indent 4 }}

- path: "/etc/systemd/system/setup.service"
  permissions: "0644"
  content: |
    [Install]
    WantedBy=multi-user.target

    [Unit]
    Requires=network-online.target
    After=network-online.target

    [Service]
    Type=oneshot
    RemainAfterExit=true
    EnvironmentFile=-/etc/environment
    ExecStart=/opt/bin/supervise.sh /opt/bin/setup

- path: "/etc/profile.d/opt-bin-path.sh"
  permissions: "0644"
  content: |
    export PATH="/opt/bin:$PATH"

- path: {{ .ContainerRuntimeConfigFileName }}
  permissions: "0644"
  content: |
{{ .ContainerRuntimeConfig | indent 4 }}

- path: "/etc/kubernetes/kubelet.conf"
  content: |
{{ kubeletConfiguration "cluster.local" .DNSIPs .KubeletFeatureGates .KubeletConfigs .ContainerRuntimeName | indent 4 }}

- path: /etc/systemd/system/kubelet-healthcheck.service
  permissions: "0644"
  content: |
{{ kubeletHealthCheckSystemdUnit | indent 4 }}

{{- with .ProviderSpec.CAPublicKey }}

- path: "/etc/ssh/trusted-user-ca-keys.pem"
  content: |
{{ . | indent 4 }}

- path: "/etc/ssh/sshd_config"
  content: |
{{ sshConfigAddendum | indent 4 }}
  append: true
{{- end }}

runcmd:
- systemctl start setup.service
`
