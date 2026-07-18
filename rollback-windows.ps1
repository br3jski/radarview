[CmdletBinding()]
param()

Set-StrictMode -Version 2.0
$ErrorActionPreference = "Stop"

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Open PowerShell as Administrator and run this script again."
}

$service = Get-Service -Name "ADSBProFeeder" -ErrorAction SilentlyContinue
if ($null -ne $service) {
    if ($service.Status -ne [ServiceProcess.ServiceControllerStatus]::Stopped) {
        Stop-Service -Name "ADSBProFeeder" -Force
    }
    & "$env:SystemRoot\System32\sc.exe" delete "ADSBProFeeder" | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Could not remove the ADSBProFeeder service."
    }
}

Write-Host "The ADS-B.Pro Windows service was removed."
Write-Host "The installation identity in $env:ProgramData\ADSBPro\Feeder was retained."
