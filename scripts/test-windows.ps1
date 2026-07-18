[CmdletBinding()]
param([switch]$PowerShellOnly)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version 2.0

foreach ($script in @("radarview_setup.ps1", "install-windows.ps1", "rollback-windows.ps1", "scripts/test-windows.ps1")) {
    $errors = $null
    [void][Management.Automation.Language.Parser]::ParseFile((Resolve-Path $script), [ref]$null, [ref]$errors)
    if ($errors.Count -ne 0) {
        throw "PowerShell parser errors in ${script}: $($errors -join '; ')"
    }
}

. ./radarview_setup.ps1 -FunctionsOnly
$manifest = Resolve-Path "releases/v2.2.1/SHA256SUMS-2.2.1"
$signature = Resolve-Path "releases/v2.2.1/SHA256SUMS-2.2.1.sig"
if (-not (Test-ReleaseSignature -ManifestPath $manifest -SignaturePath $signature)) {
    throw "Windows release signature verification failed."
}
$tampered = Join-Path $env:RUNNER_TEMP "SHA256SUMS-tampered"
[IO.File]::WriteAllText($tampered, ([IO.File]::ReadAllText($manifest) + "tampered"))
if (Test-ReleaseSignature -ManifestPath $tampered -SignaturePath $signature) {
    throw "Tampered release manifest was accepted."
}

if ($PowerShellOnly) {
    Write-Host "Windows PowerShell compatibility tests passed."
    return
}

go test ./...
if ($LASTEXITCODE -ne 0) { throw "Go tests failed." }

$binary = Join-Path $env:RUNNER_TEMP "adsbpro-feeder.exe"
go build -trimpath -o $binary ./cmd/adsbpro-feeder
if ($LASTEXITCODE -ne 0) { throw "Windows build failed." }
$version = & $binary version
if ($LASTEXITCODE -ne 0 -or $version.Trim() -ne "2.2.1") { throw "Windows binary smoke test failed." }

$serviceName = "ADSBProFeeder"
$stateRoot = Join-Path $env:ProgramData "ADSBPro\Feeder"
$dataDir = Join-Path $stateRoot "data"
$configPath = Join-Path $stateRoot "config.env"
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
[IO.File]::WriteAllText($configPath, "DATA_DIR=$dataDir`nSOURCE_HOST=127.0.0.1`nBEAST_PORT=1`nSBS_PORT=1`n")
& icacls.exe $stateRoot "/grant" "*S-1-5-19:(OI)(CI)RX" | Out-Null
& icacls.exe $dataDir "/grant" "*S-1-5-19:(OI)(CI)F" | Out-Null

$serviceCommand = '"{0}" service' -f $binary
try {
    $createOutput = & sc.exe create $serviceName "binPath=" $serviceCommand "start=" "demand" "obj=" "NT AUTHORITY\LocalService" 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Could not create the Windows test service: $($createOutput -join ' ')" }
    Start-Service -Name $serviceName
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(20))
    $statusResponse = $null
    for ($attempt = 0; $attempt -lt 40 -and $null -eq $statusResponse; $attempt++) {
        try { $statusResponse = Invoke-RestMethod -UseBasicParsing -Uri "http://127.0.0.1:54321/api/status" }
        catch { Start-Sleep -Milliseconds 250 }
    }
    if ($null -eq $statusResponse) { throw "Windows status page did not start." }
    if ($statusResponse.version -ne "2.2.1" -or [string]::IsNullOrWhiteSpace($statusResponse.installationId)) {
        throw "Windows status page API smoke test failed."
    }
    $statusPage = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:54321/"
    if ($statusPage.Content -notmatch "UPDATE AVAILABLE" -or $statusPage.Content -notmatch "No feeder token") {
        throw "Windows status page smoke test failed."
    }
    Stop-Service -Name $serviceName
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(20))
}
finally {
    if ($null -ne (Get-Service -Name $serviceName -ErrorAction SilentlyContinue)) {
        & sc.exe delete $serviceName | Out-Null
    }
}

Write-Host "All Windows feeder tests passed."
