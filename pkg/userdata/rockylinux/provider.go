/*
Copyright 2022 The Machine Controller Authors.

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
// UserData plugin for RockyLinux.
//

package rockylinux

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
		return "", fmt.Errorf("failed to parse user-data template: %w", err)
	}

	kubeletVersion, err := semver.NewVersion(req.MachineSpec.Versions.Kubelet)
	if err != nil {
		return "", fmt.Errorf("invalid kubelet version: %w", err)
	}

	pconfig, err := providerconfigtypes.GetConfig(req.MachineSpec.ProviderSpec)
	if err != nil {
		return "", fmt.Errorf("failed to get provider config: %w", err)
	}

	if pconfig.OverwriteCloudConfig != nil {
		req.CloudConfig = *pconfig.OverwriteCloudConfig
	}

	if pconfig.Network.IsStaticIPConfig() {
		return "", errors.New("static IP config is not supported with RockyLinux")
	}

	rockyLinuxConfig, err := LoadConfig(pconfig.OperatingSystemSpec)
	if err != nil {
		return "", fmt.Errorf("failed to parse OperatingSystemSpec: %w", err)
	}

	kubeconfigString, err := userdatahelper.StringifyKubeconfig(req.Kubeconfig)
	if err != nil {
		return "", err
	}

	kubernetesCACert, err := userdatahelper.GetCACert(req.Kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting cacert: %w", err)
	}

	crEngine := req.ContainerRuntime.Engine(kubeletVersion)
	crScript, err := crEngine.ScriptFor(providerconfigtypes.OperatingSystemRockyLinux)
	if err != nil {
		return "", fmt.Errorf("failed to generate container runtime install script: %w", err)
	}

	crConfig, err := crEngine.Config()
	if err != nil {
		return "", fmt.Errorf("failed to generate container runtime config: %w", err)
	}

	crAuthConfig, err := crEngine.AuthConfig()
	if err != nil {
		return "", fmt.Errorf("failed to generate container runtime auth config: %w", err)
	}

	data := struct {
		plugin.UserDataRequest
		ProviderSpec                       *providerconfigtypes.Config
		OSConfig                           *Config
		KubeletVersion                     string
		Kubeconfig                         string
		KubernetesCACert                   string
		NodeIPScript                       string
		ExtraKubeletFlags                  []string
		ContainerRuntimeScript             string
		ContainerRuntimeConfigFileName     string
		ContainerRuntimeConfig             string
		ContainerRuntimeAuthConfigFileName string
		ContainerRuntimeAuthConfig         string
		ContainerRuntimeName               string
	}{
		UserDataRequest:                    req,
		ProviderSpec:                       pconfig,
		OSConfig:                           rockyLinuxConfig,
		KubeletVersion:                     kubeletVersion.String(),
		Kubeconfig:                         kubeconfigString,
		KubernetesCACert:                   kubernetesCACert,
		NodeIPScript:                       userdatahelper.SetupNodeIPEnvScript(pconfig.Network.GetIPFamily()),
		ExtraKubeletFlags:                  crEngine.KubeletFlags(),
		ContainerRuntimeScript:             crScript,
		ContainerRuntimeConfigFileName:     crEngine.ConfigFileName(),
		ContainerRuntimeConfig:             crConfig,
		ContainerRuntimeAuthConfigFileName: crEngine.AuthConfigFileName(),
		ContainerRuntimeAuthConfig:         crAuthConfig,
		ContainerRuntimeName:               crEngine.String(),
	}

	buf := strings.Builder{}
	if err = tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute user-data template: %w", err)
	}

	return userdatahelper.CleanupTemplateOutput(buf.String())
}

// UserData template.
const userDataTemplate = `#cloud-config
bootcmd:
- modprobe ip_tables
{{ if ne .CloudProviderName "aws" }}
hostname: {{ .MachineSpec.Name }}
{{- /* Never set the hostname on AWS nodes. Kubernetes(kube-proxy) requires the hostname to be the private dns name */}}
{{ end }}

{{- if .OSConfig.DistUpgradeOnBoot }}
package_upgrade: true
package_reboot_if_required: true
{{- end }}

ssh_pwauth: false

{{- if ne (len .ProviderSpec.SSHPublicKeys) 0 }}
ssh_authorized_keys:
{{- range .ProviderSpec.SSHPublicKeys }}
  - "{{ . }}"
{{- end }}
{{- end }}

write_files:
{{- if .HTTPProxy }}
- path: "/etc/environment"
  content: |
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

- path: /etc/selinux/config
  content: |
    # This file controls the state of SELinux on the system.
    # SELINUX= can take one of these three values:
    #     enforcing - SELinux security policy is enforced.
    #     permissive - SELinux prints warnings instead of enforcing.
    #     disabled - No SELinux policy is loaded.
    SELINUX=permissive
    # SELINUXTYPE= can take one of three two values:
    #     targeted - Targeted processes are protected,
    #     minimum - Modification of targeted policy. Only selected processes are protected.
    #     mls - Multi Level Security protection.
    SELINUXTYPE=targeted

- path: "/opt/bin/setup"
  permissions: "0755"
  content: |
    #!/bin/bash
    set -xeuo pipefail

    setenforce 0 || true

{{- /* As we added some modules and don't want to reboot, restart the service */}}
    systemctl restart systemd-modules-load.service
    sysctl --system

    {{ if ne .CloudProviderName "aws" }}
{{- /*  The normal way of setting it via cloud-init is broken, see */}}
{{- /*  https://bugs.launchpad.net/cloud-init/+bug/1662542 */}}
    hostnamectl set-hostname {{ .MachineSpec.Name }}
    {{ end -}}

    yum install -y \
      device-mapper-persistent-data \
      lvm2 \
      ebtables \
      ethtool \
      nfs-utils \
      bash-completion \
      sudo \
      socat \
      wget \
      curl \
      tar \
      {{- if or (eq .CloudProviderName "vsphere") (eq .CloudProviderName "vmware-cloud-director") }}
      open-vm-tools \
      {{- end }}
      {{- if eq .CloudProviderName "nutanix" }}
      iscsi-initiator-utils \
      {{- end }}
      ipvsadm

    {{- /* iscsid service is required on Nutanix machines for CSI driver to attach volumes. */}}
    {{- if eq .CloudProviderName "nutanix" }}
    systemctl enable --now iscsid
    {{ end }}
{{ .ContainerRuntimeScript | indent 4 }}
{{ safeDownloadBinariesScript .KubeletVersion | indent 4 }}
    DEFAULT_IFC_NAME=$(ip -o route get 1  | grep -oP "dev \K\S+")
    IFC_CFG_FILE=/etc/sysconfig/network-scripts/ifcfg-$DEFAULT_IFC_NAME
    # Enable IPv6 and DHCPv6 on the default interface
    grep IPV6INIT $IFC_CFG_FILE && sed -i '/IPV6INIT*/c IPV6INIT=yes' $IFC_CFG_FILE || echo "IPV6INIT=yes" >> $IFC_CFG_FILE
    grep DHCPV6C $IFC_CFG_FILE && sed -i '/DHCPV6C*/c DHCPV6C=yes' $IFC_CFG_FILE || echo "DHCPV6C=yes" >> $IFC_CFG_FILE
    grep IPV6_AUTOCONF $IFC_CFG_FILE && sed -i '/IPV6_AUTOCONF*/c IPV6_AUTOCONF=yes' $IFC_CFG_FILE || echo "IPV6_AUTOCONF=yes" >> $IFC_CFG_FILE

    # Restart NetworkManager to apply for IPv6 configs
    systemctl restart NetworkManager
    # Let NetworkManager apply the DHCPv6 configs
    sleep 3

    # set kubelet nodeip environment variable
    mkdir -p /etc/systemd/system/kubelet.service.d/
    /opt/bin/setup_net_env.sh

    systemctl disable --now firewalld || true
    {{ if eq .CloudProviderName "vsphere" }}
    systemctl enable --now vmtoolsd.service
    {{ end -}}
    systemctl enable --now kubelet
    systemctl enable --now --no-block kubelet-healthcheck.service
    systemctl disable setup.service

- path: "/opt/bin/supervise.sh"
  permissions: "0755"
  content: |
    #!/bin/bash
    set -xeuo pipefail
    while ! "$@"; do
      sleep 1
    done

- path: "/opt/disable-swap.sh"
  permissions: "0755"
  content: |
    # Make sure we always disable swap - Otherwise the kubelet won't start as for some cloud
    # providers swap gets enabled on reboot or after the setup script has finished executing.
    sed -i.orig '/.*swap.*/d' /etc/fstab
    swapoff -a

- path: "/etc/systemd/system/kubelet.service"
  content: |
{{ kubeletSystemdUnit .ContainerRuntimeName .KubeletVersion .KubeletCloudProviderName .MachineSpec.Name .DNSIPs .ExternalCloudProvider .ProviderSpec.Network.GetIPFamily .PauseImage .MachineSpec.Taints .ExtraKubeletFlags true | indent 4 }}

{{- if ne (len .CloudConfig) 0 }}
- path: "/etc/kubernetes/cloud-config"
  permissions: "0600"
  content: |
{{ .CloudConfig | indent 4 }}
{{- end }}

- path: "/opt/bin/setup_net_env.sh"
  permissions: "0755"
  content: |
{{ .NodeIPScript | indent 4 }}

- path: "/etc/kubernetes/bootstrap-kubelet.conf"
  permissions: "0600"
  content: |
{{ .Kubeconfig | indent 4 }}

- path: "/etc/kubernetes/kubelet.conf"
  content: |
{{ kubeletConfiguration "cluster.local" .DNSIPs .KubeletFeatureGates .KubeletConfigs .ContainerRuntimeName | indent 4 }}

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

{{- if and (eq .ContainerRuntimeName "docker") .ContainerRuntimeAuthConfig }}

- path: {{ .ContainerRuntimeAuthConfigFileName }}
  permissions: "0600"
  content: |
{{ .ContainerRuntimeAuthConfig | indent 4 }}
{{- end }}

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
- systemctl enable --now setup.service
`
