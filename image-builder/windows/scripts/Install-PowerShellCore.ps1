$ProgressPreference="SilentlyContinue"
$DebugPreference="SilentlyContinue"
$WarningPreference="Continue"
$ErrorActionPreference="Stop"
Set-StrictMode -Version 2

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
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
$pwshInstallRemotingScriptPath = '"' + "$pwshInstallPath\Install-PowerShellRemoting.ps1" + '"'
$pwshSetupRemoting = @('-NoLogo', '-NonInteractive', '-ExecutionPolicy', 'RemoteSigned', '-NoProfile', '-File', $pwshInstallRemotingScriptPath, '-Force')
Start-Process -FilePath '.\pwsh.exe' -ArgumentList $pwshSetupRemoting -Wait -NoNewWindow
Pop-Location
$path = [Environment]::GetEnvironmentVariable('Path', [System.EnvironmentVariableTarget]::Machine) -split ';'
if ($path -notcontains $pwshInstallPath) { $path += $pwshInstallPath }
[Environment]::SetEnvironmentVariable('Path', $path -join ';', [System.EnvironmentVariableTarget]::Machine)
