[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Binary,
    [string]$TokenFile = "",
    [string]$SourceHost = "127.0.0.1",
    [ValidateSet("auto", "beast", "sbs")]
    [string]$SourceMode = "auto",
    [ValidateRange(1, 65535)]
    [int]$BeastPort = 30005,
    [ValidateRange(1, 65535)]
    [int]$SbsPort = 30003,
    [string]$Label = "ADS-B feeder",
    [ValidateRange(5, 600)]
    [int]$WaitSeconds = 90
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = "Stop"

$ServiceName = "ADSBProFeeder"
$ProgramRoot = Join-Path $env:ProgramFiles "ADSBPro\Feeder"
$ExecutablePath = Join-Path $ProgramRoot "adsbpro-feeder.exe"
$StateRoot = Join-Path $env:ProgramData "ADSBPro\Feeder"
$DataDir = Join-Path $StateRoot "data"
$ConfigPath = Join-Path $StateRoot "config.env"
$BackupDir = Join-Path (Join-Path $StateRoot "backups") ("before-install-" + [DateTime]::UtcNow.ToString("yyyyMMddTHHmmssZ"))
$RollbackSource = Join-Path $PSScriptRoot "rollback-windows.ps1"
$RollbackPath = Join-Path $ProgramRoot "rollback-windows.ps1"

function Assert-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw "Open PowerShell as Administrator and run the installer again."
    }
}

function Write-Utf8NoBom {
    param([string]$Path, [string]$Value)
    $encoding = New-Object System.Text.UTF8Encoding($false)
    [IO.File]::WriteAllText($Path, $Value, $encoding)
}

function Read-SecureToken {
    $secure = Read-Host "ADS-B.Pro feeder token" -AsSecureString
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    }
    finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    }
}

function Get-LegacyToken {
    $candidates = @(
        "C:\radarview.py",
        "C:\radarview\radarview.py",
        "C:\opt\radarview.py"
    )
    foreach ($candidate in $candidates) {
        if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) {
            continue
        }
        $body = [IO.File]::ReadAllText($candidate)
        $match = [regex]::Match($body, '(?m)^\s*USER_TOKEN\s*=\s*[''\"]([^''\"]+)[''\"]')
        if ($match.Success) {
            Write-Host "Using the token from the existing legacy script."
            return $match.Groups[1].Value
        }
    }
    return ""
}

function Invoke-Sc {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments)
    $output = & "$env:SystemRoot\System32\sc.exe" @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "sc.exe failed: $($output -join ' ')"
    }
    return $output
}

Assert-Administrator
if (-not (Test-Path -LiteralPath $Binary -PathType Leaf)) {
    throw "Verified feeder binary was not found: $Binary"
}
if (-not (Test-Path -LiteralPath $RollbackSource -PathType Leaf)) {
    throw "Verified package does not contain rollback-windows.ps1."
}
if ($SourceHost -notmatch '^[A-Za-z0-9._:-]+$') {
    throw "Invalid ADS-B source host."
}
if ([string]::IsNullOrWhiteSpace($Label) -or $Label.Length -gt 255 -or $Label.Contains("`n") -or $Label.Contains("`r")) {
    throw "Invalid feeder label."
}

$previousService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
$previousWasRunning = $null -ne $previousService -and $previousService.Status -eq [ServiceProcess.ServiceControllerStatus]::Running

New-Item -ItemType Directory -Force -Path $ProgramRoot, $DataDir, $BackupDir | Out-Null
if (Test-Path -LiteralPath $ExecutablePath) {
    Copy-Item -LiteralPath $ExecutablePath -Destination (Join-Path $BackupDir "adsbpro-feeder.exe")
}
if (Test-Path -LiteralPath $ConfigPath) {
    Copy-Item -LiteralPath $ConfigPath -Destination (Join-Path $BackupDir "config.env")
}
if (Test-Path -LiteralPath $RollbackPath) {
    Copy-Item -LiteralPath $RollbackPath -Destination (Join-Path $BackupDir "rollback-windows.ps1")
}
Write-Utf8NoBom -Path (Join-Path $BackupDir "service-was-running") -Value ([string]$previousWasRunning)

function Restore-PreviousInstallation {
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    if ($null -eq $previousService) {
        if ($null -ne (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue)) {
            & "$env:SystemRoot\System32\sc.exe" delete $ServiceName | Out-Null
        }
        Remove-Item -LiteralPath $ExecutablePath -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $ConfigPath -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $RollbackPath -Force -ErrorAction SilentlyContinue
        return
    }
    if (Test-Path -LiteralPath (Join-Path $BackupDir "adsbpro-feeder.exe")) {
        Copy-Item -LiteralPath (Join-Path $BackupDir "adsbpro-feeder.exe") -Destination $ExecutablePath -Force
    }
    if (Test-Path -LiteralPath (Join-Path $BackupDir "config.env")) {
        Copy-Item -LiteralPath (Join-Path $BackupDir "config.env") -Destination $ConfigPath -Force
    }
    if (Test-Path -LiteralPath (Join-Path $BackupDir "rollback-windows.ps1")) {
        Copy-Item -LiteralPath (Join-Path $BackupDir "rollback-windows.ps1") -Destination $RollbackPath -Force
    }
    if ($previousWasRunning) {
        Start-Service -Name $ServiceName
    }
}

try {
if ($null -ne $previousService -and $previousService.Status -ne [ServiceProcess.ServiceControllerStatus]::Stopped) {
    Stop-Service -Name $ServiceName -Force
    (Get-Service -Name $ServiceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(30))
}

Copy-Item -LiteralPath $Binary -Destination $ExecutablePath -Force
Copy-Item -LiteralPath $RollbackSource -Destination $RollbackPath -Force

$config = @(
    "SERVER_ADDR=feed.ads-b.pro:48582"
    "SERVER_NAME=feed.ads-b.pro"
    "SOURCE_HOST=$SourceHost"
    "SOURCE_MODE=$SourceMode"
    "BEAST_PORT=$BeastPort"
    "SBS_PORT=$SbsPort"
    "DATA_DIR=$DataDir"
    "TOKEN_FILE=$(Join-Path $DataDir 'pairing-token')"
    "FEEDER_LABEL=$Label"
) -join "`n"
Write-Utf8NoBom -Path $ConfigPath -Value ($config + "`n")

$pairedPath = Join-Path $DataDir "paired"
$pairingTokenPath = Join-Path $DataDir "pairing-token"
if (Test-Path -LiteralPath $pairedPath -PathType Leaf) {
    Remove-Item -LiteralPath $pairingTokenPath -Force -ErrorAction SilentlyContinue
}
else {
    $token = ""
    if (-not [string]::IsNullOrWhiteSpace($TokenFile)) {
        if (-not (Test-Path -LiteralPath $TokenFile -PathType Leaf)) {
            throw "Cannot read the pairing token file."
        }
        $token = ([IO.File]::ReadAllText($TokenFile)).Trim()
    }
    if ([string]::IsNullOrEmpty($token)) {
        $token = Get-LegacyToken
    }
    if ([string]::IsNullOrEmpty($token)) {
        $token = Read-SecureToken
    }
    if ($token -notmatch '^[!-~]{1,255}$') {
        throw "Invalid token format."
    }
    Write-Utf8NoBom -Path $pairingTokenPath -Value $token
    $token = $null
}

# State and private identity are writable only by SYSTEM, Administrators and LocalService.
& "$env:SystemRoot\System32\icacls.exe" $StateRoot "/inheritance:r" "/grant:r" "*S-1-5-18:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" "*S-1-5-19:(OI)(CI)RX" | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Could not protect the feeder configuration directory." }
& "$env:SystemRoot\System32\icacls.exe" $DataDir "/inheritance:r" "/grant:r" "*S-1-5-18:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" "*S-1-5-19:(OI)(CI)F" | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Could not protect the feeder identity directory." }

$serviceCommand = '"{0}" service' -f $ExecutablePath
if ($null -eq $previousService) {
    Invoke-Sc create $ServiceName "binPath=" $serviceCommand "start=" "auto" "obj=" "NT AUTHORITY\LocalService" | Out-Null
}
else {
    Invoke-Sc config $ServiceName "binPath=" $serviceCommand "start=" "auto" "obj=" "NT AUTHORITY\LocalService" | Out-Null
}
Invoke-Sc description $ServiceName "Sends local ADS-B data to ADS-B.Pro using feeder protocol v2." | Out-Null
Invoke-Sc failure $ServiceName "reset=" "86400" "actions=" "restart/5000/restart/15000/restart/60000" | Out-Null

Remove-Item -LiteralPath (Join-Path $DataDir "status.json") -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath (Join-Path $DataDir "status.json.new") -Force -ErrorAction SilentlyContinue
Start-Service -Name $ServiceName

$active = $false
$permanentError = ""
$statusPath = Join-Path $DataDir "status.json"
for ($attempt = 0; $attempt -lt $WaitSeconds; $attempt++) {
    if (Test-Path -LiteralPath $statusPath -PathType Leaf) {
        try {
            $status = Get-Content -LiteralPath $statusPath -Raw | ConvertFrom-Json
            if ($status.state -eq "active" -and (Get-Service -Name $ServiceName).Status -eq [ServiceProcess.ServiceControllerStatus]::Running) {
                $active = $true
                break
            }
            if ($status.error -match "PAIRING_WINDOW_REQUIRED") { $permanentError = "PAIRING_WINDOW_REQUIRED"; break }
            if ($status.error -match "INVALID_TOKEN") { $permanentError = "INVALID_TOKEN"; break }
        }
        catch {
            # The status file is replaced atomically; retry if a security product briefly locks it.
        }
    }
    Start-Sleep -Seconds 1
}

if (-not $active) {
    switch ($permanentError) {
        "PAIRING_WINDOW_REQUIRED" { throw "Open a 10-minute pairing window in the ADS-B.Pro account panel and run the installer again." }
        "INVALID_TOKEN" { throw "The feeder token was rejected." }
        default { throw "Feeder did not reach ACTIVE within $WaitSeconds seconds. Check $DataDir\feeder.log" }
    }
}
}
catch {
    $originalError = $_
    try {
        Restore-PreviousInstallation
    }
    catch {
        Write-Warning "Automatic restoration also failed: $($_.Exception.Message)"
    }
    throw $originalError
}

Write-Host "Feeder v2 is ACTIVE."
Write-Host "Status: & '$ExecutablePath' status"
Write-Host "Log: $DataDir\feeder.log"
Write-Host "Backup: $BackupDir"
Write-Host "Remove service: & '$RollbackPath'"
