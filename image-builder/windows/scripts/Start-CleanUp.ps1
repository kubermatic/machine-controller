$ProgressPreference="SilentlyContinue"
$DebugPreference="SilentlyContinue"
$WarningPreference="Continue"
$ErrorActionPreference="Stop"
Set-StrictMode -Version 2

Stop-Service -Name "wuauserv","BITS","DoSvc" -Force -ErrorAction SilentlyContinue

try {
  Get-ScheduledTask -TaskName packer* | Unregister-ScheduledTask -Confirm:$false
} catch {}

if([bool](Test-Path -Path 'C:\Windows\SoftwareDistribution\Download' )) {
  try {
    Remove-Item -Recurse -Force -Path 'C:\Windows\SoftwareDistribution\Download'
  } catch [System.IO.DirectoryNotFoundException] {}
}

# Remove-ItemProperty throws if the value doesn't exist (even if `-Force` is used).
# therefore we're going to make sure they exist by setting all of them to $null first.
Set-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate' -Name 'PingID' -Value $null -Force -PassThru | Remove-ItemProperty -Force -Name 'PingID'
Set-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate' -Name 'AccountDomainSid' -Value $null -Force -PassThru | Remove-ItemProperty -Force -Name 'AccountDomainSid'
Set-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate' -Name 'SusClientId' -Value $null -Force -PassThru | Remove-ItemProperty -Force -Name 'SusClientId'
Set-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate' -Name 'SusClientIDValidation' -Value $null -Force -PassThru | Remove-ItemProperty -Force -Name 'SusClientIDValidation'
