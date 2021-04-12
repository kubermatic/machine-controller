# envsubst will replace "$${a}" with a single dollar sign.
$${a}ProgressPreference="SilentlyContinue"
$${a}DebugPreference="SilentlyContinue"
$${a}WarningPreference="Continue"
$${a}ErrorActionPreference="Stop"
Set-StrictMode -Version 2

Start-Process "C:\Windows\System32\cscript.exe" -ArgumentList @("C:\Windows\System32\slmgr.vbs", "/ipk", "$win_product_key") -NoNewWindow -Wait

Start-Sleep -Seconds 5

Start-Process "C:\Windows\System32\cscript.exe" -ArgumentList @("C:\Windows\System32\slmgr.vbs", "/ato") -NoNewWindow -Wait

Start-Sleep -Seconds 5

# Debug output
Start-Process "C:\Windows\System32\cscript.exe" -ArgumentList @("C:\Windows\System32\slmgr.vbs", "/dli") -NoNewWindow -Wait
Start-Process "C:\Windows\System32\cscript.exe" -ArgumentList @("C:\Windows\System32\slmgr.vbs", "/dlv") -NoNewWindow -Wait
