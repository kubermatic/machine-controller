package centos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"text/template"

	"github.com/Masterminds/semver"
	"github.com/kubermatic/machine-controller/pkg/containerruntime"
	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	machinetemplate "github.com/kubermatic/machine-controller/pkg/template"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
	"k8s.io/apimachinery/pkg/runtime"
)

type Provider struct{}

type Config struct {
	DistUpgradeOnBoot bool `json:"distUpgradeOnBoot"`
}

type packageCompatibilityMatrix struct {
	versions []string
	pkg      string
}

var dockerInstallCandidates = []packageCompatibilityMatrix{
	{
		versions: []string{"1.12", "1.12.6"},
		pkg:      "docker-1.12.6",
	},
	{
		versions: []string{"1.13", "1.13.1"},
		pkg:      "docker-1.13.1",
	},
}

func (p Provider) SupportedContainerRuntimes() (runtimes []machinesv1alpha1.ContainerRuntimeInfo) {
	for _, installCandidate := range dockerInstallCandidates {
		for _, v := range installCandidate.versions {
			runtimes = append(runtimes, machinesv1alpha1.ContainerRuntimeInfo{Name: containerruntime.Docker, Version: v})
		}
	}
	return runtimes
}

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

func getDockerPackageName(version string) (string, error) {
	for _, installCandidate := range dockerInstallCandidates {
		for _, v := range installCandidate.versions {
			if v == version {
				return installCandidate.pkg, nil
			}
		}
	}
	return "", fmt.Errorf("no package found for version '%s'", version)
}

func (p Provider) UserData(spec machinesv1alpha1.MachineSpec, kubeconfig string, ccProvider cloud.ConfigProvider, clusterDNSIPs []net.IP, kubernetesCACert, _ string) (string, error) {
	tmpl, err := template.New("user-data").Funcs(machinetemplate.TxtFuncMap()).Parse(ctTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse user-data template: %v", err)
	}

	semverKubeletVersion, err := semver.NewVersion(spec.Versions.Kubelet)
	if err != nil {
		return "", fmt.Errorf("invalid kubelet version: '%v'", err)
	}
	kubeletVersion := semverKubeletVersion.String()

	dockerPackageName, err := getDockerPackageName(spec.Versions.ContainerRuntime.Version)
	if err != nil {
		return "", fmt.Errorf("error getting Docker package name: '%v'", err)
	}

	cpConfig, cpName, err := ccProvider.GetCloudConfig(spec)
	if err != nil {
		return "", fmt.Errorf("failed to get cloud config: %v", err)
	}

	pconfig, err := providerconfig.GetConfig(spec.ProviderConfig)
	if err != nil {
		return "", fmt.Errorf("failed to get provider config: %v", err)
	}

	osConfig, err := getConfig(pconfig.OperatingSystemSpec)
	if err != nil {
		return "", fmt.Errorf("failed to parse OperatingSystemSpec: '%v'", err)
	}

	data := struct {
		MachineSpec       machinesv1alpha1.MachineSpec
		ProviderConfig    *providerconfig.Config
		OSConfig          *Config
		Kubeconfig        string
		CloudProvider     string
		CloudConfig       string
		KubeletVersion    string
		DockerPackageName string
		ClusterDNSIPs     []net.IP
		KubernetesCACert  string
	}{
		MachineSpec:       spec,
		ProviderConfig:    pconfig,
		OSConfig:          osConfig,
		Kubeconfig:        kubeconfig,
		CloudProvider:     cpName,
		CloudConfig:       cpConfig,
		KubeletVersion:    kubeletVersion,
		DockerPackageName: dockerPackageName,
		ClusterDNSIPs:     clusterDNSIPs,
		KubernetesCACert:  kubernetesCACert,
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

{{ if ne (len .ProviderConfig.SSHPublicKeys) 0 }}
ssh_authorized_keys:
{{- range .ProviderConfig.SSHPublicKeys }}
  - "{{ . }}"
{{- end }}
{{- end }}

write_files:
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

{{- if ne .CloudConfig "" }}
- path: "/etc/kubernetes/cloud-config"
  content: |
{{ .CloudConfig | indent 4 }}
{{- end }}

- path: "/etc/kubernetes/bootstrap.kubeconfig"
  content: |
{{ .Kubeconfig | indent 4 }}

- path: /etc/kubernetes/ca.crt
  content: |
{{ .KubernetesCACert | indent 4 }}

- path: "/etc/systemd/system/kubelet.service.d/10-machine-controller.conf"
  content: |
    [Unit]
    Description=Kubelet
    Requires=network-online.target docker.service
    After=docker.service network-online.target

    [Service]
    Restart=always
    RestartSec=10
    StartLimitInterval=600
    StartLimitBurst=50
    TimeoutStartSec=5min
    Environment="PATH=/sbin:/bin:/usr/sbin:/usr/bin:/opt/bin"
    ExecStart=
    ExecStart=/bin/kubelet \
      --container-runtime=docker \
      --cgroup-driver=systemd \
      --allow-privileged=true \
      --cni-bin-dir=/opt/cni/bin \
      --cni-conf-dir=/etc/cni/net.d \
      --cluster-dns={{ ipSliceToCommaSeparatedString .ClusterDNSIPs }} \
      --cluster-domain=cluster.local \
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
      --read-only-port 0 \
      --authorization-mode=Webhook \
      --anonymous-auth=false \
      --client-ca-file=/etc/kubernetes/ca.crt

    [Install]
    WantedBy=multi-user.target

runcmd:
- setenforce 0 || true
- chage -d $(date +%s) root
- systemctl enable --now kubelet

packages:
- {{ .DockerPackageName }}
- kubelet-{{ .KubeletVersion }}
- ebtables
- ethtool
- nfs-utils
- bash-completion # Have mercy for the poor operators
- sudo
`
