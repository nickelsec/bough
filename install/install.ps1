# Installs bough on Windows.
#
#   powershell -c "irm https://www.bough.run/install.ps1 | iex"
#
# This is piped into a shell, which means nobody reads it before it runs. So it
# verifies the checksum of what it downloaded before extracting anything, and
# it touches nothing outside the install directory and a temp dir it removes on
# the way out.
#
# $env:BOUGH_VERSION pins a release, $env:BOUGH_DIR chooses where the binary
# lands. Both are optional.

$ErrorActionPreference = 'Stop'

$repo = 'nickelsec/bough'
$releases = "https://github.com/$repo/releases"

function Die($msg) {
    Write-Host "bough: $msg" -ForegroundColor Red
    exit 1
}

# TLS 1.2 is not the default on Windows PowerShell 5.1, and without it every
# download here fails with an error that says nothing about the cause.
try {
    [Net.ServicePointManager]::SecurityProtocol =
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
} catch {}

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { Die "no build for $($env:PROCESSOR_ARCHITECTURE). The releases page lists what there is: $releases" }
}

# Which release. Reading the tag from the API rather than hardcoding one is
# what keeps this script working after a release without anybody editing it.
if ($env:BOUGH_VERSION) {
    $tag = $env:BOUGH_VERSION
} else {
    try {
        $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -UseBasicParsing
        $tag = $latest.tag_name
    } catch {
        Die "could not work out the latest version. GitHub may be rate limiting; try again, or set BOUGH_VERSION"
    }
}
if (-not $tag) { Die "could not work out the latest version" }

$archive = "bough_${tag}_windows_${arch}.zip"
$base = "$releases/download/$tag"

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("bough-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    Write-Host "Downloading bough $tag for windows/$arch"
    $zip = Join-Path $tmp $archive
    $sums = Join-Path $tmp 'checksums.txt'

    try {
        Invoke-WebRequest -Uri "$base/$archive" -OutFile $zip -UseBasicParsing
    } catch {
        Die "could not download $archive. Is $tag a real release?"
    }
    try {
        Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile $sums -UseBasicParsing
    } catch {
        Die "could not download the checksums for $tag"
    }

    # The integrity check. Everything above this point came off the network.
    $line = Get-Content $sums | Where-Object { $_ -match "\s\*?$([regex]::Escape($archive))$" } | Select-Object -First 1
    if (-not $line) { Die "$archive is not listed in checksums.txt" }
    $want = ($line -split '\s+')[0]
    $got = (Get-FileHash -Path $zip -Algorithm SHA256).Hash.ToLower()

    if ($want.ToLower() -ne $got) {
        Die "checksum mismatch. Expected $want, got $got. Not installing."
    }

    Expand-Archive -Path $zip -DestinationPath $tmp -Force
    $binary = Join-Path $tmp "bough_${tag}_windows_${arch}\bough.exe"
    if (-not (Test-Path $binary)) { Die "the archive did not contain the binary where expected" }

    $dir = if ($env:BOUGH_DIR) { $env:BOUGH_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\bough' }
    New-Item -ItemType Directory -Path $dir -Force | Out-Null

    # A running bough holds its own exe open, so replacing it fails with a
    # message about the file being in use rather than anything about bough.
    try {
        Move-Item -Path $binary -Destination (Join-Path $dir 'bough.exe') -Force
    } catch {
        Die "could not write to $dir. If bough is running, close it and try again."
    }

    Write-Host "Installed to $dir\bough.exe"

    # Saying the binary is installed while the shell cannot find it is the most
    # common way one of these scripts wastes somebody's afternoon. The second
    # way is an older copy sitting earlier in PATH: everybody installing this
    # today has one from go install in ~\go\bin, and without this they would
    # run that copy for weeks while believing they had upgraded.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath -notlike "*$dir*") {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
        Write-Host ""
        Write-Host "Added $dir to your PATH."
        Write-Host "Open a new terminal, then run: bough"
    } else {
        Write-Host "Run: bough"
    }

    # Checked whichever branch ran above. A first install is exactly when the
    # older copy is most likely to be there, so this cannot sit inside the
    # branch that only runs on a reinstall.
    $found = (Get-Command bough -ErrorAction SilentlyContinue).Source
    if ($found -and $found -ne (Join-Path $dir 'bough.exe')) {
        Write-Host ""
        Write-Host "Note: another bough is earlier on your PATH and will be"
        Write-Host "used instead:"
        Write-Host "  $found"
        Write-Host ""
        Write-Host "Remove it, or run this one directly: $dir\bough.exe"
    }
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
