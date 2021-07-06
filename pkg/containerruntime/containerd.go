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
	DefaultContainerdVersion        = "1.4"
	DefaultContainerdVersionWindows = "1.5.2"
)

type Containerd struct {
	insecureRegistries []string
	registryMirrors    []string
}

func (eng *Containerd) Config() (string, error) {
	return helper.ContainerdConfig(eng.insecureRegistries, eng.registryMirrors)
}

func (eng *Containerd) ConfigFileName(os types.OperatingSystem) string {
	switch os {
	case types.OperatingSystemWindows:
		return "C:/Program Files/containerd/config.toml"
	}
	return "/etc/containerd/config.toml"
}

func (eng *Containerd) KubeletFlags(os types.OperatingSystem) []string {
	switch os {
	case types.OperatingSystemWindows:
		return []string{
			"--container-runtime=remote",
			"--container-runtime-endpoint=npipe:////./pipe/containerd-containerd",
		}
	}
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
	case types.OperatingSystemAmazonLinux2:
		err := containerdAmzn2Template.Execute(&buf, args)
		return buf.String(), err
	case types.OperatingSystemCentOS, types.OperatingSystemRHEL:
		err := containerdYumTemplate.Execute(&buf, args)
		return buf.String(), err
	case types.OperatingSystemUbuntu:
		err := containerdAptTemplate.Execute(&buf, args)
		return buf.String(), err
	case types.OperatingSystemFlatcar:
		return "", nil
	case types.OperatingSystemSLES:
		return "", nil
	case types.OperatingSystemWindows:
		args = struct {
			ContainerdVersion string
		}{
			ContainerdVersion: DefaultContainerdVersionWindows,
		}
		err := containerdWindowsTemplate.Execute(&buf, args)
		return buf.String(), err
	}

	return "", fmt.Errorf("unknown OS: %s", os)
}

var (
	containerdAmzn2Template = template.Must(template.New("containerd-yum-amzn2").Parse(`
mkdir -p /etc/systemd/system/containerd.service.d

cat <<EOF | tee /etc/systemd/system/containerd.service.d/environment.conf
[Service]
Restart=always
EnvironmentFile=-/etc/environment
EOF

cat <<EOF | tee /etc/crictl.yaml
runtime-endpoint: unix:///run/containerd/containerd.sock
EOF

yum install -y \
	containerd-{{ .ContainerdVersion }}* \
	yum-plugin-versionlock
yum versionlock add containerd

systemctl daemon-reload
systemctl enable --now containerd
`))

	containerdYumTemplate = template.Must(template.New("containerd-yum").Parse(`
yum install -y yum-utils
yum-config-manager --add-repo=https://download.docker.com/linux/centos/docker-ce.repo
{{- /*
    Due to DNF modules we have to do this on docker-ce repo
    More info at: https://bugzilla.redhat.com/show_bug.cgi?id=1756473
*/}}
yum-config-manager --save --setopt=docker-ce-stable.module_hotfixes=true

cat <<EOF | tee /etc/crictl.yaml
runtime-endpoint: unix:///run/containerd/containerd.sock
EOF

mkdir -p /etc/systemd/system/containerd.service.d
cat <<EOF | tee /etc/systemd/system/containerd.service.d/environment.conf
[Service]
Restart=always
EnvironmentFile=-/etc/environment
EOF

yum install -y containerd.io-{{ .ContainerdVersion }}* yum-plugin-versionlock
yum versionlock add containerd.io

systemctl daemon-reload
systemctl enable --now containerd
`))

	containerdAptTemplate = template.Must(template.New("containerd-apt").Parse(`
apt-get update
apt-get install -y apt-transport-https ca-certificates curl software-properties-common lsb-release
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
add-apt-repository "deb https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"

cat <<EOF | tee /etc/crictl.yaml
runtime-endpoint: unix:///run/containerd/containerd.sock
EOF

mkdir -p /etc/systemd/system/containerd.service.d
cat <<EOF | tee /etc/systemd/system/containerd.service.d/environment.conf
[Service]
Restart=always
EnvironmentFile=-/etc/environment
EOF

apt-get install -y containerd.io={{ .ContainerdVersion }}*
apt-mark hold containerd.io

systemctl daemon-reload
systemctl enable --now containerd
`))

	// grpc: address = "\\\\.\\pipe\\containerd-containerd"
	// bin_dir = "C:\\Program Files\\containerd\\cni\\bin"
	// conf_dir = "C:\\Program Files\\containerd\\cni\\conf"
	containerdWindowsTemplate = template.Must(template.New("containerd-windows").Parse(`
Set-Location -Path "$env:tmp"
[string]$arch = $env:PROCESSOR_ARCHITECTURE.ToLower()
$pscurl = Start-Process -FilePath "curl.exe" -ArgumentList @("-OfL", "https://github.com/containerd/containerd/releases/download/v{{ .ContainerdVersion }}/containerd-{{ .ContainerdVersion }}-windows-$arch.tar.gz") -Wait -PassThru
if ([bool]$pscurl.ExitCode) {
	Write-Error 'containerd download failed' -ErrorAction Stop
}
$pstar = Start-Process -FilePath "tar.exe" -ArgumentList @("xvf", ".\containerd-{{ .ContainerdVersion }}-windows-amd64.tar.gz") -Wait -PassThru
if ([bool]$pstar.ExitCode) {
    Write-Error 'containerd download failed' -ErrorAction Stop
}

Stop-Service -Name "containerd" -Force -ErrorAction SilentlyContinue

New-Item -ItemType Directory -Path "$env:ProgramFiles\containerd" -Force
Copy-Item -Path ".\bin\*" -Destination "$env:ProgramFiles\containerd" -Recurse -Force
Set-Location -Path "$env:ProgramFiles\containerd\"
.\containerd.exe config default | Out-File -PSPath "config.toml" -Encoding ascii

# Network cni setup based upon: https://github.com/kubernetes-sigs/sig-windows-tools/releases/download/v0.1.5/Install-Containerd.ps1
$config = Get-Content "config.toml"
$config = $config -replace "bin_dir = (.)*$", 'bin_dir = "c:/opt/cni/bin"'
$config = $config -replace "conf_dir = (.)*$", 'conf_dir = "c:/etc/cni/net.d"'
$config | Out-File -PSPath "config.toml" -Encoding ascii -Force
New-Item -ItemType Directory -Path "C:\opt\cni\bin" -Force
New-Item -ItemType Directory -Path "C:\opt\cni\net.d" -Force
New-Item -ItemType Directory -Path "C:\etc\cni\net.d" -Force
$IPv4DefaultRoute = Get-NetRoute "0.0.0.0/0"
$IPv4Address = Get-NetIPAddress -ifIndex $IPv4DefaultRoute.ifIndex -AddressFamily IPv4
@"
{{ "{{" }}
    "cniVersion": "0.2.0",
    "name": "nat",
    "type": "nat",
    "master": "Ethernet",
    "ipam": {{ "{{" }}
        "subnet": "{0}/{1}",
        "routes": [
            {{ "{{" }}
                "GW": "{2}"
            {{ "}}" }}
        ]
    {{ "}}" }},
    "capabilities": {{ "{{" }}
        "portMappings": true,
        "dns": true
    {{ "}}" }}
{{ "}}" }}
"@ -f $IPv4Address.IPAddress, $IPv4Address.PrefixLength, $IPv4DefaultRoute.NextHop | Set-Content "c:\etc\cni\net.d\0-containerd-nat.json" -Force

$psContainerd = Start-Process -FilePath "$env:ProgramFiles\containerd\containerd.exe" -ArgumentList @("--register-service") -Wait -PassThru
if ([bool]$psContainerd.ExitCode) {
    Write-Error 'containerd download failed' -ErrorAction Stop
}
Start-Service -Name "containerd"
`))
)
