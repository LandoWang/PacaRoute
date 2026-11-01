#!/bin/bash
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_dir"
swift build --package-path apps/macos -c release --target BridgeCore
build_dir="$(swift build --package-path apps/macos -c release --show-bin-path)"
mkdir -p artifacts/cpu-investigation
swiftc -parse-as-library -O -I "$repo_dir/apps/macos/Sources/CSQLite" -I "$build_dir/Modules" "$build_dir"/BridgeCore.build/*.swift.o \
 scripts/macos-activity-reader-check.swift -o artifacts/cpu-investigation/activity-reader-check
artifacts/cpu-investigation/activity-reader-check
swiftc -parse-as-library -O -I "$repo_dir/apps/macos/Sources/CSQLite" -I "$build_dir/Modules" "$build_dir"/BridgeCore.build/*.swift.o \
 apps/macos/Sources/BridgeApp/Runtime.swift apps/macos/Sources/BridgeApp/SourceKeychain.swift \
 apps/macos/Sources/BridgeApp/AppStore.swift apps/macos/Sources/BridgeApp/AccountsStore.swift \
 apps/macos/Sources/BridgeApp/ModelCatalogStore.swift apps/macos/Sources/BridgeApp/SourcePoolStore.swift apps/macos/Sources/BridgeApp/ProxyStore.swift \
 scripts/macos-activity-refresh-check.swift -o artifacts/cpu-investigation/activity-refresh-check
artifacts/cpu-investigation/activity-refresh-check
