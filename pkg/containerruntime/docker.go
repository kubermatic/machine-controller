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
	DefaultDockerVersion = "19.03"
	LegacyDockerVersion  = "18.09"
)

type Docker struct {
	insecureRegistries   []string
	registryMirrors      []string
	containerLogMaxFiles string
	containerLogMaxSize  string
}

func (eng *Docker) Config() (string, error) {
	return helper.DockerConfig(eng.insecureRegistries, eng.registryMirrors, eng.containerLogMaxFiles, eng.containerLogMaxSize)
}

func (eng *Docker) ConfigFileName() string {
	return "/etc/docker/daemon.json"
}

func (eng *Docker) KubeletFlags() []string {
	return []string{
		"--container-runtime=docker",
		"--container-runtime-endpoint=unix:///var/run/dockershim.sock",
	}
}

func (eng *Docker) ScriptFor(os types.OperatingSystem) (string, error) {
	var buf strings.Builder

	args := struct {
		DockerVersion     string
		ContainerdVersion string
	}{
		DockerVersion:     DefaultDockerVersion,
		ContainerdVersion: DefaultContainerdVersion,
	}

	switch os {
	case types.OperatingSystemAmazonLinux2:
		err := dockerAmazonTemplate.Execute(&buf, args)
		return buf.String(), err
	case types.OperatingSystemCentOS, types.OperatingSystemRHEL:
		err := dockerYumTemplate.Execute(&buf, args)
		return buf.String(), err
	case types.OperatingSystemUbuntu:
		err := dockerAptTemplate.Execute(&buf, args)
		return buf.String(), err
	case types.OperatingSystemFlatcar:
		err := dockerFlatcarTemplate.Execute(&buf, args)
		return buf.String(), err
	case types.OperatingSystemSLES:
		return "", nil
	}

	return "", fmt.Errorf("unknown OS: %s", os)
}

var (
	dockerFlatcarTemplate = template.Must(template.New("docker-flatcar").Parse(`
systemctl daemon-reload
systemctl enable --now docker
`))

	dockerAmazonTemplate = template.Must(template.New("docker-yum-amzn2").Parse(`
mkdir -p /etc/systemd/system/containerd.service.d /etc/systemd/system/docker.service.d

cat <<EOF | tee /etc/systemd/system/containerd.service.d/environment.conf /etc/systemd/system/docker.service.d/environment.conf
[Service]
Restart=always
EnvironmentFile=-/etc/environment
EOF

yum install -y \
{{- if .ContainerdVersion }}
    containerd-{{ .ContainerdVersion }}* \
{{- end }}
    docker-{{ .DockerVersion }}* \
    yum-plugin-versionlock
yum versionlock add docker containerd

systemctl daemon-reload
systemctl enable --now docker
`))

	dockerYumTemplate = template.Must(template.New("docker-yum").Parse(`
yum install -y yum-utils
yum-config-manager --add-repo=https://download.docker.com/linux/centos/docker-ce.repo
yum-config-manager --save --setopt=docker-ce-stable.module_hotfixes=true

mkdir -p /etc/systemd/system/containerd.service.d /etc/systemd/system/docker.service.d

cat <<EOF | tee /etc/systemd/system/containerd.service.d/environment.conf /etc/systemd/system/docker.service.d/environment.conf
[Service]
Restart=always
EnvironmentFile=-/etc/environment
EOF

yum install -y \
{{- if .ContainerdVersion }}
    docker-ce-cli-{{ .DockerVersion }}* \
    containerd.io-{{ .ContainerdVersion }}* \
{{- end }}
    docker-ce-{{ .DockerVersion }}* \
    yum-plugin-versionlock
yum versionlock add docker-ce* containerd.io

systemctl daemon-reload
systemctl enable --now docker
`))

	dockerAptTemplate = template.Must(template.New("docker-apt").Parse(`
apt-get update
apt-get install -y apt-transport-https ca-certificates curl software-properties-common lsb-release
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
add-apt-repository "deb https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"

mkdir -p /etc/systemd/system/containerd.service.d /etc/systemd/system/docker.service.d

cat <<EOF | tee /etc/systemd/system/containerd.service.d/environment.conf /etc/systemd/system/docker.service.d/environment.conf
[Service]
Restart=always
EnvironmentFile=-/etc/environment
EOF

apt-get install --allow-downgrades -y \
{{- if .ContainerdVersion }}
    containerd.io={{ .ContainerdVersion }}* \
    docker-ce-cli=5:{{ .DockerVersion }}* \
{{- end }}
    docker-ce=5:{{ .DockerVersion }}*
apt-mark hold docker-ce* containerd.io

systemctl daemon-reload
systemctl enable --now docker
`))
)

func (eng *Docker) String() string {
	return dockerName
}
