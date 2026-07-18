$ErrorActionPreference = "Stop"
Set-StrictMode -Version 2.0

foreach ($script in @("radarview_setup.ps1", "install-windows.ps1", "rollback-windows.ps1", "scripts/test-windows.ps1")) {
    $errors = $null
    [void][Management.Automation.Language.Parser]::ParseFile((Resolve-Path $script), [ref]$null, [ref]$errors)
    if ($errors.Count -ne 0) {
        throw "PowerShell parser errors in ${script}: $($errors -join '; ')"
    }
}

go test ./...
if ($LASTEXITCODE -ne 0) { throw "Go tests failed." }

$binary = Join-Path $env:RUNNER_TEMP "adsbpro-feeder.exe"
go build -trimpath -o $binary ./cmd/adsbpro-feeder
if ($LASTEXITCODE -ne 0) { throw "Windows build failed." }
$version = & $binary version
if ($LASTEXITCODE -ne 0 -or $version.Trim() -ne "2.1.0") { throw "Windows binary smoke test failed." }

. ./radarview_setup.ps1 -FunctionsOnly
$manifest = Resolve-Path "releases/v2.0.0/SHA256SUMS-2.0.0"
$signature = Resolve-Path "releases/v2.0.0/SHA256SUMS-2.0.0.sig"
if (-not (Test-ReleaseSignature -ManifestPath $manifest -SignaturePath $signature)) {
    throw "Windows release signature verification failed."
}
$tampered = Join-Path $env:RUNNER_TEMP "SHA256SUMS-tampered"
[IO.File]::WriteAllText($tampered, ([IO.File]::ReadAllText($manifest) + "tampered"))
if (Test-ReleaseSignature -ManifestPath $tampered -SignaturePath $signature) {
    throw "Tampered release manifest was accepted."
}

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
    & sc.exe create $serviceName "binPath= $serviceCommand" "start= demand" "obj= NT AUTHORITY\LocalService" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Could not create the Windows test service." }
    Start-Service -Name $serviceName
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(20))
    Stop-Service -Name $serviceName
    (Get-Service -Name $serviceName).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(20))
}
finally {
    if ($null -ne (Get-Service -Name $serviceName -ErrorAction SilentlyContinue)) {
        & sc.exe delete $serviceName | Out-Null
    }
}

Write-Host "All Windows feeder tests passed."
