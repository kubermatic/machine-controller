package ubuntu

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"text/template"

	"github.com/Masterminds/semver"

	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
	userdatahelper "github.com/kubermatic/machine-controller/pkg/userdata/helper"

	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

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

	tmpl, err := template.New("user-data").Funcs(userdatahelper.TxtFuncMap()).Parse(ctTemplate)
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

	if pconfig.Network != nil {
		return "", errors.New("static IP config is not supported with Ubuntu")
	}

	osConfig, err := getConfig(pconfig.OperatingSystemSpec)
	if err != nil {
		return "", fmt.Errorf("failed to get ubuntu config from provider config: %v", err)
	}

	serverAddr, err := userdatahelper.GetServerAddressFromKubeconfig(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting server address from kubeconfig: %v", err)
	}

	kubeconfigString, err := userdatahelper.StringifyKubeconfig(kubeconfig)
	if err != nil {
		return "", err
	}

	kubernetesCACert, err := userdatahelper.GetCACert(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting cacert: %v", err)
	}

	baseProvisioningScript, err := GetBaseProvisioningScript(cpName, kubeletVersion)
	if err != nil {
		return "", fmt.Errorf("error generating base provisioning script: %v", err)
	}

	data := struct {
		MachineSpec            clusterv1alpha1.MachineSpec
		ProviderConfig         *providerconfig.Config
		OSConfig               *Config
		CloudProvider          string
		CloudConfig            string
		ClusterDNSIPs          []net.IP
		ServerAddr             string
		KubeletVersion         string
		Kubeconfig             string
		KubernetesCACert       string
		BaseProvisioningScript string
	}{
		MachineSpec:            spec,
		ProviderConfig:         pconfig,
		OSConfig:               osConfig,
		CloudProvider:          cpName,
		CloudConfig:            cpConfig,
		ClusterDNSIPs:          clusterDNSIPs,
		ServerAddr:             serverAddr,
		KubeletVersion:         kubeletVersion.String(),
		Kubeconfig:             kubeconfigString,
		KubernetesCACert:       kubernetesCACert,
		BaseProvisioningScript: baseProvisioningScript,
	}
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute user-data template: %v", err)
	}

	return b.String(), nil
}

const ctTemplate = `#cloud-config
{{ if ne .CloudProvider "aws" }}
hostname: {{ .MachineSpec.Name }}
# Never set the hostname on AWS nodes. Kubernetes(kube-proxy) requires the hostname to be the private dns name
{{ end }}

ssh_pwauth: no

ssh_authorized_keys:
{{- range .ProviderConfig.SSHPublicKeys }}
- "{{ . }}"
{{- end }}

write_files:

- path: "/opt/bin/base-provisioning.sh"
  permissions: "0755"
  content: |
{{ .BaseProvisioningScript | indent 4 }}

- path: "/opt/bin/setup"
  permissions: "0755"
  content: |
    #!/bin/bash
    set -xeuo pipefail
    /opt/bin/base-provisioning.sh

    {{- if .OSConfig.DistUpgradeOnBoot }}
    DEBIAN_FRONTEND=noninteractive apt-get -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" dist-upgrade -y
    {{- end }}
    if [[ -e /var/run/reboot-required ]]; then
      reboot
    fi

{{ startAllUnits | indent 4 }}

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
{{ kubeletSystemdUnit .KubeletVersion .CloudProvider .MachineSpec.Name .ClusterDNSIPs | indent 4 }}

- path: "/etc/systemd/system/kubelet.service.d/extras.conf"
  content: |
    [Service]
    Environment="KUBELET_EXTRA_ARGS=--resolv-conf=/run/systemd/resolve/resolv.conf"

- path: "/etc/kubernetes/cloud-config"
  content: |
{{ .CloudConfig | indent 4 }}

- path: "/etc/kubernetes/bootstrap-kubelet.conf"
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
    ExecStart=/opt/bin/supervise.sh /opt/bin/setup

runcmd:
- systemctl enable --now setup.service
`
