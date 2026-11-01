#!/bin/bash
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
output_dir="${1:-$repo_dir/artifacts/pacaroute-release}"
architecture="${2:-split}"
if [[ "$architecture" == split ]]; then
    bash "$repo_dir/scripts/build-macos-app.sh" "$output_dir/arm64" arm64
    bash "$repo_dir/scripts/build-macos-app.sh" "$output_dir/x64" x64
    exit 0
fi
case "$architecture" in
    arm64) swift_arch=arm64; go_arch=arm64 ;;
    x64|x86_64|amd64) architecture=x64; swift_arch=x86_64; go_arch=amd64 ;;
    *) echo 'Choose split, arm64, or x64.' >&2; exit 1 ;;
esac
[[ "$(uname -s)" == Darwin ]] || { echo 'Build the macOS UI on macOS.' >&2; exit 1; }
go_bin="${PACAROUTE_GO:-$(command -v go || true)}"
[[ -n "$go_bin" ]] || { echo 'Install Go 1.26 or set PACAROUTE_GO.' >&2; exit 1; }
mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd)"
app_dir="$output_dir/PacaRoute.app"
mkdir -p "$app_dir/Contents/MacOS" "$app_dir/Contents/Resources/bridge"
cd "$repo_dir"
CGO_ENABLED=0 GOOS=darwin GOARCH="$go_arch" "$go_bin" build -trimpath -ldflags='-s -w' -o "$app_dir/Contents/Resources/bridge/pacaroute" ./cmd/pacaroute
swift build --package-path apps/macos -c release --arch "$swift_arch"
swift_bin_dir="$(swift build --package-path apps/macos -c release --arch "$swift_arch" --show-bin-path)"
cp "$swift_bin_dir/PacaRouteMac" "$app_dir/Contents/MacOS/PacaRouteMac"
for binary in "$app_dir/Contents/MacOS/PacaRouteMac" "$app_dir/Contents/Resources/bridge/pacaroute"; do
    lipo "$binary" -verify_arch "$swift_arch"
done
python3 - "$app_dir" <<'PY'
import pathlib,plistlib,sys
app=pathlib.Path(sys.argv[1])
info={'CFBundleName':'PacaRoute','CFBundleDisplayName':'PacaRoute',
'CFBundleIdentifier':'com.landowang.PacaRoute','CFBundleExecutable':'PacaRouteMac',
'CFBundlePackageType':'APPL','CFBundleShortVersionString':'0.6.0','CFBundleVersion':'17',
'LSMinimumSystemVersion':'14.0','LSApplicationCategoryType':'public.app-category.developer-tools',
'NSHighResolutionCapable':True,'NSPrincipalClass':'NSApplication','CFBundleIconFile':'AppIcon',
'NSAppTransportSecurity':{'NSAllowsLocalNetworking':True}}
with (app/'Contents/Info.plist').open('wb') as f:plistlib.dump(info,f)
PY
swiftc "$repo_dir/apps/macos/Sources/BridgeApp/AgentLinkMark.swift" "$repo_dir/scripts/macos-icon.swift" -o "$output_dir/icon-builder"
"$output_dir/icon-builder" "$output_dir"
iconutil -c icns "$output_dir/AppIcon.iconset" -o "$app_dir/Contents/Resources/AppIcon.icns"
cp "$repo_dir/THIRD_PARTY_NOTICES.md" "$app_dir/Contents/Resources/THIRD_PARTY_NOTICES.md"
codesign --force --sign - "$app_dir/Contents/Resources/bridge/pacaroute"
codesign --force --sign - "$app_dir"
codesign --verify --deep --strict "$app_dir"
ditto -c -k --sequesterRsrc --keepParent "$app_dir" "$output_dir/PacaRoute-macOS-$architecture.zip"
bash "$repo_dir/scripts/package-macos-dmg.sh" "$app_dir" "$output_dir/PacaRoute-macOS-$architecture.dmg"
printf 'Built: %s\n' "$app_dir"
