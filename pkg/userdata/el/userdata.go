package el

import (
	"bytes"
	"fmt"
	"net"
	"text/template"

	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	machinetemplate "github.com/kubermatic/machine-controller/pkg/template"
	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
)

type Provider struct{}

func (p Provider) SupportedContainerRuntimes() (runtimes []machinesv1alpha1.ContainerRuntimeInfo) {
	return []machinesv1alpha1.ContainerRuntimeInfo{
		{
			Name:    "docker",
			Version: "1.13",
		},
	}
}

func (p Provider) UserData(spec machinesv1alpha1.MachineSpec, kubeconfig string, ccProvider cloud.ConfigProvider, clusterDNSIPs []net.IP) (string, error) {
	tmpl, err := template.New("user-data").Funcs(machinetemplate.TxtFuncMap()).Parse(ctTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse user-data template: %v", err)
	}

	cpConfig, cpName, err := ccProvider.GetCloudConfig(spec)
	if err != nil {
		return "", fmt.Errorf("failed to get cloud config: %v", err)
	}

	pconfig, err := providerconfig.GetConfig(spec.ProviderConfig)
	if err != nil {
		return "", fmt.Errorf("failed to get provider config: %v", err)
	}

	data := struct {
		MachineSpec    machinesv1alpha1.MachineSpec
		ProviderConfig *providerconfig.Config
		Kubeconfig     string
		CloudProvider  string
		CloudConfig    string
		ClusterDNSIPs  []net.IP
	}{
		MachineSpec:    spec,
		ProviderConfig: pconfig,
		Kubeconfig:     kubeconfig,
		CloudProvider:  cpName,
		CloudConfig:    cpConfig,
		ClusterDNSIPs:  clusterDNSIPs,
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

ssh_authorized_keys:
{{- range .ProviderConfig.SSHPublicKeys }}
- "{{ . }}"
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

- path: "/etc/systemd/system/kubelet.service"
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
    ExecStartPre=/etc/kubernetes/download.sh
    ExecStart=/opt/bin/kubelet \
      --container-runtime=docker \
      --cgroup-driver="systemd" \
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
      --authorization-mode=Webhook

    [Install]
    WantedBy=multi-user.target

bootcmd:
- sudo setenforce 0

packages:
- docker
- kubelet
- ebtables
- ethtool
- nfs-utils
- bash-completion # Have mercy for the poor operators
- sudo
`
