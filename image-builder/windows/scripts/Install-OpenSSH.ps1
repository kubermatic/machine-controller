$ProgressPreference="SilentlyContinue"
$DebugPreference="SilentlyContinue"
$WarningPreference="Continue"
$ErrorActionPreference="Stop"
Set-StrictMode -Version 2

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$OpenSSHTempFile = Join-Path -Path $env:TEMP -ChildPath 'openssh.zip'
$OpenSSHInstallPath = 'C:/Program Files/OpenSSH-Win64'
$latestVersionURL = (ConvertFrom-Json -InputObject (Invoke-WebRequest -Uri 'https://api.github.com/repos/PowerShell/Win32-OpenSSH/releases/latest' -UseBasicParsing).Content).assets.where{$_.name -eq 'OpenSSH-Win64.zip'}.browser_download_url
Invoke-WebRequest -Uri $latestVersionURL -OutFile $OpenSSHTempFile -UseBasicParsing
if (Test-Path -Path $OpenSSHInstallPath) {
  if ([bool](Get-Service -Name 'sshd' -ErrorAction SilentlyContinue)) {
    Stop-Service -Name 'sshd' -Force
    Get-CimInstance -ClassName Win32_Service -Filter "Name='sshd'" | Remove-CimInstance -Confirm:$false
  }
  Get-Process -Name 'sshd' -ErrorAction SilentlyContinue | Stop-Process -Force
  Push-Location -Path $OpenSSHInstallPath
  try { ./uninstall-sshd.ps1 } catch {}
  Pop-Location
  Remove-Item -Path $OpenSSHInstallPath -Recurse -Force
}
Expand-Archive -Path $OpenSSHTempFile -DestinationPath $OpenSSHInstallPath -Force
Move-Item -Path "$OpenSSHInstallPath/OpenSSH-Win64/*" -Destination $OpenSSHInstallPath -Force
Remove-Item -Path "$OpenSSHInstallPath/OpenSSH-Win64" -Force
Remove-Item -Path $OpenSSHTempFile -Force
Push-Location -Path $OpenSSHInstallPath
./install-sshd.ps1
Pop-Location
New-Item -Path $env:ProgramData -Name 'ssh' -ItemType Directory -Force
$path = [Environment]::GetEnvironmentVariable('Path', [System.EnvironmentVariableTarget]::Machine) -split ';'
if ($path -notcontains $OpenSSHInstallPath) { $path += $OpenSSHInstallPath }
[Environment]::SetEnvironmentVariable('Path', $path -join ';', [System.EnvironmentVariableTarget]::Machine)
try { Remove-NetFirewallRule -Name 'sshd' } catch {}
New-NetFirewallRule -Name 'sshd' -DisplayName 'OpenSSH Server (sshd)' -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22
@'
Port 22
AuthorizedKeysFile  .ssh/authorized_keys
PasswordAuthentication yes
PubkeyAuthentication yes
Subsystem sftp  sftp-server.exe
Subsystem powershell  pwsh.exe -sshs -NoLogo -NoProfile
'@ | Out-File -Path "C:/ProgramData/ssh/sshd_config" -Force -Encoding ascii
Set-Service -Name 'sshd' -StartupType Automatic
Set-Service -Name 'ssh-agent' -StartupType Automatic
Start-Service -Name 'sshd'
Start-Service -Name 'ssh-agent'
