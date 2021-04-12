$ProgressPreference="SilentlyContinue"
$DebugPreference="SilentlyContinue"
$WarningPreference="Continue"
$ErrorActionPreference="Stop"
Set-StrictMode -Version 2

# Windows Defender
Set-MpPreference `
  -CheckForSignaturesBeforeRunningScan $true `
  -DisableBehaviorMonitoring $false
  -DisableCatchupFullScan $false `
  -DisableCatchupQuickScan $false `
  -DisableEmailScanning $false `
  -DisableIntrusionPreventionSystem $false `
  -DisableIOAVProtection $false `
  -DisableRealtimeMonitoring $false `
  -DisableRemovableDriveScanning $false `
  -SubmitSamplesConsent 0 `
  -UILockdown $true

# SmartScreen
New-ItemProperty -Path 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\System' -Name EnableSmartScreen -Value 1 -PropertyType DWord -Force
New-ItemProperty -Path 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\System' -Name ShellSmartScreenLevel -Value 'Warn' -PropertyType String -Force

# Disable IPv6 Privacy Extension and Interface Identifier Randomization
# (Workaround for OpenStack connectivity issues)
# https://www.sami-lehtinen.net/blog/how-to-persistently-disable-ipv6-privacy-addressing-on-windows-2012-r2-server
Set-NetIPv6Protocol -RandomizeIdentifiers Disabled -UseTemporaryAddresses Disabled

# Set RDP security level to require NLA
Set-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\Terminal Server\WinStations\RDP-Tcp\' -Name "UserAuthentication" -Value 1

# Enable RDP
Set-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\Terminal Server\' -Name "fDenyTSConnections" -Value 0
Enable-NetFirewallRule -Group '@FirewallAPI.dll,-28752'

# Enable ICMP Echo Requests
Get-NetFirewallRule -Group "@FirewallAPI.dll,-28502" | Where-Object Name -like "*ICMP*" | Enable-NetFirewallRule

# Disable Auto Logon
Set-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" -Name "AutoAdminLogon" -Value "0" -Type String
Set-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" -Name "DefaultUsername" -Value "" -Type String
Set-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" -Name "DefaultPassword" -Value "" -Type String

# Enable CPU Vulnerability Mitigations e.g. Spectre, Meltdown, ...
Set-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management' -Name 'FeatureSettingsOverride' -Value 8264
Set-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management' -Name 'FeatureSettingsOverrideMask' -Value 3

# Configure TLS to only use strong protocols and ciphers
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
Invoke-WebRequest -Uri "https://www.nartac.com/Downloads/IISCrypto/IISCryptoCli.exe" -OutFile "IISCryptoCli.exe" -UseBasicParsing
Unblock-File "IISCryptoCli.exe"
.\IISCryptoCli.exe /template strict
Remove-File "IISCryptoCli.exe"

# Enable SCHANNEL EventLogging
Set-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Control\SecurityProviders\SCHANNEL" -name 'EventLogging' -value '0x00000001' -Type 'DWord' -Force
