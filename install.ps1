# Goboot installer for Windows.
# Downloads a release binary; the repository is not needed at runtime.
$ErrorActionPreference = 'Stop'

# 发布仓库地址；GitHub 上改名后同步更新。
$Repo = if ($env:GOBOOT_REPO) { $env:GOBOOT_REPO } else { 'ivaquero/goboot' }
$BinDir = if ($env:GOBOOT_BINDIR) { $env:GOBOOT_BINDIR } else { Join-Path $HOME 'bin' }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "goboot: unsupported architecture '$env:PROCESSOR_ARCHITECTURE'" }
}

$version = $env:GOBOOT_VERSION
if (-not $version) {
        $releases = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases?per_page=30"
    $latest = $releases | Where-Object { $_.tag_name -match '^v[0-9]' } | Select-Object -First 1
    if (-not $latest) { throw 'goboot: no release found; set GOBOOT_VERSION' }
    $version = $latest.tag_name -replace '^v', ''
}

$name = "ox_${version}_windows_${arch}"
$url = "https://github.com/$Repo/releases/download/v$version/$name.zip"

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

try {
    Write-Host "goboot: downloading $version (windows/$arch)"
    $zip = Join-Path $tmp "$name.zip"
    Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
    Expand-Archive -Path $zip -DestinationPath $tmp -Force

    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    $target = Join-Path $BinDir 'goboot.exe'
    Move-Item -Path (Join-Path $tmp 'goboot.exe') -Destination $target -Force
    Write-Host "goboot: installed $target"
}
finally {
    Remove-Item -Path $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

if ($env:PATH -notlike "*$BinDir*") {
    Write-Host "goboot: $BinDir is not in PATH"
    Write-Host "  add it with: [Environment]::SetEnvironmentVariable('PATH', `"$BinDir;`$env:PATH`", 'User')"
}

Write-Host ''
Write-Host 'Next: add this line to $PROFILE:'
Write-Host '  goboot init --shell pwsh | Out-String | Invoke-Expression'
Write-Host ''
Write-Host 'Optional completion:'
Write-Host '  goboot completion --shell pwsh | Out-String | Invoke-Expression'
