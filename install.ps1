# Grit installer for Windows.
# Downloads a release binary; the repository is not needed at runtime.
$ErrorActionPreference = 'Stop'

# 发布仓库地址，fork 或用镜像时可用 GRIT_REPO 覆盖。
$Repo = if ($env:GRIT_REPO) { $env:GRIT_REPO } else { 'ivaquero/grit' }
$BinDir = if ($env:GRIT_BINDIR) { $env:GRIT_BINDIR } else { Join-Path $HOME 'bin' }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "grit: unsupported architecture '$env:PROCESSOR_ARCHITECTURE'" }
}

$version = $env:GRIT_VERSION
if (-not $version) {
        $releases = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases?per_page=30"
    $latest = $releases | Where-Object { $_.tag_name -match '^v[0-9]' } | Select-Object -First 1
    if (-not $latest) { throw 'grit: no release found; set GRIT_VERSION' }
    $version = $latest.tag_name -replace '^v', ''
}

$name = "grit_${version}_windows_${arch}"
$url = "https://github.com/$Repo/releases/download/v$version/$name.zip"

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

try {
    Write-Host "grit: downloading $version (windows/$arch)"
    $zip = Join-Path $tmp "$name.zip"
    Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
    Expand-Archive -Path $zip -DestinationPath $tmp -Force

    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    $target = Join-Path $BinDir 'grit.exe'
    Move-Item -Path (Join-Path $tmp 'grit.exe') -Destination $target -Force
    Write-Host "grit: installed $target"
}
finally {
    Remove-Item -Path $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

if ($env:PATH -notlike "*$BinDir*") {
    Write-Host "grit: $BinDir is not in PATH"
    Write-Host "  add it with: [Environment]::SetEnvironmentVariable('PATH', `"$BinDir;`$env:PATH`", 'User')"
}

Write-Host ''
Write-Host 'Next: add this line to $PROFILE:'
Write-Host '  grit init --shell pwsh | Out-String | Invoke-Expression'
Write-Host ''
Write-Host 'Optional completion:'
Write-Host '  grit completion --shell pwsh | Out-String | Invoke-Expression'
