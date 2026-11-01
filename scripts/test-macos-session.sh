#!/bin/bash
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
resources="${1:?Pass app Contents/Resources}"
cd "$repo_dir"
swift build --package-path apps/macos --target BridgeCore
build_dir="$(swift build --package-path apps/macos --show-bin-path)"
mkdir -p artifacts
swiftc -I "$repo_dir/apps/macos/Sources/CSQLite" -I "$build_dir/Modules" "$build_dir"/BridgeCore.build/*.swift.o \
 apps/macos/Sources/BridgeApp/Runtime.swift apps/macos/Sources/BridgeApp/SourceKeychain.swift \
 apps/macos/Sources/BridgeApp/AppStore.swift apps/macos/Sources/BridgeApp/AccountsStore.swift \
 apps/macos/Sources/BridgeApp/ModelCatalogStore.swift apps/macos/Sources/BridgeApp/SourcePoolStore.swift apps/macos/Sources/BridgeApp/ProxyStore.swift \
 scripts/macos-session-check.swift -o artifacts/macos-session-check
artifacts/macos-session-check "$resources"
