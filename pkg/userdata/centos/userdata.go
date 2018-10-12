package centos

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"text/template"

	"github.com/Masterminds/semver"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	machinetemplate "github.com/kubermatic/machine-controller/pkg/template"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
	userdatahelper "github.com/kubermatic/machine-controller/pkg/userdata/helper"

	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func getConfig(r runtime.RawExtension) (*Config, error) {
	p := Config{}
	if len(r.Raw) == 0 {
		return &p, nil
	}
	if err := json.Unmarshal(r.Raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Config TODO
type Config struct {
	DistUpgradeOnBoot bool `json:"distUpgradeOnBoot"`
}

// Provider is a pkg/userdata.Provider implementation
type Provider struct{}

// UserData renders user-data template
func (p Provider) UserData(
	spec clusterv1alpha1.MachineSpec,
	kubeconfig *clientcmdapi.Config,
	ccProvider cloud.ConfigProvider,
	clusterDNSIPs []net.IP,
) (string, error) {

	tmpl, err := template.New("user-data").Funcs(machinetemplate.TxtFuncMap()).Parse(ctTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse user-data template: %v", err)
	}

	kubeletVersion, err := semver.NewVersion(spec.Versions.Kubelet)
	if err != nil {
		return "", fmt.Errorf("invalid kubelet version: '%v'", err)
	}

	cpConfig, cpName, err := ccProvider.GetCloudConfig(spec)
	if err != nil {
		return "", fmt.Errorf("failed to get cloud config: %v", err)
	}

	pconfig, err := providerconfig.GetConfig(spec.ProviderConfig)
	if err != nil {
		return "", fmt.Errorf("failed to get provider config: %v", err)
	}

	if pconfig.OverwriteCloudConfig != nil {
		cpConfig = *pconfig.OverwriteCloudConfig
	}

	if pconfig.Network != nil {
		return "", errors.New("static IP config is not supported with CentOS")
	}

	osConfig, err := getConfig(pconfig.OperatingSystemSpec)
	if err != nil {
		return "", fmt.Errorf("failed to parse OperatingSystemSpec: '%v'", err)
	}

	bootstrapToken, err := userdatahelper.GetTokenFromKubeconfig(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting token: %v", err)
	}

	kubeadmCACertHash, err := userdatahelper.GetKubeadmCACertHash(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting kubeadm cacert hash: %v", err)
	}

	serverAddr, err := userdatahelper.GetServerAddressFromKubeconfig(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting server address from kubeconfig: %v", err)
	}

	data := struct {
		MachineSpec       clusterv1alpha1.MachineSpec
		ProviderConfig    *providerconfig.Config
		OSConfig          *Config
		BoostrapToken     string
		CloudProvider     string
		CloudConfig       string
		KubeletVersion    string
		ClusterDNSIPs     []net.IP
		KubeadmCACertHash string
		ServerAddr        string
		JournaldMaxSize   string
	}{
		MachineSpec:       spec,
		ProviderConfig:    pconfig,
		OSConfig:          osConfig,
		BoostrapToken:     bootstrapToken,
		CloudProvider:     cpName,
		CloudConfig:       cpConfig,
		KubeletVersion:    kubeletVersion.String(),
		ClusterDNSIPs:     clusterDNSIPs,
		KubeadmCACertHash: kubeadmCACertHash,
		ServerAddr:        serverAddr,
		JournaldMaxSize:   userdatahelper.JournaldMaxUse,
	}
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute user-data template: %v", err)
	}
	return b.String(), nil
}

const ctTemplate = `#cloud-config
hostname: {{ .MachineSpec.Name }}

{{- if .OSConfig.DistUpgradeOnBoot }}
package_upgrade: true
package_reboot_if_required: true
{{- end }}

ssh_pwauth: no

{{- if ne (len .ProviderConfig.SSHPublicKeys) 0 }}
ssh_authorized_keys:
{{- range .ProviderConfig.SSHPublicKeys }}
  - "{{ . }}"
{{- end }}
{{- end }}

write_files:
- path: "/etc/systemd/journald.conf.d/max_disk_use.conf"
  content: |
    [Journal]
    SystemMaxUse={{ .JournaldMaxSize }}

- path: "/etc/sysctl.d/k8s.conf"
  content: |
    net.bridge.bridge-nf-call-ip6tables = 1
    net.bridge.bridge-nf-call-iptables = 1
    kernel.panic_on_oops = 1
    kernel.panic = 10
    vm.overcommit_memory = 1

- path: "/etc/yum.repos.d/kubernetes.repo"
  content: |
    [kubernetes]
    name=Kubernetes
    baseurl=https://packages.cloud.google.com/yum/repos/kubernetes-el7-$basearch
    enabled=1
    gpgcheck=1
    repo_gpgcheck=1
    gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg

- path: /etc/sysconfig/selinux
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

- path: "/etc/sysconfig/kubelet-overwrite"
  content: |
    KUBELET_DNS_ARGS=
    KUBELET_EXTRA_ARGS=--authentication-token-webhook=true \
      {{- if .CloudProvider }}
      --cloud-provider={{ .CloudProvider }} \
      --cloud-config=/etc/kubernetes/cloud-config \
      {{- end}}
      --hostname-override={{ .MachineSpec.Name }} \
      --read-only-port=0 \
      --protect-kernel-defaults=true \
      --cluster-dns={{ ipSliceToCommaSeparatedString .ClusterDNSIPs }} \
      --cluster-domain=cluster.local

{{- if semverCompare "<1.11.0" .KubeletVersion }}
- path: "/etc/systemd/system/kubelet.service.d/20-extra.conf"
  content: |
    [Service]
    EnvironmentFile=/etc/sysconfig/kubelet
{{- end }}

- path: "/etc/kubernetes/cloud-config"
  content: |
{{ if ne .CloudConfig "" }}{{ .CloudConfig | indent 4 }}{{ end }}

- path: "/usr/local/bin/setup"
  permissions: "0755"
  content: |
    #!/bin/bash
    set -xeuo pipefail
    setenforce 0 || true
    sysctl --system

    yum install -y docker-1.13.1 \
      kubelet-{{ .KubeletVersion }} \
      kubeadm-{{ .KubeletVersion }} \
      ebtables \
      ethtool \
      nfs-utils \
      bash-completion \
      sudo

    cp /etc/sysconfig/kubelet-overwrite /etc/sysconfig/kubelet

    systemctl enable --now docker
    systemctl enable --now kubelet

    if ! [[ -e /etc/kubernetes/pki/ca.crt ]]; then
      kubeadm join \
        --token {{ .BoostrapToken }} \
        --discovery-token-ca-cert-hash sha256:{{ .KubeadmCACertHash }} \
        {{- if semverCompare ">=1.9.X" .KubeletVersion }}
        --ignore-preflight-errors=CRI \
        {{- end }}
        {{ .ServerAddr }}
    fi

    systemctl enable --now --no-block kubelet-healthcheck.service
    systemctl enable --now --no-block docker-healthcheck.service

- path: "/usr/local/bin/supervise.sh"
  permissions: "0755"
  content: |
    #!/bin/bash
    set -xeuo pipefail
    while ! "$@"; do
      sleep 1
    done

- path: "/etc/systemd/system/setup.service"
  content: |
    [Install]
    WantedBy=multi-user.target

    [Unit]
    Requires=network-online.target
    After=network-online.target

    [Service]
    Type=oneshot
    RemainAfterExit=true
    ExecStart=/usr/local/bin/supervise.sh /usr/local/bin/setup

- path: /etc/systemd/system/kubelet-healthcheck.service
  permissions: "0644"
  content: |
    [Unit]
    Requires=setup.service
    After=setup.service

    [Service]
    ExecStart=/usr/local/bin/health-monitor.sh kubelet

    [Install]
    WantedBy=multi-user.target

- path: /etc/systemd/system/docker-healthcheck.service
  permissions: "0644"
  content: |
    [Unit]
    Requires=setup.service
    After=setup.service

    [Service]
    ExecStart=/usr/local/bin/health-monitor.sh container-runtime

    [Install]
    WantedBy=multi-user.target

- path: /usr/local/bin/health-monitor.sh
  permissions: "0755"
  # This script is a slightly adjusted version of
  # https://github.com/kubernetes/kubernetes/blob/e1a1aa211224fcd9b213420b80b2ae680669683d/cluster/gce/gci/health-monitor.sh
  # Adjustments are:
  # * Kubelet health port is 10248 not 10255
  # * Removal of all all references to the KUBE_ENV file
  content: |
    #!/usr/bin/env bash

    # Copyright 2016 The Kubernetes Authors.
    #
    # Licensed under the Apache License, Version 2.0 (the "License");
    # you may not use this file except in compliance with the License.
    # You may obtain a copy of the License at
    #
    #     http://www.apache.org/licenses/LICENSE-2.0
    #
    # Unless required by applicable law or agreed to in writing, software
    # distributed under the License is distributed on an "AS IS" BASIS,
    # WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
    # See the License for the specific language governing permissions and
    # limitations under the License.

    # This script is for master and node instance health monitoring, which is
    # packed in kube-manifest tarball. It is executed through a systemd service
    # in cluster/gce/gci/<master/node>.yaml. The env variables come from an env
    # file provided by the systemd service.

    set -o nounset
    set -o pipefail

    # We simply kill the process when there is a failure. Another systemd service will
    # automatically restart the process.
    function container_runtime_monitoring {
      local -r max_attempts=5
      local attempt=1
      local -r crictl="${KUBE_HOME}/bin/crictl"
      local -r container_runtime_name="${CONTAINER_RUNTIME_NAME:-docker}"
      # We still need to use 'docker ps' when container runtime is "docker". This is because
      # dockershim is still part of kubelet today. When kubelet is down, crictl pods
      # will also fail, and docker will be killed. This is undesirable especially when
      # docker live restore is disabled.
      local healthcheck_command="docker ps"
      if [[ "${CONTAINER_RUNTIME:-docker}" != "docker" ]]; then
        healthcheck_command="${crictl} pods"
      fi
      # Container runtime startup takes time. Make initial attempts before starting
      # killing the container runtime.
      until timeout 60 ${healthcheck_command} > /dev/null; do
        if (( attempt == max_attempts )); then
          echo "Max attempt ${max_attempts} reached! Proceeding to monitor container runtime healthiness."
          break
        fi
        echo "$attempt initial attempt \"${healthcheck_command}\"! Trying again in $attempt seconds..."
        sleep "$(( 2 ** attempt++ ))"
      done
      while true; do
        if ! timeout 60 ${healthcheck_command} > /dev/null; then
          echo "Container runtime ${container_runtime_name} failed!"
          if [[ "$container_runtime_name" == "docker" ]]; then
              # Dump stack of docker daemon for investigation.
              # Log fle name looks like goroutine-stacks-TIMESTAMP and will be saved to
              # the exec root directory, which is /var/run/docker/ on Ubuntu and COS.
              pkill -SIGUSR1 dockerd
          fi
          systemctl kill --kill-who=main "${container_runtime_name}"
          # Wait for a while, as we don't want to kill it again before it is really up.
          sleep 120
        else
          sleep "${SLEEP_SECONDS}"
        fi
      done
    }

    function kubelet_monitoring {
      echo "Wait for 2 minutes for kubelet to be functional"
      # TODO(andyzheng0831): replace it with a more reliable method if possible.
      sleep 120
      local -r max_seconds=10
      local output=""
      while [ 1 ]; do
        if ! output=$(curl -m "${max_seconds}" -f -s -S http://127.0.0.1:10248/healthz 2>&1); then
          # Print the response and/or errors.
          echo $output
          echo "Kubelet is unhealthy!"
          systemctl kill kubelet
          # Wait for a while, as we don't want to kill it again before it is really up.
          sleep 60
        else
          sleep "${SLEEP_SECONDS}"
        fi
      done
    }


    ############## Main Function ################
    if [[ "$#" -ne 1 ]]; then
      echo "Usage: health-monitor.sh <container-runtime/kubelet>"
      exit 1
    fi

    KUBE_HOME="/home/kubernetes"

    SLEEP_SECONDS=10
    component=$1
    echo "Start kubernetes health monitoring for ${component}"
    if [[ "${component}" == "container-runtime" ]]; then
      container_runtime_monitoring
    elif [[ "${component}" == "kubelet" ]]; then
      kubelet_monitoring
    else
      echo "Health monitoring for component "${component}" is not supported!"
    fi

runcmd:
- systemctl enable --now setup.service
`
