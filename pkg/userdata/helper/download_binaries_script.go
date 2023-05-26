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

package helper

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

const (
	safeDownloadBinariesTpl = `{{- /*setup some common directories */ -}}
opt_bin=/opt/bin
usr_local_bin=/usr/local/bin
cni_bin_dir=/opt/cni/bin

{{- /* create all the necessary dirs */}}
mkdir -p /etc/cni/net.d /etc/kubernetes/manifests "$opt_bin" "$cni_bin_dir"

{{- /* HOST_ARCH can be defined outside of machine-controller (in kubeone for example) */}}
arch=${HOST_ARCH-}
if [ -z "$arch" ]
then
case $(uname -m) in
x86_64)
    arch="amd64"
    ;;
aarch64)
    arch="arm64"
    ;;
*)
    echo "unsupported CPU architecture, exiting"
    exit 1
    ;;
esac
fi

{{- /* # CNI variables */}}
CNI_VERSION="${CNI_VERSION:-{{ .CNIVersion }}}"
cni_base_url="https://github.com/containernetworking/plugins/releases/download/$CNI_VERSION"
cni_filename="cni-plugins-linux-$arch-$CNI_VERSION.tgz"

{{- /* download CNI */}}
curl -Lfo "$cni_bin_dir/$cni_filename" "$cni_base_url/$cni_filename"

{{- /* download CNI checksum */}}
cni_sum=$(curl -Lf "$cni_base_url/$cni_filename.sha256")
cd "$cni_bin_dir"

{{- /* verify CNI checksum */}}
sha256sum -c <<<"$cni_sum"

{{- /* unpack CNI */}}
tar xvf "$cni_filename"
rm -f "$cni_filename"
cd -

{{- /* # cri-tools variables */}}
CRI_TOOLS_RELEASE="${CRI_TOOLS_RELEASE:-{{ .CRIToolsVersion }}}"
cri_tools_base_url="https://github.com/kubernetes-sigs/cri-tools/releases/download/${CRI_TOOLS_RELEASE}"
cri_tools_filename="crictl-${CRI_TOOLS_RELEASE}-linux-${arch}.tar.gz"

{{- /* download cri-tools */}}
curl -Lfo "$opt_bin/$cri_tools_filename" "$cri_tools_base_url/$cri_tools_filename"

{{- /* download cri-tools checksum */}}
{{- /* the cri-tools checksum file provides only the checksum without the file name, so we need to handle it specially */}}
cri_tools_sum_value=$(curl -Lf "$cri_tools_base_url/$cri_tools_filename.sha256")
cri_tools_sum="$cri_tools_sum_value $cri_tools_filename"
cd "$opt_bin"

{{- /* verify cri-tools checksum */}}
sha256sum -c <<<"$cri_tools_sum"

{{- /* unpack cri-tools and symlink to path so it's available to all users */}}
tar xvf "$cri_tools_filename"
rm -f "$cri_tools_filename"
ln -sf "$opt_bin/crictl" "$usr_local_bin"/crictl || echo "symbolic link is skipped"
cd -

{{- /* kubelet */}}
KUBE_VERSION="${KUBE_VERSION:-{{ .KubeVersion }}}"
kube_dir="$opt_bin/kubernetes-$KUBE_VERSION"
kube_base_url="https://storage.googleapis.com/kubernetes-release/release/$KUBE_VERSION/bin/linux/$arch"
kube_sum_file="$kube_dir/sha256"

{{- /* create versioned kube dir */}}
mkdir -p "$kube_dir"
: >"$kube_sum_file"

for bin in kubelet kubeadm kubectl; do
    {{- /* download kube binary */}}
    curl -Lfo "$kube_dir/$bin" "$kube_base_url/$bin"
    chmod +x "$kube_dir/$bin"

    {{- /* download kube binary checksum */}}
    sum=$(curl -Lf "$kube_base_url/$bin.sha256")

    {{- /* save kube binary checksum */}}
    echo "$sum  $kube_dir/$bin" >>"$kube_sum_file"
done

{{- /* check kube binaries checksum */}}
sha256sum -c "$kube_sum_file"

for bin in kubelet kubeadm kubectl; do
    {{- /* link kube binaries from verioned dir to $opt_bin */}}
    ln -sf "$kube_dir/$bin" "$opt_bin"/$bin
done

if [[ ! -x /opt/bin/health-monitor.sh ]]; then
    curl -Lfo /opt/bin/health-monitor.sh https://raw.githubusercontent.com/kubermatic/machine-controller/7967a0af2b75f29ad2ab227eeaa26ea7b0f2fbde/pkg/userdata/scripts/health-monitor.sh
    chmod +x /opt/bin/health-monitor.sh
fi
`

	downloadBinariesTpl = `{{- /*setup some common directories */ -}}
mkdir -p /opt/bin/
mkdir -p /var/lib/calico
mkdir -p /etc/kubernetes/manifests
mkdir -p /etc/cni/net.d
mkdir -p /opt/cni/bin

{{- /* # cni */}}
if [ ! -f /opt/cni/bin/loopback ]; then
    curl -L https://github.com/containernetworking/plugins/releases/download/v0.8.7/cni-plugins-linux-amd64-v0.8.7.tgz | tar -xvzC /opt/cni/bin -f -
fi

{{- if .DownloadKubelet }}
{{- /* kubelet */}}
if [ ! -f /opt/bin/kubelet ]; then
    curl -Lfo /opt/bin/kubelet https://storage.googleapis.com/kubernetes-release/release/v{{ .KubeletVersion }}/bin/linux/amd64/kubelet
    chmod +x /opt/bin/kubelet
fi
{{- end }}

if [[ ! -x /opt/bin/health-monitor.sh ]]; then
    curl -Lfo /opt/bin/health-monitor.sh https://raw.githubusercontent.com/kubermatic/machine-controller/8b5b66e4910a6228dfaecccaa0a3b05ec4902f8e/pkg/userdata/scripts/health-monitor.sh
    chmod +x /opt/bin/health-monitor.sh
fi
`
)

// SafeDownloadBinariesScript returns the script which is responsible to
// download and check checksums of all required binaries.
func SafeDownloadBinariesScript(kubeVersion string) (string, error) {
	tmpl, err := template.New("download-binaries").Funcs(TxtFuncMap()).Parse(safeDownloadBinariesTpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse download-binaries template: %w", err)
	}

	const (
		CNIVersion      = "v1.2.0"
		CRIToolsVersion = "v1.26.0"
	)

	// force v in case if it's not there
	if !strings.HasPrefix(kubeVersion, "v") {
		kubeVersion = "v" + kubeVersion
	}

	data := struct {
		KubeVersion     string
		CNIVersion      string
		CRIToolsVersion string
	}{
		KubeVersion:     kubeVersion,
		CNIVersion:      CNIVersion,
		CRIToolsVersion: CRIToolsVersion,
	}

	b := &bytes.Buffer{}
	err = tmpl.Execute(b, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute download-binaries template: %w", err)
	}

	return b.String(), nil
}

// DownloadBinariesScript returns the script which is responsible to download
// all required binaries.
func DownloadBinariesScript(kubeletVersion string, downloadKubelet bool) (string, error) {
	tmpl, err := template.New("download-binaries").Funcs(TxtFuncMap()).Parse(downloadBinariesTpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse download-binaries template: %w", err)
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
		return "", fmt.Errorf("failed to execute download-binaries template: %w", err)
	}

	return b.String(), nil
}
