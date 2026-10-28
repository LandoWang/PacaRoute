#!/bin/bash
set -euo pipefail

app_path="${1:?Usage: package-macos-dmg.sh /path/to/Copilot\ Bridge.app /path/to/output.dmg}"
dmg_path="${2:?Output DMG path is required}"
codesign --verify --deep --strict "$app_path"
stage_dir="$(mktemp -d "${TMPDIR:-/tmp}/copilot-bridge-dmg.XXXXXX")"
trap 'rm -rf "$stage_dir"' EXIT
ditto "$app_path" "$stage_dir/Copilot Bridge.app"
ln -s /Applications "$stage_dir/Applications"
hdiutil create -quiet -volname 'Copilot Bridge' -srcfolder "$stage_dir" -format UDZO -ov "$dmg_path"
hdiutil verify -quiet "$dmg_path"
printf 'DMG: %s\n' "$dmg_path"
