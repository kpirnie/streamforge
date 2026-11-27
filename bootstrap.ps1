# Install a StreamForge release for the current Windows user.
$ErrorActionPreference = 'Stop'

$machine = $env:PROCESSOR_ARCHITEW6432
if (!$machine) { $machine = $env:PROCESSOR_ARCHITECTURE }
$arch = switch ($machine) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    'x86' { '386' }
    default { throw "No StreamForge release for Windows $machine." }
}

$missing = foreach ($tool in 'ffmpeg.exe', 'ffprobe.exe') {
    $command = Get-Command $tool -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if (!$command) {
        $tool
        continue
    }
    try {
        & $command.Source -version *> $null
        if ($LASTEXITCODE -ne 0) {
            $tool
            continue
        }
        $protocols = & $command.Source -protocols 2>$null
        if ($LASTEXITCODE -ne 0 -or !($protocols -match '^\s+fd\s*$')) {
            "$tool (requires fd protocol)"
        }
    } catch {
        $tool
    }
}
if ($missing) {
    $ffmpegPackage = switch ($arch) {
        '386' { 'Defisym.FFmpeg.GPL' }
        'arm64' { 'yt-dlp.FFmpeg' }
        default { 'Gyan.FFmpeg' }
    }
    throw "StreamForge requires runnable FFmpeg tools on PATH. Missing: $($missing -join ', '). Run: winget install --id $ffmpegPackage --exact"
}

$repo = 'https://github.com/kpirnie/streamforge/releases'
$url = "$repo/latest/download"
if ($env:STREAMFORGE_RELEASE) {
    $tag = [uri]::EscapeDataString($env:STREAMFORGE_RELEASE)
    $url = "$repo/download/$tag"
}
$binDir = $env:STREAMFORGE_BIN_DIR
if (!$binDir) { $binDir = Join-Path $env:LOCALAPPDATA 'StreamForge\bin' }
$null = New-Item -ItemType Directory -Force -Path $binDir
$destination = Join-Path $binDir 'streamforge.exe'
$asset = "streamforge-windows-$arch.exe"
$temp = Join-Path $binDir ('.streamforge-' + [guid]::NewGuid())
$null = New-Item -ItemType Directory -Path $temp
try {
    $download = Join-Path $temp $asset
    $checksums = Join-Path $temp 'SHA256SUMS'
    Invoke-WebRequest -UseBasicParsing -Uri "$url/$asset" -OutFile $download
    Invoke-WebRequest -UseBasicParsing -Uri "$url/SHA256SUMS" -OutFile $checksums
    $entry = '^([a-fA-F0-9]{64})\s+\*?' + [regex]::Escape($asset) + '$'
    $hashes = @(Select-String -LiteralPath $checksums -Pattern $entry)
    if ($hashes.Count -ne 1 -or (Get-FileHash -LiteralPath $download -Algorithm SHA256).Hash -ne $hashes[0].Matches[0].Groups[1].Value) {
        throw "Checksum verification failed for $asset."
    }
    if (Test-Path -LiteralPath $destination) {
        $existing = Get-Item -LiteralPath $destination -Force
        if ($existing.PSIsContainer -or ($existing.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw "Refusing to replace $destination because it is not a regular file."
        }
        [IO.File]::Replace($download, $destination, $null)
    } else {
        [IO.File]::Move($download, $destination)
    }
} finally {
    Remove-Item -LiteralPath $temp -Recurse -Force
}
Write-Host "Installed StreamForge at $destination"
if (($env:PATH -split ';') -notcontains $binDir) {
    Write-Host "Add $binDir to PATH to run streamforge by name."
}
