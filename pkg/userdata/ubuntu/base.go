package ubuntu

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/kubermatic/machine-controller/pkg/userdata/helper"
)

// GetBaseProvisioningScript will return a bash script which will prepare the Ubuntu 18.04 for Kubernetes.
// All required binaries & services will be installed, but without cluster specifics like Kubeconfig, CA, etc.
func GetBaseProvisioningScript(cloudProviderName string, kubeletVersion string) (string, error) {
	files := []File{
		{
			Filename: "/etc/systemd/journald.conf.d/max_disk_use.conf",
			Content:  helper.JournalDConfig(),
		},
		{
			Filename: "/etc/modules-load.d/k8s.conf",
			Content:  helper.KernelModules(),
		},
		{
			Filename: "/etc/sysctl.d/k8s.conf",
			Content:  helper.KernelSettings(),
		},
		{
			Filename: "/etc/profile.d/opt-bin-path.sh",
			Content:  `export PATH="/opt/bin:$PATH"`,
		},
		{
			Filename: "/etc/systemd/system/kubelet-healthcheck.service",
			Content:  helper.KubeletHealthCheckSystemdUnit(),
		},
		{
			Filename: "/etc/systemd/system/docker-healthcheck.service",
			Content:  helper.ContainerRuntimeHealthCheckSystemdUnit(),
		},
		{
			Filename: "/etc/docker/daemon.json",
			Content:  helper.DockerDaemonConfig(),
		},
	}

	dockerSystemdUnit, err := helper.DockerSystemdUnit(true)
	if err != nil {
		return "", fmt.Errorf("failed to create docker systemd unit: %v", err)
	}

	files = append(files, File{
		Filename: "/etc/systemd/system/docker.service",
		Content:  dockerSystemdUnit,
	})

	tmpl, err := template.New("base-setup").Funcs(helper.TxtFuncMap()).Parse(baseProvisioningTpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse base provisioning script template: %v", err)
	}

	data := struct {
		CloudProvider  string
		Files          []File
		KubeletVersion string
	}{
		Files:          files,
		CloudProvider:  cloudProviderName,
		KubeletVersion: kubeletVersion,
	}
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute base provisioning script template: %v", err)
	}
	return b.String(), nil
}

type File struct {
	Filename string
	Content  string
	Mode     string
}

const (
	baseProvisioningTpl = `#!/bin/bash
set -xeuo pipefail

{{ range .Files }}
mkdir -p $(dirname {{ .Filename }})
cat >{{ .Filename }} <<EOL
{{ .Content  | replace "$" "\\$" }} 
EOL
{{ if .Mode -}}
chmod {{ .Mode }} {{ .Filename }}
{{ else -}}
chmod 644 {{ .Filename -}}
{{ end }}
{{ end }}

# As we added some modules and don't want to reboot, restart the service
systemctl restart systemd-modules-load.service
sysctl --system

apt-get update

# Make sure we always disable swap - Otherwise the kubelet won't start'.
systemctl mask swap.target
swapoff -a

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
  bridge-utils \
  libapparmor1 \
  libltdl7 \
  perl \
  ipvsadm{{ if eq .CloudProvider "vsphere" }} \
  open-vm-tools{{ end }}

{{ downloadBinariesScript .KubeletVersion true }}
`
)
