[CmdletBinding()]
param(
    [string]$Version = "2.2.4",
    [string]$TokenFile = "",
    [string]$SourceHost = "127.0.0.1",
    [ValidateSet("auto", "beast", "sbs")]
    [string]$SourceMode = "auto",
    [ValidateRange(1, 65535)]
    [int]$BeastPort = 30005,
    [ValidateRange(1, 65535)]
    [int]$SbsPort = 30003,
    [string]$Label = "ADS-B feeder",
    [string]$StatusListen = "private:54321",
    [string]$AircraftJson = "",
    [ValidateRange(5, 600)]
    [int]$WaitSeconds = 90,
    [switch]$FunctionsOnly
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = "Stop"

function Convert-DerInteger {
    param([byte[]]$Value, [int]$Offset, [int]$Length)
    while ($Length -gt 32 -and $Value[$Offset] -eq 0) {
        $Offset++
        $Length--
    }
    if ($Length -gt 32 -or $Length -lt 1) {
        throw "Invalid ECDSA signature integer."
    }
    $result = New-Object byte[] 32
    [Array]::Copy($Value, $Offset, $result, 32 - $Length, $Length)
    return $result
}

function Read-DerLength {
    param([byte[]]$Value, [ref]$Offset)
    if ($Offset.Value -ge $Value.Length) { throw "Invalid ECDSA signature." }
    $first = [int]$Value[$Offset.Value]
    $Offset.Value++
    if (($first -band 0x80) -eq 0) { return $first }
    $count = $first -band 0x7f
    if ($count -lt 1 -or $count -gt 2 -or $Offset.Value + $count -gt $Value.Length) {
        throw "Invalid ECDSA signature length."
    }
    $length = 0
    for ($index = 0; $index -lt $count; $index++) {
        $length = ($length -shl 8) -bor [int]$Value[$Offset.Value]
        $Offset.Value++
    }
    return $length
}

function Convert-DerEcdsaSignature {
    param([byte[]]$Value)
    $offset = 0
    if ($Value.Length -lt 8 -or $Value[$offset] -ne 0x30) { throw "Invalid ECDSA signature." }
    $offset++
    $sequenceLength = Read-DerLength -Value $Value -Offset ([ref]$offset)
    if ($offset + $sequenceLength -ne $Value.Length) { throw "Invalid ECDSA signature size." }
    if ($Value[$offset] -ne 0x02) { throw "Invalid ECDSA signature." }
    $offset++
    $rLength = Read-DerLength -Value $Value -Offset ([ref]$offset)
    $r = Convert-DerInteger -Value $Value -Offset $offset -Length $rLength
    $offset += $rLength
    if ($offset -ge $Value.Length -or $Value[$offset] -ne 0x02) { throw "Invalid ECDSA signature." }
    $offset++
    $sLength = Read-DerLength -Value $Value -Offset ([ref]$offset)
    $s = Convert-DerInteger -Value $Value -Offset $offset -Length $sLength
    $offset += $sLength
    if ($offset -ne $Value.Length) { throw "Invalid ECDSA signature." }
    $result = New-Object byte[] 64
    [Array]::Copy($r, 0, $result, 0, 32)
    [Array]::Copy($s, 0, $result, 32, 32)
    return $result
}

function Test-ReleaseSignature {
    param([string]$ManifestPath, [string]$SignaturePath)
    # ECDSA P-256 release public key encoded as a Windows CNG ECCPUBLICBLOB.
    $publicBlob = [Convert]::FromBase64String("RUNTMSAAAAAAvGuX0ZF/I1gvn0lHmwzmvfRjhycMEwgOn1myckt8N+6ucXJt3fFfi3QftCI2zc1gAYnpYM+XtISWZkbYZpid")
    $key = [Security.Cryptography.CngKey]::Import($publicBlob, [Security.Cryptography.CngKeyBlobFormat]::EccPublicBlob)
    $ecdsa = New-Object Security.Cryptography.ECDsaCng -ArgumentList $key
    try {
        $ecdsa.HashAlgorithm = [Security.Cryptography.CngAlgorithm]::Sha256
        $manifest = [IO.File]::ReadAllBytes($ManifestPath)
        $signature = Convert-DerEcdsaSignature -Value ([IO.File]::ReadAllBytes($SignaturePath))
        return $ecdsa.VerifyData($manifest, $signature)
    }
    finally {
        $ecdsa.Dispose()
        $key.Dispose()
    }
}

if ($FunctionsOnly) {
    return
}

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Open PowerShell as Administrator and run this installer again."
}
if ($Version -notmatch '^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$') {
    throw "Invalid release version."
}

$architecture = $env:PROCESSOR_ARCHITEW6432
if ([string]::IsNullOrWhiteSpace($architecture)) { $architecture = $env:PROCESSOR_ARCHITECTURE }
switch ($architecture.ToUpperInvariant()) {
    "AMD64" { $platform = "windows-amd64" }
    "ARM64" { $platform = "windows-arm64" }
    default { throw "Unsupported Windows architecture: $architecture" }
}

[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$baseUrl = "https://raw.githubusercontent.com/br3jski/radarview/main/releases/v$Version"
$asset = "adsbpro-feeder-$Version-$platform.zip"
$manifestName = "SHA256SUMS-$Version"
$temporary = Join-Path ([IO.Path]::GetTempPath()) ("adsbpro-feeder-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $temporary | Out-Null

try {
    $archivePath = Join-Path $temporary $asset
    $manifestPath = Join-Path $temporary $manifestName
    $signaturePath = Join-Path $temporary "$manifestName.sig"
    Write-Host "Downloading ADS-B.Pro feeder v$Version for $platform..."
    Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$asset" -OutFile $archivePath
    Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$manifestName" -OutFile $manifestPath
    Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$manifestName.sig" -OutFile $signaturePath

    if (-not (Test-ReleaseSignature -ManifestPath $manifestPath -SignaturePath $signaturePath)) {
        throw "Release signature verification failed. Nothing was installed."
    }
    $escapedAsset = [regex]::Escape($asset)
    $manifestLine = @(Get-Content -LiteralPath $manifestPath | Where-Object { $_ -match "^[0-9a-fA-F]{64}  $escapedAsset$" })
    if ($manifestLine.Count -ne 1) {
        throw "The signed manifest does not contain exactly one checksum for $asset."
    }
    $expectedHash = $manifestLine[0].Substring(0, 64).ToLowerInvariant()
    $actualHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "Release checksum verification failed. Nothing was installed."
    }

    $packageDir = Join-Path $temporary "package"
    Expand-Archive -LiteralPath $archivePath -DestinationPath $packageDir
    $installer = Join-Path $packageDir "install-windows.ps1"
    $binary = Join-Path $packageDir "adsbpro-feeder.exe"
    if (-not (Test-Path -LiteralPath $installer -PathType Leaf) -or -not (Test-Path -LiteralPath $binary -PathType Leaf)) {
        throw "Verified package is incomplete."
    }
    Unblock-File -LiteralPath $installer, $binary
    $installerArguments = @{ Binary = $binary }
    foreach ($argumentName in @("TokenFile", "SourceHost", "SourceMode", "BeastPort", "SbsPort", "Label", "StatusListen", "AircraftJson", "WaitSeconds")) {
        if ($PSBoundParameters.ContainsKey($argumentName)) {
            $installerArguments[$argumentName] = Get-Variable -Name $argumentName -ValueOnly
        }
    }
    & $installer @installerArguments
}
finally {
    Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}
