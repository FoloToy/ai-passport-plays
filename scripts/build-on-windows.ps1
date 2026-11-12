param(
    [switch]$Upload,      # upload the installer and ZIP to the GitHub release v<VERSION>
    [switch]$SkipInstall  # do not install missing tools with winget
)
# One-shot Windows build: installs the toolchain if needed, packages, optionally uploads.
# Usage (inside the cloned repo):
#   powershell -ExecutionPolicy Bypass -File scripts\build-on-windows.ps1 -Upload
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')

function Update-SessionPath {
    $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' + [Environment]::GetEnvironmentVariable('Path', 'User')
}
function Need([string]$Command, [string]$WingetId) {
    if (Get-Command $Command -ErrorAction SilentlyContinue) { return }
    if ($SkipInstall) { throw "$Command is missing; install $WingetId first." }
    Write-Host "Installing $WingetId ..." -ForegroundColor Cyan
    winget install -e --id $WingetId --accept-package-agreements --accept-source-agreements --silent
    Update-SessionPath
    if (!(Get-Command $Command -ErrorAction SilentlyContinue)) { throw "$WingetId installed but $Command is still not on PATH; open a new PowerShell and rerun." }
}

Update-SessionPath
Need git 'Git.Git'
Need go 'GoLang.Go'
$iscc = @((Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6/ISCC.exe'), (Join-Path $env:ProgramFiles 'Inno Setup 6/ISCC.exe'), (Join-Path $env:LOCALAPPDATA 'Programs/Inno Setup 6/ISCC.exe')) | Where-Object { Test-Path $_ } | Select-Object -First 1
if (!$iscc) {
    if ($SkipInstall) { throw 'Inno Setup 6 is missing.' }
    Write-Host 'Installing Inno Setup 6 ...' -ForegroundColor Cyan
    winget install -e --id JRSoftware.InnoSetup --accept-package-agreements --accept-source-agreements --silent
}
if ($Upload) { Need gh 'GitHub.cli' }

# Mirrors that also work inside mainland China. Go toolchains download through GOPROXY.
if (!$env:GOPROXY) { $env:GOPROXY = 'https://goproxy.cn,https://proxy.golang.org,direct' }

git pull --ff-only
if ($LASTEXITCODE -ne 0) { throw "git pull failed; refusing to package stale sources." }
& (Join-Path $PSScriptRoot 'package-windows.ps1')
if ($LASTEXITCODE) { throw "package-windows.ps1 failed ($LASTEXITCODE)" }

if ($Upload) {
    $version = (Get-Content VERSION -Raw).Trim()
    $assets = Get-ChildItem 'dist/windows' -File | Where-Object { $_.Name -match '\.(exe|zip|sha256)$' } | ForEach-Object { $_.FullName }
    gh release upload "v$version" @assets --clobber --repo FoloToy/ai-passport-plays
    if ($LASTEXITCODE) { throw 'gh release upload failed' }
    Write-Host "Uploaded to https://github.com/FoloToy/ai-passport-plays/releases/tag/v$version" -ForegroundColor Green
}
