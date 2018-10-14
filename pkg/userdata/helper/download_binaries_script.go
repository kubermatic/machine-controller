package helper

import (
	"bytes"
	"fmt"
	"text/template"
)

const (
	downloadBinariesTpl = `#setup some common directories
mkdir -p /opt/bin/
mkdir -p /var/lib/calico
mkdir -p /etc/kubernetes/manifests
mkdir -p /etc/cni/net.d
mkdir -p /opt/cni/bin

# cni
if [ ! -f /opt/cni/bin/loopback ]; then
    curl -L https://github.com/containernetworking/plugins/releases/download/v0.6.0/cni-plugins-amd64-v0.6.0.tgz | tar -xvzC /opt/cni/bin -f -
fi

{{- if .DownloadKubelet }}
# kubelet
if [ ! -f /opt/bin/kubelet ]; then
    curl -Lfo /opt/bin/kubelet https://storage.googleapis.com/kubernetes-release/release/v{{ .KubeletVersion }}/bin/linux/amd64/kubelet
    chmod +x /opt/bin/kubelet
fi
{{- end }}

if [[ ! -x /opt/bin/health-monitor.sh ]]; then
    curl -Lfo /opt/bin/health-monitor.sh https://raw.githubusercontent.com/kubermatic/machine-controller/8b5b66e4910a6228dfaecccaa0a3b05ec4902f8e/pkg/userdata/scripts/health-monitor.sh
    chmod +x /opt/bin/health-monitor.sh
fi

# crictl
if [ ! -f /opt/bin/crictl ]; then
    {{- if semverCompare ">=1.10.0-0, < 1.11.0-0" .KubeletVersion }}
    wget -O /opt/crictl.tar.gz https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.0.0-beta.1/crictl-v1.0.0-beta.1-linux-amd64.tar.gz
    {{- else if semverCompare ">=1.11.0-0, < 1.12.0-0" .KubeletVersion }}
    wget -O /opt/crictl.tar.gz https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.11.1/crictl-v1.11.1-linux-amd64.tar.gz
    {{- else }}
    wget -O /opt/crictl.tar.gz https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.12.0/crictl-v1.12.0-linux-amd64.tar.gz
    {{- end }}
    tar -xvf /opt/crictl.tar.gz -C /opt/bin/
fi

# runc
if [ ! -f /opt/bin/runc ]; then
    wget -O /opt/bin/runc https://github.com/opencontainers/runc/releases/download/v1.0.0-rc5/runc.amd64        
    chmod +x /opt/bin/runc
fi

# containerd
if [ ! -f /opt/bin/containerd ]; then
    wget -O /opt/containerd.tar.gz https://github.com/containerd/containerd/releases/download/v1.2.0-rc.1/containerd-1.2.0-rc.1.linux-amd64.tar.gz
    sudo tar -xvf /opt/containerd.tar.gz -C /opt/
fi
`
)

// DownloadBinariesScript returns the script which is responsible to download all required binaries.
// Extracted into a dedicated function so we can use it to prepare custom images: TODO: Use it to prepare custom images...
func DownloadBinariesScript(kubeletVersion string, downloadKubelet bool) (string, error) {
	tmpl, err := template.New("download-binaries").Funcs(TxtFuncMap()).Parse(downloadBinariesTpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse download-binaries template: %v", err)
	}

	data := struct {
		KubeletVersion  string
		DownloadKubelet bool
	}{
		KubeletVersion:  kubeletVersion,
		DownloadKubelet: downloadKubelet,
	}
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute download-binaries template: %v", err)
	}

	return string(b.String()), nil
}
