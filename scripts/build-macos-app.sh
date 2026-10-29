#!/bin/bash
set -euo pipefail

repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
output_dir="${1:-$repo_dir/artifacts/macos}"
architecture="${2:-${BRIDGE_MACOS_ARCH:-split}}"
case "$architecture" in
    split)
        bash "$repo_dir/scripts/build-macos-app.sh" "$output_dir/arm64" arm64
        bash "$repo_dir/scripts/build-macos-app.sh" "$output_dir/x64" x64
        exit 0
        ;;
    universal) architectures=(arm64 x64); expected_arches=(arm64 x86_64) ;;
    arm64) architectures=(arm64); expected_arches=(arm64) ;;
    x64|x86_64) architecture=x64; architectures=(x64); expected_arches=(x86_64) ;;
    *) echo 'Architecture must be split, universal, arm64, or x64.' >&2; exit 1 ;;
esac
dotnet_bin="${BRIDGE_DOTNET:-$(command -v dotnet || true)}"
if [[ -z "$dotnet_bin" && -x "$HOME/.dotnet/dotnet" ]]; then dotnet_bin="$HOME/.dotnet/dotnet"; fi
if [[ -z "$dotnet_bin" ]]; then echo 'Install the .NET 10 SDK or set BRIDGE_DOTNET.' >&2; exit 1; fi
if [[ "$(uname -s)" != Darwin ]]; then echo 'Build this app on macOS.' >&2; exit 1; fi
case "$(uname -m)" in
    arm64|x86_64) ;;
    *) echo 'Build this app on an Apple Silicon or Intel Mac.' >&2; exit 1 ;;
esac

app_dir="$output_dir/AgentLink.app"
mkdir -p "$output_dir" "$app_dir/Contents/MacOS" "$app_dir/Contents/Resources/bridge"
output_dir="$(cd "$output_dir" && pwd)"
app_dir="$output_dir/AgentLink.app"
cd "$repo_dir"
ui_binaries=(); bridge_binaries=(); updater_binaries=()
for arch in "${architectures[@]}"; do
    swift_arch="$arch"
    if [[ "$arch" == x64 ]]; then swift_arch=x86_64; fi
    engine_dir="$repo_dir/artifacts/macos-engine-$arch"
    mkdir -p "$engine_dir"
    "$dotnet_bin" publish src/CopilotBridge.Cli -c Release -r "osx-$arch" -o "$engine_dir" -p:Version=0.5.28-macos.8
    "$dotnet_bin" publish src/CopilotBridge.Updater -c Release -r "osx-$arch" -o "$engine_dir" -p:Version=0.5.28-macos.8
    swift build --package-path apps/macos -c release --arch "$swift_arch"
    swift_bin_dir="$(swift build --package-path apps/macos -c release --arch "$swift_arch" --show-bin-path)"
    ui_binaries+=("$swift_bin_dir/CopilotBridgeMac")
    bridge_binaries+=("$engine_dir/copilot-bridge")
    updater_binaries+=("$engine_dir/copilot-updater")
done
if [[ "$architecture" == universal ]]; then
    lipo -create "${ui_binaries[@]}" -output "$app_dir/Contents/MacOS/CopilotBridgeMac"
    lipo -create "${bridge_binaries[@]}" -output "$app_dir/Contents/Resources/bridge/copilot-bridge"
    lipo -create "${updater_binaries[@]}" -output "$app_dir/Contents/Resources/bridge/copilot-updater"
else
    cp "${ui_binaries[0]}" "$app_dir/Contents/MacOS/CopilotBridgeMac"
    cp "${bridge_binaries[0]}" "$app_dir/Contents/Resources/bridge/copilot-bridge"
    cp "${updater_binaries[0]}" "$app_dir/Contents/Resources/bridge/copilot-updater"
fi
for binary in "$app_dir/Contents/MacOS/CopilotBridgeMac" "$app_dir/Contents/Resources/bridge/copilot-bridge" "$app_dir/Contents/Resources/bridge/copilot-updater"; do
    lipo "$binary" -verify_arch "${expected_arches[@]}"
done
python3 - "$engine_dir/appsettings.json" "$app_dir" <<'PY'
import json, plistlib, pathlib, sys
source, app = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
settings = json.loads(source.read_text())
settings['AutoUpdate']['EnableAutoUpdate'] = False
settings['Tracing']['Enabled'] = False
settings['Logging']['LogLevel']['CopilotBridge.Cli'] = 'Information'
(app / 'Contents/Resources/bridge/appsettings.json').write_text(json.dumps(settings, indent=2) + '\n')
info = {
    'CFBundleName': 'AgentLink', 'CFBundleDisplayName': 'AgentLink',
    'CFBundleIdentifier': 'com.landowang.CopilotBridge', 'CFBundleExecutable': 'CopilotBridgeMac',
    'CFBundlePackageType': 'APPL', 'CFBundleShortVersionString': '0.5.28', 'CFBundleVersion': '13',
    'LSMinimumSystemVersion': '14.0', 'LSApplicationCategoryType': 'public.app-category.developer-tools',
    'NSHighResolutionCapable': True, 'NSPrincipalClass': 'NSApplication',
    'CFBundleIconFile': 'AppIcon',
}
with (app / 'Contents/Info.plist').open('wb') as f: plistlib.dump(info, f)
PY
swiftc "$repo_dir/apps/macos/Sources/BridgeApp/AgentLinkMark.swift" "$repo_dir/scripts/macos-icon.swift" -o "$output_dir/icon-builder"
"$output_dir/icon-builder" "$output_dir"
iconutil -c icns "$output_dir/AppIcon.iconset" -o "$app_dir/Contents/Resources/AppIcon.icns"
codesign --force --sign - "$app_dir/Contents/Resources/bridge/copilot-bridge"
codesign --force --sign - "$app_dir/Contents/Resources/bridge/copilot-updater"
codesign --force --sign - "$app_dir"
codesign --verify --deep --strict "$app_dir"
ditto -c -k --sequesterRsrc --keepParent "$app_dir" "$output_dir/AgentLink-macOS-$architecture.zip"
bash "$repo_dir/scripts/package-macos-dmg.sh" "$app_dir" "$output_dir/AgentLink-macOS-$architecture.dmg"
printf 'Built: %s\n' "$app_dir"
