#!/bin/bash
# Build "AI Passport 玩法社区.app" for Apple Silicon (arm64), Intel (x86_64) and a
# Universal app holding both, each as its own DMG. Runs on either kind of Mac: the
# other architecture is cross-compiled with clang -arch.
# Env: PLAYS_DIST (output dir), ALLOW_DIRTY=1 (skip clean-tree check).
set -euo pipefail
cd "$(dirname "$0")/.."
if [ "$(uname -s)" != Darwin ]; then echo 'Run this packager on macOS.' >&2; exit 1; fi
export GOTOOLCHAIN=go1.26.0
export MACOSX_DEPLOYMENT_TARGET=12.0

version=$(tr -d '[:space:]' < VERSION)
app_name='AI Passport 玩法社区'
exe=ai-passport-plays
dest=${PLAYS_DIST:-dist}
mkdir -p "$dest"
dest=$(cd "$dest" && pwd)

if [ "${ALLOW_DIRTY:-0}" != 1 ] && [ -n "$(git status --porcelain)" ]; then
  echo "Commit the source tree before packaging (or set ALLOW_DIRTY=1)." >&2; exit 1
fi

go test ./...

work=$(mktemp -d "${TMPDIR:-/tmp}/plays-mac.XXXXXX")
trap 'rm -rf "$work"' EXIT

# build <goarch> <clang arch> <output>
build() {
  CGO_ENABLED=1 GOARCH=$1 \
    CC="clang -arch $2" \
    CGO_CFLAGS="-arch $2 -mmacosx-version-min=12.0" \
    CGO_LDFLAGS="-arch $2 -mmacosx-version-min=12.0" \
    go build -tags production -trimpath -ldflags "-s -w -X main.version=$version" -o "$3" .
}
build arm64 arm64 "$work/$exe-arm64"
build amd64 x86_64 "$work/$exe-x86_64"
lipo -create -output "$work/$exe-universal" "$work/$exe-arm64" "$work/$exe-x86_64"
lipo -info "$work/$exe-universal"

# package <label> : app bundle + DMG for one binary
package() {
  local label=$1
  local app="$dest/$label/$app_name.app"
  local dmg="$dest/AI-Passport-Plays-$version-macOS-$label.dmg"
  rm -rf "$dest/$label"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources/ThirdParty/esp-flasher-stub"
  cp "$work/$exe-$label" "$app/Contents/MacOS/$exe"
  cp assets/AppIcon.icns "$app/Contents/Resources/AppIcon.icns"
  cp THIRD_PARTY.md "$app/Contents/Resources/ThirdParty/"
  cp internal/esp/stub/LICENSE-APACHE internal/esp/stub/LICENSE-MIT "$app/Contents/Resources/ThirdParty/esp-flasher-stub/"
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

  local stage="$work/dmg-$label"
  mkdir -p "$stage"
  cp -R "$app" "$stage/"
  ln -s /Applications "$stage/Applications"
  rm -f "$dmg"
  hdiutil create -quiet -volname "$app_name" -srcfolder "$stage" -ov -format UDZO "$dmg"
  (cd "$dest" && shasum -a 256 "$(basename "$dmg")" > "$(basename "$dmg").sha256")
  printf 'Packaged %s\n' "$dmg"
}

# Self-test every architecture this Mac can run (x86_64 needs Rosetta on Apple Silicon).
selftest() {
  local bin="$dest/$1/$app_name.app/Contents/MacOS/$exe"
  if [ "$2" = x86_64 ] && [ "$(uname -m)" = arm64 ] && ! arch -x86_64 /usr/bin/true 2>/dev/null; then
    echo "Skipping x86_64 self-test of $1: Rosetta is not installed."; return
  fi
  arch "-$2" "$bin" -self-test
}

for label in arm64 x86_64 universal; do package "$label"; done
selftest x86_64 x86_64
selftest universal x86_64
if [ "$(uname -m)" = arm64 ]; then
  selftest arm64 arm64
  selftest universal arm64
else
  echo "Skipping arm64 self-test: this Intel Mac cannot run arm64 code."
fi
