param(
    [string]$OutputDir = 'dist/windows',
    [string]$InnoCompiler = $env:PLAYS_ISCC,
    [switch]$AllowDirty
)
# Build the Windows x64 installer (Setup.exe) and portable ZIP of AI Passport 玩法社区.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Set-Location (Join-Path $PSScriptRoot '..')
if ($env:OS -ne 'Windows_NT') { throw 'Run this packager on Windows x64.' }
if (-not $AllowDirty -and (git status --porcelain)) { throw 'Commit the source tree before packaging (or pass -AllowDirty).' }

$version = (Get-Content VERSION -Raw).Trim()
$env:GOTOOLCHAIN = 'go1.26.0'
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$exeName = 'AIPassportPlays.exe'

function Invoke-Checked {
    param([string]$File, [string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE" }
}

$OutputDir = [IO.Path]::GetFullPath((Join-Path (Get-Location) $OutputDir))
$package = Join-Path $OutputDir 'AI Passport Plays'
if (Test-Path $OutputDir) { Remove-Item $OutputDir -Recurse -Force }
New-Item -ItemType Directory -Force $package | Out-Null

Invoke-Checked 'go' @('test', './...')
Invoke-Checked 'go' @('build', '-tags', 'production', '-trimpath', '-ldflags', "-s -w -H windowsgui -X main.version=$version", '-o', (Join-Path $package $exeName), '.')

$thirdParty = Join-Path $package 'ThirdParty'
New-Item -ItemType Directory -Force $thirdParty | Out-Null
Copy-Item 'THIRD_PARTY.md' $thirdParty
$stubLicenses = Join-Path $thirdParty 'esp-flasher-stub'
New-Item -ItemType Directory -Force $stubLicenses | Out-Null
Copy-Item 'pkg/esp/stub/LICENSE-APACHE', 'pkg/esp/stub/LICENSE-MIT' $stubLicenses
Copy-Item 'README.md' $package
(git rev-parse HEAD).Trim() | Set-Content (Join-Path $package 'source-commit.txt') -Encoding ascii

# WebView2 is preinstalled on Windows 11; ship Microsoft's bootstrapper for Windows 10.
$runtime = Join-Path $package 'runtime'
New-Item -ItemType Directory -Force $runtime | Out-Null
$bootstrapper = Join-Path $runtime 'MicrosoftEdgeWebview2Setup.exe'
Invoke-WebRequest 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $bootstrapper -UseBasicParsing
$signature = Get-AuthenticodeSignature $bootstrapper
if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch 'O=Microsoft Corporation') {
    throw 'Microsoft WebView2 bootstrapper signature verification failed.'
}

@"
AI Passport 玩法社区 $version / Windows x64

安装版：运行 AI-Passport-Plays-$version-Windows-x64-Setup.exe。
免安装版：完整解压整个文件夹后运行 $exeName，请保留旁边的 ThirdParty、runtime 目录。
不需要安装 Python。若提示缺少 WebView2，请运行 runtime\MicrosoftEdgeWebview2Setup.exe（需要联网）。
本测试版没有代码签名，Windows 可能提示"已保护你的电脑"，点"更多信息 → 仍要运行"。

Installer: run the Setup.exe. Portable: extract the whole folder and run $exeName.
"@ | Set-Content (Join-Path $package 'START-HERE.txt') -Encoding utf8

# The app is a GUI program; piping its output makes PowerShell wait for it.
$selfTest = & (Join-Path $package $exeName) -self-test | Out-String
if ($LASTEXITCODE -ne 0) { throw "Packaged self-test failed: $selfTest" }
if (-not ($selfTest | ConvertFrom-Json).passed) { throw "Packaged self-test failed: $selfTest" }

$zip = Join-Path $OutputDir "AI-Passport-Plays-$version-Windows-x64.zip"
Compress-Archive -Path $package -DestinationPath $zip -CompressionLevel Optimal

$iscc = $InnoCompiler
if (!$iscc) { $iscc = Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6/ISCC.exe' }
if (!(Test-Path $iscc)) { $iscc = Join-Path $env:ProgramFiles 'Inno Setup 6/ISCC.exe' }
if (!(Test-Path $iscc)) { $iscc = Join-Path $env:LOCALAPPDATA 'Programs/Inno Setup 6/ISCC.exe' }
if (!(Test-Path $iscc)) { throw 'Install Inno Setup 6 before packaging the installer.' }
$icon = (Resolve-Path 'assets/icon.ico').Path
Invoke-Checked $iscc @('/Q', "/DPackageDir=$package", "/DOutputDir=$OutputDir", "/DAppVersion=$version", "/DIconFile=$icon", 'scripts/windows-installer.iss')

Get-ChildItem $OutputDir -File | Where-Object { $_.Extension -in '.zip', '.exe' } | ForEach-Object {
    $hash = (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $($_.Name)" | Set-Content ($_.FullName + '.sha256') -Encoding ascii
}
Write-Host "Packaged Windows x64 files in $OutputDir"
Get-ChildItem $OutputDir -File | Format-Table Name, Length
