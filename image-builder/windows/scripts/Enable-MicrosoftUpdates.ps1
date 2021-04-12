$ProgressPreference="SilentlyContinue"
$DebugPreference="SilentlyContinue"
$WarningPreference="Continue"
$ErrorActionPreference="Stop"
Set-StrictMode -Version 2

Stop-Service -Name 'wuauserv'
Set-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update' -Name "EnableFeaturedSoftware" -Value 1 -Force
Set-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update' -Name "UserAuthentication" -Value 1 -Force
(New-Object -ComObject Microsoft.Update.ServiceManager).AddService2("7971f918-a847-4430-9279-4a52d1efe18d",7,"" )
