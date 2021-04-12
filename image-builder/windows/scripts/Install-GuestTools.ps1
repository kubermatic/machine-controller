$ProgressPreference="SilentlyContinue"
$DebugPreference="SilentlyContinue"
$WarningPreference="Continue"
$ErrorActionPreference="Stop"
Set-StrictMode -Version 2

function Install-Virtio {
  param()
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
  $isoUrl = 'https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/latest-virtio/virtio-win.iso'
  $tempFile = Join-Path -Path $env:TEMP -ChildPath 'virtio-win.iso'
  [System.Net.WebClient]::new().DownloadFile($isoUrl, $tempFile)
  $null = Get-Package -ProviderName msi | Where-Object Name -like 'Virtio-win-guest-tools' | Uninstall-Package -Force -PackageManagementProvider msi
  Start-Sleep -Seconds 5

  $mount = Mount-DiskImage -ImagePath "$TempFile" -StorageType ISO
  $mountDriveLetter = ($mount | Get-Volume).DriveLetter
  # Import Trusted Publisher certificate
  $importedCerts = Import-Certificate -FilePath 'E:\amd64\2k19\vioscsi.cat' -CertStoreLocation Cert:\LocalMachine\TrustedPublisher
  # The import also imported the ca certificates as trusted publishers.
  # Therefore delete all imported certificates again except for the one that we actually want.
  $importedCerts.Where{$_.Thumbprint -ne 'F01DAC89598C52D94FE8CA91187E1853947D115A'}.PSPath | Remove-Item
  $Arguments = @(
    '/i'
    "$($isoDriveLetter):\virtio-win-gt-x64.msi"
    'ADDLOCAL=ALL'
    'REMOVE=FE_spice_Agent,FE_RHEV_Agent'
    '/qn'
    '/quiet'
    '/norestart'
  )
  Start-Process -FilePath 'C:\Windows\System32\msiexec.exe' -ArgumentList $Arguments -Wait -NoNewWindow

  Start-Sleep -Seconds 5
  if (Test-Path -Path $TempFile) {
    Remove-Item -Path $TempFile -Force
  }
}

[bool]$isQEMU = (Get-CimInstance -ClassName Win32_ComputerSystem).Manufacturer.Equals('QEMU')

if ($isQEMU) {
    Install-Virtio
}
