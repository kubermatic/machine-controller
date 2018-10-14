package helper

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/Masterminds/sprig"
)

const (
	containerdConfigTpl = `[plugins]
  {{- if semverCompare ">=1.10.0-0, < 1.11.0-0" .KubeletVersion }}
  [plugins.cri]
    # see release notes: https://github.com/containerd/containerd/releases/tag/v1.2.0-rc.1
    stream_server_address = ""
  {{- end }}
`
	containerdSystemdUnitTpl = `[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target

[Service]
Environment="PATH=/opt/bin:/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin/"
ExecStartPre=/sbin/modprobe overlay
ExecStart=/opt/bin/containerd --config /etc/containerd/config.toml
Restart=always
RestartSec=5
Delegate=yes
KillMode=process
OOMScoreAdjust=-999
LimitNOFILE=1048576
# Having non-zero Limit*s causes performance problems due to accounting overhead
# in the kernel. We recommend using cgroups to do container-local accounting.
LimitNPROC=infinity
LimitCORE=infinity

[Install]
WantedBy=multi-user.target
`
)

// ContainerdConfig returns the config file for containerd
func ContainerdConfig(kubeletVersion string) (string, error) {
	tmpl, err := template.New("containerd-config").Funcs(sprig.TxtFuncMap()).Parse(containerdConfigTpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse containerd-config template: %v", err)
	}

	data := struct {
		KubeletVersion string
	}{
		KubeletVersion: kubeletVersion,
	}
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute containerd-config template: %v", err)
	}

	return string(b.String()), nil
}

// ContainerdSystemdUnit returns the systemd unit for containerd
func ContainerdSystemdUnit(kubeletVersion string) (string, error) {
	tmpl, err := template.New("containerd-systemd-unit").Funcs(sprig.TxtFuncMap()).Parse(containerdSystemdUnitTpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse containerd-systemd-unit template: %v", err)
	}

	data := struct {
		KubeletVersion string
	}{
		KubeletVersion: kubeletVersion,
	}
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute containerd-systemd-unit template: %v", err)
	}

	return string(b.String()), nil
}
