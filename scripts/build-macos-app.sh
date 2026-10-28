#!/bin/bash
set -euo pipefail

repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
output_dir="${1:-$repo_dir/artifacts/macos}"
dotnet_bin="${BRIDGE_DOTNET:-$(command -v dotnet || true)}"
if [[ -z "$dotnet_bin" && -x "$HOME/.dotnet/dotnet" ]]; then dotnet_bin="$HOME/.dotnet/dotnet"; fi
if [[ -z "$dotnet_bin" ]]; then echo 'Install the .NET 10 SDK or set BRIDGE_DOTNET.' >&2; exit 1; fi
if [[ "$(uname -s)" != Darwin ]]; then echo 'Build this app on macOS.' >&2; exit 1; fi
if [[ "$(uname -m)" != arm64 ]]; then echo 'This build currently supports Apple Silicon.' >&2; exit 1; fi

app_dir="$output_dir/Copilot Bridge.app"
engine_dir="$repo_dir/artifacts/macos-engine"
mkdir -p "$engine_dir" "$app_dir/Contents/MacOS" "$app_dir/Contents/Resources/bridge"
cd "$repo_dir"
"$dotnet_bin" publish src/CopilotBridge.Cli -c Release -r osx-arm64 -o "$engine_dir" -p:Version=0.5.28-macos.4
"$dotnet_bin" publish src/CopilotBridge.Updater -c Release -r osx-arm64 -o "$engine_dir" -p:Version=0.5.28-macos.4
swift build --package-path apps/macos -c release
swift_bin_dir="$(swift build --package-path apps/macos -c release --show-bin-path)"
cp "$swift_bin_dir/CopilotBridgeMac" "$app_dir/Contents/MacOS/CopilotBridgeMac"
cp "$engine_dir/copilot-bridge" "$engine_dir/copilot-updater" "$app_dir/Contents/Resources/bridge/"
python3 - "$engine_dir/appsettings.json" "$app_dir" <<'PY'
import json, plistlib, pathlib, sys
source, app = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
settings = json.loads(source.read_text())
settings['AutoUpdate']['EnableAutoUpdate'] = False
settings['Tracing']['Enabled'] = False
settings['Logging']['LogLevel']['CopilotBridge.Cli'] = 'Information'
(app / 'Contents/Resources/bridge/appsettings.json').write_text(json.dumps(settings, indent=2) + '\n')
info = {
    'CFBundleName': 'Copilot Bridge', 'CFBundleDisplayName': 'Copilot Bridge',
    'CFBundleIdentifier': 'com.landowang.CopilotBridge', 'CFBundleExecutable': 'CopilotBridgeMac',
    'CFBundlePackageType': 'APPL', 'CFBundleShortVersionString': '0.5.28', 'CFBundleVersion': '4',
    'LSMinimumSystemVersion': '14.0', 'LSApplicationCategoryType': 'public.app-category.developer-tools',
    'NSHighResolutionCapable': True, 'NSPrincipalClass': 'NSApplication',
    'CFBundleIconFile': 'AppIcon',
}
with (app / 'Contents/Info.plist').open('wb') as f: plistlib.dump(info, f)
PY
swift "$repo_dir/scripts/macos-icon.swift" "$output_dir"
iconutil -c icns "$output_dir/AppIcon.iconset" -o "$app_dir/Contents/Resources/AppIcon.icns"
codesign --force --sign - "$app_dir/Contents/Resources/bridge/copilot-bridge"
codesign --force --sign - "$app_dir/Contents/Resources/bridge/copilot-updater"
codesign --force --sign - "$app_dir"
codesign --verify --deep --strict "$app_dir"
ditto -c -k --sequesterRsrc --keepParent "$app_dir" "$output_dir/Copilot-Bridge-macOS-arm64.zip"
bash "$repo_dir/scripts/package-macos-dmg.sh" "$app_dir" "$output_dir/Copilot-Bridge-macOS-arm64.dmg"
printf 'Built: %s\n' "$app_dir"
