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

//
// UserData plugin for Windows.
//

package windows

import (
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/semver"

	"github.com/kubermatic/machine-controller/pkg/apis/plugin"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	userdatahelper "github.com/kubermatic/machine-controller/pkg/userdata/helper"
)

// Provider is a pkg/userdata/plugin.Provider implementation.
type Provider struct{}

// UserData renders user-data template to string.
func (p Provider) UserData(req plugin.UserDataRequest) (string, error) {
	tmpl, err := template.New("user-data").Funcs(userdatahelper.TxtFuncMap()).Parse(userDataTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse user-data template: %w", err)
	}

	kubeletVersion, err := semver.NewVersion(req.MachineSpec.Versions.Kubelet)
	if err != nil {
		return "", fmt.Errorf("invalid kubelet version: %w", err)
	}

	pconfig, err := providerconfigtypes.GetConfig(req.MachineSpec.ProviderSpec)
	if err != nil {
		return "", fmt.Errorf("failed to get provider config: %w", err)
	}

	if pconfig.OverwriteCloudConfig != nil {
		req.CloudConfig = *pconfig.OverwriteCloudConfig
	}

	if pconfig.Network != nil {
		return "", errors.New("static IP config is not supported with Windows")
	}

	windowsConfig, err := LoadConfig(pconfig.OperatingSystemSpec)
	if err != nil {
		return "", fmt.Errorf("failed to parse OperatingSystemSpec: %w", err)
	}

	serverAddr, err := userdatahelper.GetServerAddressFromKubeconfig(req.Kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting server address from kubeconfig: %w", err)
	}

	kubeconfigString, err := userdatahelper.StringifyKubeconfig(req.Kubeconfig)
	if err != nil {
		return "", err
	}

	kubernetesCACert, err := userdatahelper.GetCACert(req.Kubeconfig)
	if err != nil {
		return "", fmt.Errorf("error extracting cacert: %w", err)
	}

	crEngine := req.ContainerRuntime.Engine(kubeletVersion)
	crScript, err := crEngine.ScriptFor(providerconfigtypes.OperatingSystemWindows)
	if err != nil {
		return "", fmt.Errorf("failed to generate container runtime install script: %w", err)
	}

	crConfig, err := crEngine.Config()
	if err != nil {
		return "", fmt.Errorf("failed to generate container runtime config: %w", err)
	}

	data := struct {
		plugin.UserDataRequest
		ProviderSpec                   *providerconfigtypes.Config
		OSConfig                       *Config
		KubeletVersion                 string
		ServerAddr                     string
		Kubeconfig                     string
		KubernetesCACert               string
		NodeIPScript                   string
    ExtraKubeletFlags              []string
    ContainerRuntimeName           string
		ContainerRuntimeScript         string
		ContainerRuntimeConfigFileName string
		ContainerRuntimeConfig         string
	}{
		UserDataRequest:                req,
		ProviderSpec:                   pconfig,
    OSConfig:                       windowsConfig,
		KubeletVersion:                 kubeletVersion.String(),
		ServerAddr:                     serverAddr,
		Kubeconfig:                     kubeconfigString,
		KubernetesCACert:               kubernetesCACert,
		NodeIPScript:                   userdatahelper.SetupWinNodeIPEnvScript(),
    ExtraKubeletFlags:              crEngine.KubeletFlags(providerconfigtypes.OperatingSystemWindows),
    ContainerRuntimeName:           req.ContainerRuntime.String(),
		ContainerRuntimeScript:         crScript,
		ContainerRuntimeConfigFileName: crEngine.ConfigFileName(providerconfigtypes.OperatingSystemWindows),
		ContainerRuntimeConfig:         crConfig,
	}

	buf := strings.Builder{}
	if err = tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute user-data template: %w", err)
	}

	return userdatahelper.CleanupTemplateOutput(buf.String())
}

// UserData template.
const userDataTemplate = `#cloud-config
set_hostname: {{ .MachineSpec.Name }}

users:
  - name: Administrator
    primary_group: Users
    groups: Administrators
{{- if ne (len .ProviderSpec.SSHPublicKeys) 0 }}
    inactive: False
    ssh_authorized_keys:
{{- range .ProviderSpec.SSHPublicKeys }}
      - "{{ . }}"
{{- end }}
{{- else }}
    inactive: True
{{- end }}

runcmd:
  - 'call powershell -File C:\setup-node.ps1 -NoLogo -ExecutionPolicy ByPass -OutputFormat Text -InputFormat Text -NonInteractive'

write_files:
  - path: C:\setup-node.ps1
    permissions: '0644'
    content: |
{{ .ContainerRuntimeScript | indent 6 }}
      Invoke-WebRequest -Uri "https://github.com/kubernetes-sigs/sig-windows-tools/releases/latest/download/PrepareNode.ps1" -UseBasicParsing -OutFile "C:/k/PrepareNode.ps1"
      C:/k/PrepareNode.ps1 -KubernetesVersion "v{{ .KubeletVersion }}" -ContainerRuntime "{{ .ContainerRuntimeName }}"
      # set kubelet nodeip environment variable
      C:/k/setup_net_env.ps1
      C:/k/Install-PowerShellCore.ps1
      C:/k/Install-OpenSSH.ps1
      # Enable ICMP echo requests this instance over IPv4 and IPv6
      (Get-NetFirewallRule -Group "@FirewallAPI.dll,-28502").Where{$_.Name -like "*ICMP*"} | Enable-NetFirewallRule
      # Contact Microsoft activation servers and activate windows if possible (will silently fail if no activation is possible)
      $null = Start-Process "C:\Windows\System32\cscript.exe" -ArgumentList @("C:\Windows\System32\slmgr.vbs", "/ato")
      # Disable IPv6 Privacy Extension (causes conflicts with Portsecurity implementations)
      Set-NetIPv6Protocol -RandomizeIdentifiers Disabled -UseTemporaryAddresses Disabled
  - path: "C:/k/Install-PowerShellCore.ps1
    permissions: '0644'
    content: |
      $pwshInstallPath = 'C:/Program Files/PowerShell/7'
      $pwshTempFile = Join-Path -Path $env:TEMP -ChildPath 'PWSH.msi'
      $pwshTempFileQ = '"' + $pwshTempFile + '"'
      $newVersionJson = ConvertFrom-Json -InputObject (Invoke-WebRequest -Uri 'https://api.github.com/repos/PowerShell/PowerShell/releases/latest' -UseBasicParsing).Content
      $newVersionURL = $newVersionJson.assets.where{$_.name -like 'PowerShell-*-win-x64.msi'}.browser_download_url
      Invoke-WebRequest -Uri $newVersionURL -OutFile $pwshTempFile -UseBasicParsing
      Get-Package -ProviderName msi -Name "PowerShell *" | ForEach-Object { $_ | Uninstall-Package -Force -PackageManagementProvider msi | Out-Null }
      $installArguments = @('/i', $pwshTempFileQ, '/qn', '/norestart')
      Start-Process -FilePath 'msiexec.exe' -ArgumentList $installArguments -Wait -NoNewWindow
      Remove-Item -Path $pwshTempFile -Force
      Push-Location -Path $pwshInstallPath
      $pwshSetupRemoting = @('-NoLogo', '-NonInteractive', '-ExecutionPolicy', 'RemoteSigned', '-NoProfile', '-File', "$pwshInstallPath\Install-PowerShellRemoting.ps1")
      Start-Process -FilePath '.\pwsh.exe' -ArgumentList $pwshArguments -Wait -NoNewWindow
      Pop-Location
      $path = [Environment]::GetEnvironmentVariable('Path', [System.EnvironmentVariableTarget]::Machine) -split ';'
      if ($path -notcontains $pwshInstallPath) {
        $path += $pwshInstallPath
      }
      [Environment]::SetEnvironmentVariable('Path', $path -join ';', [System.EnvironmentVariableTarget]::Machine)
  - path: "C:/k/Install-OpenSSH.ps1
    permissions: '0644'
    content: |
      $OpenSSHTempFile = Join-Path -Path $env:TEMP -ChildPath 'openssh.zip'
      $OpenSSHInstallPath = 'C:/Program Files/OpenSSH-Win64'
      $latestVersionURL = (ConvertFrom-Json -InputObject (Invoke-WebRequest -Uri 'https://api.github.com/repos/PowerShell/Win32-OpenSSH/releases/latest' -UseBasicParsing).Content).assets.where{$_.name -eq 'OpenSSH-Win64.zip'}.browser_download_url
      Invoke-WebRequest -Uri $latestVersionURL -OutFile $OpenSSHTempFile -UseBasicParsing
      if (Test-Path -Path $OpenSSHInstallPath) {
        Push-Location -Path $OpenSSHInstallPath
        try { ./uninstall-sshd.ps1 } catch {}
        Pop-Location
        Item -Path $OpenSSHInstallPath -Recurse -Force
      }
      Expand-Archive -Path $OpenSSHTempFile -DestinationPath $OpenSSHInstallPath -Force
      Remove-Item -Path $OpenSSHTempFile -Force
      Push-Location -Path $OpenSSHInstallPath
      ./install-sshd.ps1
      Pop-Location
      New-Item -Path $env:ProgramData -Name 'ssh' -ItemType Directory -Force
      $path = [Environment]::GetEnvironmentVariable('Path', [System.EnvironmentVariableTarget]::Machine) -split ';'
      if ($path -notcontains $OpenSSHInstallPath) {
        $path += $OpenSSHInstallPath
      }
      [Environment]::SetEnvironmentVariable('Path', $path -join ';', [System.EnvironmentVariableTarget]::Machine)
      try { Remove-NetFirewallRule -Name 'sshd' } catch {}
      New-NetFirewallRule -Name 'sshd' -DisplayName 'OpenSSH Server (sshd)' -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22
      Set-Service -Name 'sshd' -StartupType Automatic
      Set-Service -Name 'ssh-agent' -StartupType Automatic
      Start-Service -Name 'sshd'
      Start-Service -Name 'ssh-agent'
  - path: "C:/ProgramData/ssh/sshd_config"
    permissions: "0644"
    content: |
      $configFile = @'
      Port 22
      AuthorizedKeysFile  .ssh/authorized_keys
      PasswordAuthentication yes
      PubkeyAuthentication yes
      Subsystem sftp  sftp-server.exe
      Subsystem powershell  pwsh.exe -sshs -NoLogo -NoProfile
      '@

  - path: "C:/k/cloud-config"
    permissions: "0600"
    content: |
{{ .CloudConfig | indent 6 }}
  - path: "C:/k/setup_net_env.ps1"
    permissions: "0755"
    content: |
{{ .NodeIPScript | indent 6 }}
  - path: "C:/k/bootstrap-kubelet.conf"
    permissions: "0600"
    content: |
{{ .Kubeconfig | indent 6 }}
  - path: "C:/k/kubelet.conf"
    content: |
{{ kubeletConfiguration "cluster.local" .DNSIPs .KubeletFeatureGates | indent 6 }}
  - path: "C:/k/pki/ca.crt"
    content: |
{{ .KubernetesCACert | indent 6 }}
  - path: {{ .ContainerRuntimeConfigFileName }}
    permissions: "0644"
    content: |
{{ /* TODO: .ContainerRuntimeConfig | indent 6 */ }}
      version = 2
      root = "C:\\ProgramData\\containerd\\root"
      state = "C:\\ProgramData\\containerd\\state"
      plugin_dir = ""
      disabled_plugins = []
      required_plugins = []
      oom_score = 0
      
      [grpc]
        address = "\\\\.\\pipe\\containerd-containerd"
        tcp_address = ""
        tcp_tls_cert = ""
        tcp_tls_key = ""
        uid = 0
        gid = 0
        max_recv_message_size = 16777216
        max_send_message_size = 16777216
      
      [ttrpc]
        address = ""
        uid = 0
        gid = 0
      
      [debug]
        address = ""
        uid = 0
        gid = 0
        level = ""
      
      [metrics]
        address = ""
        grpc_histogram = false
      
      [cgroup]
        path = ""
      
      [timeouts]
        "io.containerd.timeout.shim.cleanup" = "5s"
        "io.containerd.timeout.shim.load" = "5s"
        "io.containerd.timeout.shim.shutdown" = "3s"
        "io.containerd.timeout.task.state" = "2s"
      
      [plugins]
        [plugins."io.containerd.gc.v1.scheduler"]
          pause_threshold = 0.02
          deletion_threshold = 0
          mutation_threshold = 100
          schedule_delay = "0s"
          startup_delay = "100ms"
        [plugins."io.containerd.grpc.v1.cri"]
          disable_tcp_service = true
          stream_server_address = "127.0.0.1"
          stream_server_port = "0"
          stream_idle_timeout = "4h0m0s"
          enable_selinux = false
          selinux_category_range = 0
          sandbox_image = "mcr.microsoft.com/oss/kubernetes/pause:1.4.0"     
          stats_collect_period = 10
          systemd_cgroup = false
          enable_tls_streaming = false
          max_container_log_line_size = 16384
          disable_cgroup = false
          disable_apparmor = false
          restrict_oom_score_adj = false
          max_concurrent_downloads = 3
          disable_proc_mount = false
          unset_seccomp_profile = ""
          tolerate_missing_hugetlb_controller = false
          disable_hugetlb_controller = false
          ignore_image_defined_volumes = false
          [plugins."io.containerd.grpc.v1.cri".containerd]
            snapshotter = "windows"
            default_runtime_name = "runhcs-wcow-process"
            no_pivot = false
            disable_snapshot_annotations = false
            discard_unpacked_layers = false
            [plugins."io.containerd.grpc.v1.cri".containerd.default_runtime] 
              runtime_type = ""
              runtime_engine = ""
              runtime_root = ""
              privileged_without_host_devices = false
              base_runtime_spec = ""
            [plugins."io.containerd.grpc.v1.cri".containerd.untrusted_workload_runtime]
              runtime_type = ""
              runtime_engine = ""
              runtime_root = ""
              privileged_without_host_devices = false
              base_runtime_spec = ""
            [plugins."io.containerd.grpc.v1.cri".containerd.runtimes]
              [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runhcs-wcow-process]
                runtime_type = "io.containerd.runhcs.v1"
                runtime_engine = ""
                runtime_root = ""
                privileged_without_host_devices = false
                base_runtime_spec = ""
          [plugins."io.containerd.grpc.v1.cri".cni]
            bin_dir = "C:\\Program Files\\containerd\\cni\\bin"
            conf_dir = "C:\\Program Files\\containerd\\cni\\conf"
            max_conf_num = 1
            conf_template = ""
          [plugins."io.containerd.grpc.v1.cri".registry]
            [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
              [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
                endpoint = ["https://registry-1.docker.io"]
          [plugins."io.containerd.grpc.v1.cri".image_decryption]
            key_model = ""
          [plugins."io.containerd.grpc.v1.cri".x509_key_pair_streaming]
            tls_cert_file = ""
            tls_key_file = ""
        [plugins."io.containerd.internal.v1.opt"]
          path = "C:\\ProgramData\\containerd\\root\\opt"
        [plugins."io.containerd.internal.v1.restart"]
          interval = "10s"
        [plugins."io.containerd.metadata.v1.bolt"]
          content_sharing_policy = "shared"
        [plugins."io.containerd.runtime.v2.task"]
          platforms = ["windows/amd64", "linux/amd64"]
        [plugins."io.containerd.service.v1.diff-service"]
          default = ["windows", "windows-lcow"]

`
