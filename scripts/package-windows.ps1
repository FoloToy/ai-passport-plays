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
$workerName = 'ai-passport-plays-worker'

function Invoke-Checked {
    param([string]$File, [string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE" }
}

# Pick Python 3.12+: PLAYS_PYTHON, the py launcher, or python on PATH.
$pyFile = 'python'; $pyArgs = @()
if ($env:PLAYS_PYTHON) { $pyFile = $env:PLAYS_PYTHON }
elseif (Get-Command py -ErrorAction SilentlyContinue) { $pyFile = 'py'; $pyArgs = @('-3.12') }
Invoke-Checked $pyFile ($pyArgs + @('-c', 'import sys,struct; assert sys.version_info >= (3,12) and struct.calcsize(chr(80))==8'))

$OutputDir = [IO.Path]::GetFullPath((Join-Path (Get-Location) $OutputDir))
$package = Join-Path $OutputDir 'AI Passport Plays'
if (Test-Path $OutputDir) { Remove-Item $OutputDir -Recurse -Force }
New-Item -ItemType Directory -Force $package | Out-Null

if (!(Test-Path '.runtime/worker-venv-windows/Scripts/python.exe')) {
    Invoke-Checked $pyFile ($pyArgs + @('-m', 'venv', '.runtime/worker-venv-windows'))
}
$python = (Resolve-Path '.runtime/worker-venv-windows/Scripts/python.exe').Path
Invoke-Checked $python @('-m', 'pip', 'install', '-q', '-r', 'worker/requirements-windows.lock')
Invoke-Checked $python @('-m', 'unittest', 'discover', '-s', 'worker', '-p', 'test_*.py')
Invoke-Checked $python @('-m', 'PyInstaller', '--noconfirm', '--clean', '--onedir', '--console', '--name', $workerName,
    '--distpath', '.runtime/worker-dist-windows', '--workpath', '.runtime/worker-build-windows', '--specpath', '.runtime',
    '--collect-all', 'esptool', '--collect-all', 'esp_pylib', 'worker/main.py')
Copy-Item ".runtime/worker-dist-windows/$workerName" (Join-Path $package 'worker') -Recurse

Invoke-Checked 'go' @('test', './...')
Invoke-Checked 'go' @('build', '-tags', 'production', '-trimpath', '-ldflags', "-s -w -H windowsgui -X main.version=$version", '-o', (Join-Path $package $exeName), '.')

$thirdParty = Join-Path $package 'ThirdParty'
New-Item -ItemType Directory -Force $thirdParty | Out-Null
Copy-Item 'worker/main.py', 'worker/requirements.txt', 'worker/requirements.lock', 'worker/requirements-windows.lock', 'THIRD_PARTY.md' $thirdParty
Invoke-Checked $python @('scripts/collect-licenses.py', (Join-Path $thirdParty 'licenses'))
if (!(Test-Path '.runtime/sources/esptool-5.6.0.tar.gz')) {
    Invoke-Checked $python @('-m', 'pip', 'download', '-q', '--no-deps', '--no-binary=:all:', 'esptool==5.6.0', '-d', '.runtime/sources')
}
Copy-Item '.runtime/sources/esptool-5.6.0.tar.gz' $thirdParty
(& $python -m pip freeze) | Set-Content (Join-Path $thirdParty 'python-packages.txt') -Encoding utf8
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
免安装版：完整解压整个文件夹后运行 $exeName，请保留旁边的 worker、ThirdParty、runtime 目录。
不需要安装 Python。若提示缺少 WebView2，请运行 runtime\MicrosoftEdgeWebview2Setup.exe（需要联网）。
本测试版没有代码签名，Windows 可能提示"已保护你的电脑"，点"更多信息 → 仍要运行"。

Installer: run the Setup.exe. Portable: extract the whole folder and run $exeName.
"@ | Set-Content (Join-Path $package 'START-HERE.txt') -Encoding utf8

$worker = Join-Path $package "worker/$workerName.exe"
Invoke-Checked $worker @('version')
Invoke-Checked $worker @('self-test')
$ports = & $worker ports
if ($LASTEXITCODE -ne 0) { throw 'Packaged USB enumeration failed.' }
$ports | ConvertFrom-Json | Out-Null

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
