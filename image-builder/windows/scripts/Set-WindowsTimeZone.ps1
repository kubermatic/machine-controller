$ProgressPreference="SilentlyContinue"
$DebugPreference="SilentlyContinue"
$WarningPreference="Continue"
$ErrorActionPreference="Stop"
Set-StrictMode -Version 2

# This script is necessary because setting the timezone
# via the Autounattend.xml file alone isn't reliable anymore.
Set-TimeZone -Id "UTC"
