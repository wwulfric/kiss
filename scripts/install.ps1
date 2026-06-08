param(
    [string]$Repo = $(if ($env:KISS_REPO) { $env:KISS_REPO } else { "wwulfric/kiss" }),
    [string]$Version = $(if ($env:KISS_VERSION) { $env:KISS_VERSION } else { "latest" }),
    [string]$InstallDir = $(if ($env:KISS_INSTALL_DIR) { $env:KISS_INSTALL_DIR } elseif ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA "Programs\kiss\bin" } else { Join-Path $HOME ".kiss\bin" }),
    [switch]$NoPathUpdate
)

$ErrorActionPreference = "Stop"

if ($env:OS -and $env:OS -ne "Windows_NT") {
    throw "unsupported OS: install.ps1 is for Windows"
}

if ($env:KISS_NO_PATH_UPDATE -eq "1") {
    $NoPathUpdate = $true
}

$verifySignature = $env:KISS_VERIFY_SIGNATURE -eq "1"
$artifactVersion = $Version
if ($Version -eq "latest") {
    $latest = Invoke-RestMethod `
        -Uri "https://api.github.com/repos/$Repo/releases/latest" `
        -Headers @{ "User-Agent" = "kiss-installer" }
    $artifactVersion = $latest.tag_name
}

$arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
switch ($arch) {
    "x64" { $artifactArch = "amd64" }
    default { throw "unsupported architecture: $arch; current Windows release provides amd64" }
}

$baseUrl = "https://github.com/$Repo/releases/download/$artifactVersion"
$archive = "kiss_${artifactVersion}_windows_${artifactArch}.tar.gz"
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("kiss-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

function Download-File {
    param(
        [string]$Uri,
        [string]$OutFile
    )
    Invoke-WebRequest -Uri $Uri -OutFile $OutFile -Headers @{ "User-Agent" = "kiss-installer" }
}

function Path-Contains {
    param(
        [string]$PathValue,
        [string]$Entry
    )
    if ([string]::IsNullOrWhiteSpace($PathValue)) {
        return $false
    }
    $normalizedEntry = Normalize-PathEntry $Entry
    foreach ($part in $PathValue -split ';') {
        if ([string]::IsNullOrWhiteSpace($part)) {
            continue
        }
        $normalizedPart = Normalize-PathEntry $part
        if ([string]::Equals($normalizedPart, $normalizedEntry, [System.StringComparison]::OrdinalIgnoreCase)) {
            return $true
        }
    }
    return $false
}

function Normalize-PathEntry {
    param(
        [string]$Entry
    )
    try {
        return [System.IO.Path]::GetFullPath($Entry).TrimEnd('\')
    }
    catch {
        return $Entry.Trim().TrimEnd('\')
    }
}

try {
    $archivePath = Join-Path $tempDir $archive
    $checksumsPath = Join-Path $tempDir "checksums.txt"

    Download-File "$baseUrl/$archive" $archivePath
    Download-File "$baseUrl/checksums.txt" $checksumsPath

    if ($verifySignature) {
        $cosign = Get-Command cosign -ErrorAction SilentlyContinue
        if (-not $cosign) {
            throw "KISS_VERIFY_SIGNATURE=1 requires cosign in PATH"
        }
        $signaturePath = Join-Path $tempDir "checksums.txt.sig"
        $certificatePath = Join-Path $tempDir "checksums.txt.pem"
        Download-File "$baseUrl/checksums.txt.sig" $signaturePath
        Download-File "$baseUrl/checksums.txt.pem" $certificatePath
        & cosign verify-blob `
            --certificate $certificatePath `
            --signature $signaturePath `
            --certificate-identity "https://github.com/$Repo/.github/workflows/release.yml@refs/tags/$artifactVersion" `
            --certificate-oidc-issuer "https://token.actions.githubusercontent.com" `
            $checksumsPath
        if ($LASTEXITCODE -ne 0) {
            throw "cosign verification failed"
        }
    }

    $checksumLine = Get-Content $checksumsPath | Where-Object { $_ -match "\s+$([regex]::Escape($archive))$" } | Select-Object -First 1
    if (-not $checksumLine) {
        throw "missing checksum for $archive"
    }
    $expectedHash = ($checksumLine -split '\s+')[0].ToLowerInvariant()
    $actualHash = (Get-FileHash -Algorithm SHA256 -Path $archivePath).Hash.ToLowerInvariant()
    if ($expectedHash -ne $actualHash) {
        throw "checksum mismatch for $archive"
    }

    $tar = Get-Command tar -ErrorAction SilentlyContinue
    if (-not $tar) {
        throw "tar is required to extract $archive"
    }
    & tar -C $tempDir -xzf $archivePath
    if ($LASTEXITCODE -ne 0) {
        throw "tar extraction failed"
    }

    $kissExe = Join-Path $tempDir "kiss.exe"
    if (-not (Test-Path $kissExe)) {
        throw "archive did not contain kiss.exe"
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $target = Join-Path $InstallDir "kiss.exe"
    Copy-Item -Force $kissExe $target

    if (-not $NoPathUpdate) {
        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        if (-not (Path-Contains $userPath $InstallDir)) {
            $newUserPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $InstallDir } else { "$userPath;$InstallDir" }
            [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
            Write-Host "Added $InstallDir to the user PATH. Open a new terminal to use kiss from PATH."
        }
        if (-not (Path-Contains $env:Path $InstallDir)) {
            $env:Path = "$env:Path;$InstallDir"
        }
    }

    Write-Host "Installed kiss to $target"
}
finally {
    Remove-Item -Recurse -Force $tempDir -ErrorAction SilentlyContinue
}
