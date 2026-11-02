#!/bin/bash
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
output_dir="${1:-$repo_dir/artifacts/pacaroute-desktop-release}"
targets="${2:-all}"
mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd)"
cd "$repo_dir"
if [[ "${PACAROUTE_WEB_READY:-0}" != 1 ]]; then bash scripts/build-web.sh; fi
if [[ "$targets" == all || "$targets" == macos ]]; then
    [[ "$(uname -s)" == Darwin ]] || { echo 'Build macOS bundles on macOS.' >&2; exit 1; }
    swiftc apps/macos/Sources/BridgeApp/AgentLinkMark.swift scripts/macos-icon.swift -o "$output_dir/icon-builder"
    "$output_dir/icon-builder" "$output_dir"
    iconutil -c icns "$output_dir/AppIcon.iconset" -o "$output_dir/AppIcon.icns"
    for arch in arm64 amd64; do
        target_dir="$output_dir/macos-$arch"
        app_dir="$target_dir/PacaRoute.app"
        mkdir -p "$app_dir/Contents/MacOS" "$app_dir/Contents/Resources"
        CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" MACOSX_DEPLOYMENT_TARGET=14.0 go build -tags desktop -trimpath -ldflags='-s -w' -o "$app_dir/Contents/MacOS/pacaroute" ./cmd/pacaroute
        cp "$output_dir/AppIcon.icns" "$app_dir/Contents/Resources/AppIcon.icns"
        cp THIRD_PARTY_NOTICES.md "$app_dir/Contents/Resources/THIRD_PARTY_NOTICES.md"
        python3 - "$app_dir" <<'PY'
import pathlib,plistlib,sys
app=pathlib.Path(sys.argv[1])
info={'CFBundleName':'PacaRoute','CFBundleDisplayName':'PacaRoute','CFBundleIdentifier':'com.landowang.pacaroute.tray','CFBundleExecutable':'pacaroute','CFBundlePackageType':'APPL','CFBundleShortVersionString':'0.8.1','CFBundleVersion':'20','LSMinimumSystemVersion':'14.0','LSUIElement':True,'NSHighResolutionCapable':True,'NSPrincipalClass':'NSApplication','CFBundleIconFile':'AppIcon','LSApplicationCategoryType':'public.app-category.developer-tools'}
with (app/'Contents/Info.plist').open('wb') as f:plistlib.dump(info,f)
PY
        codesign --force --sign - "$app_dir"
        codesign --verify --deep --strict "$app_dir"
        ditto -c -k --sequesterRsrc --keepParent "$app_dir" "$target_dir/PacaRoute-macOS-$arch.zip"
        bash scripts/package-macos-dmg.sh "$app_dir" "$target_dir/PacaRoute-macOS-$arch.dmg"
    done
fi
if [[ "$targets" == all || "$targets" == windows ]]; then
    target_dir="$output_dir/windows-amd64"
    mkdir -p "$target_dir"
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags desktop -trimpath -ldflags='-s -w -H=windowsgui' -o "$target_dir/PacaRoute.exe" ./cmd/pacaroute
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "$target_dir/pacaroute-cli.exe" ./cmd/pacaroute
    cp THIRD_PARTY_NOTICES.md "$target_dir/THIRD_PARTY_NOTICES.md"
    cp docs/pacaroute-web-desktop.md "$target_dir/README.md"
    python3 - "$target_dir" "$output_dir/PacaRoute-Windows-x64.zip" <<'PY'
import pathlib,sys,zipfile
source=pathlib.Path(sys.argv[1])
with zipfile.ZipFile(sys.argv[2],'w',zipfile.ZIP_DEFLATED) as z:
    for p in source.iterdir():
        if p.is_file():z.write(p,p.name)
PY
fi
printf 'Desktop packages: %s\n' "$output_dir"
