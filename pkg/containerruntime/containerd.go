/*
Copyright 2020 The Machine Controller Authors.

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

package containerruntime

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/kubermatic/machine-controller/pkg/userdata/helper"
)

const (
	DefaultContainerdVersion = "1.4.3-1"
)

type Containerd struct {
	insecureRegistries []string
	registryMirrors    []string
}

func (eng *Containerd) Config() (string, error) {
	return helper.ContainerdConfig(eng.insecureRegistries, eng.registryMirrors)
}

func (eng *Containerd) ConfigFileName() string {
	return "/etc/containerd/config.toml"
}

func (eng *Containerd) KubeletFlags() []string {
	return []string{
		"--container-runtime=remote",
		"--container-runtime-endpoint=unix:///run/containerd/containerd.sock",
	}
}

func (eng *Containerd) ScriptFor(os types.OperatingSystem) (string, error) {
	var buf strings.Builder

	args := struct {
		ContainerdVersion string
	}{
		ContainerdVersion: DefaultContainerdVersion,
	}

	switch os {
	case types.OperatingSystemCentOS, types.OperatingSystemRHEL:
		err := containerdYumTemplate.Execute(&buf, args)
		return buf.String(), err
	case types.OperatingSystemUbuntu:
		err := containerdAptTemplate.Execute(&buf, args)
		return buf.String(), err
	case types.OperatingSystemFlatcar, types.OperatingSystemCoreos:
		return "", nil
	case types.OperatingSystemSLES:
		return "", nil
	}

	return "", fmt.Errorf("unknown OS: %s", os)
}

var (
	containerdYumTemplate = template.Must(template.New("containerd-yum").Parse(`
yum install -y yum-utils
yum-config-manager --add-repo=https://download.docker.com/linux/centos/docker-ce.repo
{{- /*
    Due to DNF modules we have to do this on docker-ce repo
    More info at: https://bugzilla.redhat.com/show_bug.cgi?id=1756473
*/}}
yum-config-manager --save --setopt=docker-ce-stable.module_hotfixes=true
yum install -y containerd.io-{{ .ContainerdVersion }} yum-plugin-versionlock
yum versionlock add containerd.io

cat <<EOF | tee /etc/crictl.yaml
runtime-endpoint: unix:///run/containerd/containerd.sock
EOF

mkdir -p /etc/systemd/system/containerd.service.d
cat <<EOF | tee /etc/systemd/system/containerd.service.d/environment.conf
[Service]
Restart=always
EnvironmentFile=-/etc/environment
EOF

systemctl daemon-reload
systemctl enable --now containerd
`))

	containerdAptTemplate = template.Must(template.New("containerd-apt").Parse(`
apt-get update
apt-get install -y apt-transport-https ca-certificates curl software-properties-common lsb-release
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
add-apt-repository "deb https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
apt-get install -y containerd.io={{ .ContainerdVersion }}*

cat <<EOF | tee /etc/crictl.yaml
runtime-endpoint: unix:///run/containerd/containerd.sock
EOF

mkdir -p /etc/systemd/system/containerd.service.d
cat <<EOF | tee /etc/systemd/system/containerd.service.d/environment.conf
[Service]
Restart=always
EnvironmentFile=-/etc/environment
EOF

systemctl daemon-reload
systemctl enable --now containerd
`))
)
