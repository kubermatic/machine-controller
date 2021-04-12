$ProgressPreference="SilentlyContinue"
$DebugPreference="SilentlyContinue"
$WarningPreference="Continue"
$ErrorActionPreference="Stop"
Set-StrictMode -Version 2

[bool]$isQEMUKVM = (Get-CimInstance -ClassName Win32_ComputerSystem).Manufacturer.Equals('QEMU')

if ($isQEMUKVM) {
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
  $TempFile = Join-Path -Path $env:TEMP -ChildPath 'CloudbaseInitSetup_x64.msi'
  $TempFileQ = '"' + $TempFile + '"'
  $latestVersionURL = (ConvertFrom-Json -InputObject (Invoke-WebRequest -Uri 'https://api.github.com/repos/cloudbase/cloudbase-init/releases/latest' -UseBasicParsing).Content).assets.where{$_.name.EndsWith("x64.msi")}.browser_download_url
  Invoke-WebRequest -Uri $latestVersionURL -OutFile $TempFile -UseBasicParsing
  Get-Package -ProviderName msi | Where-Object Name -like 'CloudbaseInit*' | Uninstall-Package -Force -PackageManagementProvider msi
  $MSIArguments = @(
    '/i'
    $TempFileQ
    '/qn'
    '/norestart'
  )
  Start-Process -FilePath 'msiexec.exe' -ArgumentList $MSIArguments -Wait -NoNewWindow
  Remove-Item -Path $TempFile -Force

  $configUnattend = @'
[DEFAULT]
username=Administrator
groups=Administrators
inject_user_password=true
config_drive_raw_hhd=true
config_drive_cdrom=true
config_drive_vfat=true
bsdtar_path=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\bin\bsdtar.exe
mtools_path=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\bin\
verbose=true
debug=true
logdir=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\log\
logfile=cloudbase-init-unattend.log
default_log_levels=comtypes=INFO,suds=INFO,iso8601=WARN,requests=WARN
logging_serial_port_settings=COM1,115200,N,8
mtu_use_dhcp_config=true
ntp_use_dhcp_config=true
local_scripts_path=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\LocalScripts\
check_latest_version=true
allow_reboot=true
stop_service_on_exit=false
use_eventlog=true
activate_windows=false
set_kms_product_key=false
first_logon_behaviour=no
add_metadata_private_ip_route=true
enable_automatic_updates=true
trim_enabled=true
metadata_report_provisioning_started=true
metadata_report_provisioning_completed=true
# https://cloudbase-init.readthedocs.io/en/latest/config.html
'@
  $config = @'
[DEFAULT]
username=Administrator
groups=Administrators
inject_user_password=true
config_drive_raw_hhd=true
config_drive_cdrom=true
config_drive_vfat=true
bsdtar_path=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\bin\bsdtar.exe
mtools_path=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\bin\
verbose=true
debug=true
logdir=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\log\
logfile=cloudbase-init.log
default_log_levels=comtypes=INFO,suds=INFO,iso8601=WARN,requests=WARN
logging_serial_port_settings=COM1,115200,N,8
mtu_use_dhcp_config=true
ntp_use_dhcp_config=true
local_scripts_path=C:\Program Files\Cloudbase Solutions\Cloudbase-Init\LocalScripts\
check_latest_version=true
allow_reboot=true
stop_service_on_exit=false
use_eventlog=true
activate_windows=false
set_kms_product_key=false
first_logon_behaviour=no
add_metadata_private_ip_route=true
enable_automatic_updates=true
trim_enabled=true
metadata_report_provisioning_started=true
metadata_report_provisioning_completed=true
# https://cloudbase-init.readthedocs.io/en/latest/config.html
'@
  $config | Out-File -FilePath "C:\Program Files\Cloudbase Solutions\Cloudbase-Init\conf\cloudbase-init.conf" -Encoding ascii -Force
  $configUnattend | Out-File -FilePath "C:\Program Files\Cloudbase Solutions\Cloudbase-Init\conf\cloudbase-init-unattend.conf" -Encoding ascii -Force
}
