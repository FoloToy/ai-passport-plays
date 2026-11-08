#!/bin/bash
# Build "AI Passport 玩法社区.app" and a DMG for the current Mac architecture.
# Env: PLAYS_PYTHON (Python >= 3.12), PLAYS_DIST (output dir), ALLOW_DIRTY=1 (skip clean-tree check).
set -euo pipefail
cd "$(dirname "$0")/.."
if [ "$(uname -s)" != Darwin ]; then echo 'Run this packager on macOS.' >&2; exit 1; fi
export GOTOOLCHAIN=go1.26.0
export MACOSX_DEPLOYMENT_TARGET=12.0
export CGO_CFLAGS="${CGO_CFLAGS:-} -mmacosx-version-min=12.0"
export CGO_LDFLAGS="${CGO_LDFLAGS:-} -mmacosx-version-min=12.0"

version=$(tr -d '[:space:]' < VERSION)
app_name='AI Passport 玩法社区'
exe=ai-passport-plays
worker=ai-passport-plays-worker
arch=$(uname -m)
dest=${PLAYS_DIST:-dist}
app="$dest/$app_name.app"
dmg="$dest/AI-Passport-Plays-$version-macOS-$arch.dmg"

if [ "${ALLOW_DIRTY:-0}" != 1 ] && [ -n "$(git status --porcelain)" ]; then
  echo "Commit the source tree before packaging (or set ALLOW_DIRTY=1)." >&2; exit 1
fi

python_bin=${PLAYS_PYTHON:-python3.12}
"$python_bin" -c 'import sys; assert sys.version_info >= (3, 12), "Packaging requires Python 3.12 or later"'
if [ ! -x .runtime/worker-venv/bin/python ]; then "$python_bin" -m venv .runtime/worker-venv; fi
.runtime/worker-venv/bin/python -m pip install -q -r worker/requirements.lock
.runtime/worker-venv/bin/python -m unittest discover -q -s worker -p 'test_*.py'
.runtime/worker-venv/bin/python -m pip freeze > .runtime/worker-resolved.txt
.runtime/worker-venv/bin/python -m PyInstaller --noconfirm --clean --onedir --console --name "$worker" \
  --distpath .runtime/worker-dist --workpath .runtime/worker-build --specpath .runtime \
  --paths worker --hidden-import restore \
  --collect-all esptool --collect-all esp_pylib worker/main.py

go test ./...

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources/worker" "$app/Contents/Resources/ThirdParty"
go build -tags production -trimpath -ldflags "-s -w -X main.version=$version" -o "$app/Contents/MacOS/$exe" .
cp -R ".runtime/worker-dist/$worker/." "$app/Contents/Resources/worker/"
cp assets/AppIcon.icns "$app/Contents/Resources/AppIcon.icns"
cp .runtime/worker-resolved.txt "$app/Contents/Resources/ThirdParty/python-packages.txt"
cp worker/main.py worker/restore.py worker/requirements.txt THIRD_PARTY.md "$app/Contents/Resources/ThirdParty/"
.runtime/worker-venv/bin/python scripts/collect-licenses.py "$app/Contents/Resources/ThirdParty/licenses"
mkdir -p .runtime/sources
if [ ! -f .runtime/sources/esptool-5.6.0.tar.gz ]; then
  .runtime/worker-venv/bin/python -m pip download -q --no-deps --no-binary=:all: esptool==5.6.0 -d .runtime/sources
fi
cp .runtime/sources/esptool-5.6.0.tar.gz "$app/Contents/Resources/ThirdParty/"

cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>$app_name</string>
<key>CFBundleDisplayName</key><string>$app_name</string>
<key>CFBundleIdentifier</key><string>com.folotoy.ai-passport-plays</string>
<key>CFBundleExecutable</key><string>$exe</string>
<key>CFBundleIconFile</key><string>AppIcon</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>$version</string>
<key>CFBundleVersion</key><string>$version</string>
<key>CFBundleDevelopmentRegion</key><string>zh_CN</string>
<key>LSMinimumSystemVersion</key><string>12.0</string>
<key>LSApplicationCategoryType</key><string>public.app-category.utilities</string>
<key>NSHighResolutionCapable</key><true/>
<key>NSHumanReadableCopyright</key><string>FoloToy</string>
</dict></plist>
PLIST
git rev-parse HEAD > "$app/Contents/Resources/source-commit.txt" 2>/dev/null || true

# No Developer ID yet: ad-hoc sign so the bundle is internally consistent.
codesign --force --deep --sign - "$app"
codesign --verify --deep --strict "$app"
"$app/Contents/Resources/worker/$worker" version
"$app/Contents/Resources/worker/$worker" self-test

stage=$(mktemp -d "${TMPDIR:-/tmp}/plays-dmg.XXXXXX")
trap 'rm -rf "$stage"' EXIT
cp -R "$app" "$stage/"
ln -s /Applications "$stage/Applications"
rm -f "$dmg"
hdiutil create -quiet -volname "$app_name" -srcfolder "$stage" -ov -format UDZO "$dmg"
(cd "$dest" && shasum -a 256 "$(basename "$dmg")" > "$(basename "$dmg").sha256")
printf 'Packaged %s\n' "$dmg"
