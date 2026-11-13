#!/bin/bash
# Build the Windows 7 / 8 / 8.1 edition (x64 and 32-bit ZIPs) from macOS or Linux.
# Current Go and WebView2 no longer run on Windows 7, so this edition is compiled with
# go-legacy-win7 (a Go fork patched for Windows 7) and shows the UI in Chrome 109.
# Env: PLAYS_DIST (output dir), ALLOW_DIRTY=1 (skip clean-tree check).
set -euo pipefail
cd "$(dirname "$0")/.."

# Pinned toolchain: https://github.com/thongtech/go-legacy-win7/releases/tag/v1.26.11-1
gw7_tag=v1.26.11-1
gw7_ver=1.26.11-1
case "$(uname -s)-$(uname -m)" in
  Darwin-arm64) gw7_os=darwin_arm64; gw7_sha=60996604cc7fe8c89f1ee218050def2c9bdf562ca860113efa6deb090394f463 ;;
  Darwin-x86_64) gw7_os=darwin_amd64; gw7_sha=315e82d5b77b4bb72dcd8d5c44fd8e20c7a6e73492a68fe09e80c0447df988f1 ;;
  Linux-x86_64) gw7_os=linux_amd64; gw7_sha=b3aaa2206a79a62d0552485d60240e512555cec701edd4fc0b5ff45644e0c21f ;;
  Linux-aarch64) gw7_os=linux_arm64; gw7_sha=88430c039eef696adb20111076c04bbb51286b93147b281e4d3073d87727bffe ;;
  *) echo "No pinned go-legacy-win7 toolchain for $(uname -s)-$(uname -m)" >&2; exit 1 ;;
esac

version=$(tr -d '[:space:]' < VERSION)
dest=${PLAYS_DIST:-dist}/win7
if [ "${ALLOW_DIRTY:-0}" != 1 ] && [ -n "$(git status --porcelain)" ]; then
  echo "Commit the source tree before packaging (or set ALLOW_DIRTY=1)." >&2; exit 1
fi

tool=.runtime/go-legacy-win7-$gw7_ver-$gw7_os
if [ ! -x "$tool/go-legacy-win7/bin/go" ]; then
  mkdir -p .runtime/downloads "$tool"
  archive=.runtime/downloads/go-legacy-win7-$gw7_ver.$gw7_os.tar.gz
  curl -fsSL -o "$archive" "https://github.com/thongtech/go-legacy-win7/releases/download/$gw7_tag/go-legacy-win7-$gw7_ver.$gw7_os.tar.gz"
  echo "$gw7_sha  $archive" | shasum -a 256 -c -
  tar -xzf "$archive" -C "$tool"
fi
go7=$(cd "$tool/go-legacy-win7/bin" && pwd)/go
export GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=windows
"$go7" version

# The shared code is tested with the normal toolchain by package-mac.sh / CI; here
# only check that the Windows 7 build compiles and vets.
rm -rf "$dest"
mkdir -p "$dest"
for arch in amd64 386; do
  label=x64; [ "$arch" = 386 ] && label=x86
  name="AI-Passport-Plays-$version-Windows7-$label"
  pkg="$dest/$name"
  mkdir -p "$pkg/drivers" "$pkg/ThirdParty/esp-flasher-stub"
  GOARCH=$arch "$go7" vet -tags win7 . ./internal/...
  GOARCH=$arch "$go7" build -tags win7 -trimpath -ldflags "-s -w -X main.version=$version" -o "$pkg/AIPassportPlays.exe" .
  cp internal/driver/ai-passport-usb-serial.inf "$pkg/drivers/"
  cp drivers/windows7/AI-Passport-Win7-JTAG-driver-Espressif-2021-07-15.zip "$pkg/drivers/"
  cp THIRD_PARTY.md "$pkg/ThirdParty/"
  cp internal/esp/stub/LICENSE-APACHE internal/esp/stub/LICENSE-MIT "$pkg/ThirdParty/esp-flasher-stub/"
  cp docs/WINDOWS-7.md "$pkg/"
  git rev-parse HEAD > "$pkg/source-commit.txt" 2>/dev/null || true
  sed 's/$/\r/' > "$pkg/START-HERE.txt" <<TXT
AI Passport 玩法社区 ${version} / Windows 7、8、8.1 版（${label}）

1. 先安装 Google Chrome 109（Windows 7 能装的最后一个版本）。在下载本程序的 GitHub Release 页面
   下载 Chrome-109.0.5414.120-Windows-x64-offline-installer.exe（32 位系统用 x86 的），或用 Google 官方地址：
   64 位：https://dl.google.com/release2/chrome/czao2hrvpk5wgqrkz4kks5r734_109.0.5414.120/109.0.5414.120_chrome_installer.exe
   32 位：https://dl.google.com/release2/chrome/acihtkcueyye3ymoj2afvv7ulzxa_109.0.5414.120/109.0.5414.120_chrome_installer.exe
   安装包的数字签名应为 Google LLC；SHA-256 校验值见 WINDOWS-7.md。不要从第三方网站下载。
2. 把整个文件夹解压到电脑上（不要在压缩包里直接运行），双击 AIPassportPlays.exe。
3. 会出现一个黑色窗口，并自动用 Chrome 打开界面。使用期间不要关闭黑色窗口；关掉它就退出程序。刷机时千万不要关。
4. 插上 AI Passport 后，如果界面提示"没有装驱动"，点"安装驱动"，在弹出的窗口点"是"；
   如果提示"无法验证此驱动程序软件的发布者"，选"始终安装此驱动程序软件"。装好后重新插拔设备。
   自动安装失败时，按 WINDOWS-7.md 第 3 节用 Zadig 手动安装。

32 位 Windows 用 x86 版，64 位用 x64 版（"计算机"右键 → 属性 → 系统类型）。
本版本没有代码签名，杀毒软件可能误报。还没有在 Windows 7 真机上验收过。

Windows 7/8/8.1 edition. Install Chrome 109 (installer on the Release page, or the official links above), extract the folder, run AIPassportPlays.exe and keep
the console window open. If the app says the serial driver is missing, click "Install driver".
TXT
  (cd "$dest" && rm -f "$name.zip" && zip -qr "$name.zip" "$name" && shasum -a 256 "$name.zip" > "$name.zip.sha256")
done
printf 'Packaged Windows 7 ZIPs in %s\n' "$dest"
ls -l "$dest"/*.zip
