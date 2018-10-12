package coreos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"text/template"

	"github.com/Masterminds/semver"
	ctconfig "github.com/coreos/container-linux-config-transpiler/config"
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
	DisableAutoUpdate bool `json:"disableAutoUpdate"`
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
		return "", fmt.Errorf("invalid kubelet version: %v", err)
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

	coreosConfig, err := getConfig(pconfig.OperatingSystemSpec)
	if err != nil {
		return "", fmt.Errorf("failed to get coreos config from provider config: %v", err)
	}

	kubeconfigString, err := userdatahelper.StringifyKubeconfig(kubeconfig)
	if err != nil {
		return "", err
	}

	kubernetesCACert, err := userdatahelper.GetCACert(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting cacert: %v", err)
	}

	data := struct {
		MachineSpec       clusterv1alpha1.MachineSpec
		ProviderConfig    *providerconfig.Config
		CoreOSConfig      *Config
		Kubeconfig        string
		CloudProvider     string
		CloudConfig       string
		HyperkubeImageTag string
		ClusterDNSIPs     []net.IP
		KubernetesCACert  string
		JournaldMaxSize   string
		KubeletVersion    string
	}{
		MachineSpec:       spec,
		ProviderConfig:    pconfig,
		CoreOSConfig:      coreosConfig,
		Kubeconfig:        kubeconfigString,
		CloudProvider:     cpName,
		CloudConfig:       cpConfig,
		HyperkubeImageTag: fmt.Sprintf("v%s", kubeletVersion.String()),
		ClusterDNSIPs:     clusterDNSIPs,
		KubernetesCACert:  kubernetesCACert,
		JournaldMaxSize:   userdatahelper.JournaldMaxUse,
		KubeletVersion:    kubeletVersion.String(),
	}
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute user-data template: %v", err)
	}

	// Convert to ignition
	cfg, ast, report := ctconfig.Parse(b.Bytes())
	if len(report.Entries) > 0 {
		return "", fmt.Errorf("failed to validate coreos cloud config: %s", report.String())
	}

	ignCfg, report := ctconfig.Convert(cfg, "", ast)
	if len(report.Entries) > 0 {
		return "", fmt.Errorf("failed to convert container linux config to ignition: %s", report.String())
	}

	out, err := json.MarshalIndent(ignCfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal ignition config: %v", err)
	}

	return string(out), nil
}

const ctTemplate = `
passwd:
  users:
    - name: core
      ssh_authorized_keys:
        {{range .ProviderConfig.SSHPublicKeys}}- {{.}}
        {{end}}

{{- if .ProviderConfig.Network }}
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
        Address={{ .ProviderConfig.Network.CIDR }}
        Gateway={{ .ProviderConfig.Network.Gateway }}
        {{range .ProviderConfig.Network.DNS.Servers}}DNS={{.}}
        {{end}}
{{- end }}

systemd:
  units:
{{- if .CoreOSConfig.DisableAutoUpdate }}
    - name: update-engine.service
      mask: true
    - name: locksmithd.service
      mask: true
{{ end }}
    - name: docker.service
      enabled: true

    - name: kubelet-healthcheck.service
      enabled: true
      contents: |
        [Unit]
        Requires=network-online.target
        After=network-online.target

        [Service]
        ExecStart=/opt/bin/health-monitor.sh kubelet

        [Install]
        WantedBy=multi-user.target

    - name: docker-healthcheck.service
      enabled: true
      contents: |
        [Unit]
        Requires=network-online.target
        After=network-online.target

        [Service]
        ExecStart=/opt/bin/health-monitor.sh container-runtime

        [Install]
        WantedBy=multi-user.target

    - name: kubelet.service
      enabled: true
      dropins:
      - name: 40-docker.conf
        contents: |
          [Unit]
          Requires=docker.service
          After=docker.service
      contents: |
        [Unit]
        Description=Kubernetes Kubelet
        Requires=docker.service
        After=docker.service
        [Service]
        TimeoutStartSec=5min
        Environment=KUBELET_IMAGE=docker://k8s.gcr.io/hyperkube-amd64:{{ .HyperkubeImageTag }}
        Environment="RKT_RUN_ARGS=--uuid-file-save=/var/cache/kubelet-pod.uuid \
          --insecure-options=image \
          --volume=resolv,kind=host,source=/etc/resolv.conf \
          --mount volume=resolv,target=/etc/resolv.conf \
          --volume cni-bin,kind=host,source=/opt/cni/bin \
          --mount volume=cni-bin,target=/opt/cni/bin \
          --volume cni-conf,kind=host,source=/etc/cni/net.d \
          --mount volume=cni-conf,target=/etc/cni/net.d \
          --volume etc-kubernetes,kind=host,source=/etc/kubernetes \
          --mount volume=etc-kubernetes,target=/etc/kubernetes \
          --volume var-log,kind=host,source=/var/log \
          --mount volume=var-log,target=/var/log \
          --volume var-lib-calico,kind=host,source=/var/lib/calico \
          --mount volume=var-lib-calico,target=/var/lib/calico"
        ExecStartPre=/bin/mkdir -p /var/lib/calico
        ExecStartPre=/bin/mkdir -p /etc/kubernetes/manifests
        ExecStartPre=/bin/mkdir -p /etc/cni/net.d
        ExecStartPre=/bin/mkdir -p /opt/cni/bin
        ExecStartPre=-/usr/bin/rkt rm --uuid-file=/var/cache/kubelet-pod.uuid
        ExecStart=/usr/lib/coreos/kubelet-wrapper \
          --container-runtime=docker \
          --allow-privileged=true \
          --cni-bin-dir=/opt/cni/bin \
          --cni-conf-dir=/etc/cni/net.d \
          --cluster-dns={{ ipSliceToCommaSeparatedString .ClusterDNSIPs }} \
          --cluster-domain=cluster.local \
          --authentication-token-webhook=true \
          --hostname-override={{ .MachineSpec.Name }} \
          --network-plugin=cni \
          {{- if .CloudProvider }}
          --cloud-provider={{ .CloudProvider }} \
          --cloud-config=/etc/kubernetes/cloud-config \
          {{- end }}
          --cert-dir=/etc/kubernetes/ \
          --pod-manifest-path=/etc/kubernetes/manifests \
          --resolv-conf=/etc/resolv.conf \
          --rotate-certificates=true \
          --kubeconfig=/etc/kubernetes/kubeconfig \
          --bootstrap-kubeconfig=/etc/kubernetes/bootstrap.kubeconfig \
          --lock-file=/var/run/lock/kubelet.lock \
          --exit-on-lock-contention \
          --read-only-port=0 \
          --protect-kernel-defaults=true \
          --authorization-mode=Webhook \
          --anonymous-auth=false \
          --client-ca-file=/etc/kubernetes/ca.crt
        ExecStop=-/usr/bin/rkt stop --uuid-file=/var/cache/kubelet-pod.uuid
        Restart=always
        RestartSec=10
        [Install]
        WantedBy=multi-user.target

storage:
  files:
    - path: "/etc/systemd/journald.conf.d/max_disk_use.conf"
      filesystem: root
      mode: 0644
      contents:
        inline: |
          [Journal]
          SystemMaxUse={{ .JournaldMaxSize }}

    - path: /etc/sysctl.d/k8s.conf
      filesystem: root
      mode: 0644
      contents:
        inline: |
          kernel.panic_on_oops = 1
          kernel.panic = 10
          vm.overcommit_memory = 1

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

    - path: /etc/kubernetes/bootstrap.kubeconfig
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

    - path: /etc/kubernetes/ca.crt
      filesystem: root
      mode: 0644
      contents:
        inline: |
{{ .KubernetesCACert | indent 10 }}

{{- if semverCompare "<=1.11.*" .KubeletVersion }}
    - path: /etc/coreos/docker-1.12
      mode: 0644
      filesystem: root
      contents:
        inline: |
          yes
{{ end }}

    - path: /etc/hostname
      filesystem: root
      mode: 0600
      contents:
        inline: '{{ .MachineSpec.Name }}'

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

    - path: /etc/systemd/system/docker.service.d/10-storage.conf
      filesystem: root
      mode: 0644
      contents:
        inline: |
          [Service]
          Environment=DOCKER_OPTS=--storage-driver=overlay2

    - path: /opt/bin/health-monitor.sh
      filesystem: root
      mode: 755
      # This script is a slightly adjusted version of
      # https://github.com/kubernetes/kubernetes/blob/e1a1aa211224fcd9b213420b80b2ae680669683d/cluster/gce/gci/health-monitor.sh
      # Adjustments are:
      # * Kubelet health port is 10248 not 10255
      # * Removal of all all references to the KUBE_ENV file
      contents:
        inline: |
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
`
