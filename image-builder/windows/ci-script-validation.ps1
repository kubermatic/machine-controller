#Requires -Version 6

$ProgressPreference="SilentlyContinue"
$DebugPreference="SilentlyContinue"
$WarningPreference="Continue"
$ErrorActionPreference="Stop"
Set-StrictMode -Version 2

# Enable newer TLS protocols. Default still is Tls1.0 and older...
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
# Workaround for automatically detecting proxy settings:
[System.AppContext]::SetSwitch("System.Net.Http.UseSocketsHttpHandler", $false)

Install-Package -Name PowerShellGet,PSScriptAnalyzer -Force -WarningAction SilentlyContinue -Scope CurrentUser
Import-Module PSScriptAnalyzer

$excludeRules = @(
    "PSAvoidUsingConvertToSecureStringWithPlainText"
    "PSAvoidUsingEmptyCatchBlock"
    "PSProvideCommentHelp"
    "PSUseShouldProcessForStateChangingFunctions"
)

$issues = Invoke-ScriptAnalyzer -Path "$((Get-Location).Path)/scripts/" -ExcludeRule $excludeRules
if ($null -ne $issues) {
    $issues | Format-Table -AutoSize | Out-String | Write-Error -Category SyntaxError
}
