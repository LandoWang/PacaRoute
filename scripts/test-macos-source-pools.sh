#!/bin/bash
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_dir"
swift build --package-path apps/macos --target BridgeCore
build_dir="$(swift build --package-path apps/macos --show-bin-path)"
mkdir -p artifacts/source-pool-verification
swiftc -I "$repo_dir/apps/macos/Sources/CSQLite" -I "$build_dir/Modules" "$build_dir"/BridgeCore.build/*.swift.o \
 apps/macos/Sources/BridgeApp/Runtime.swift apps/macos/Sources/BridgeApp/SourceKeychain.swift \
 apps/macos/Sources/BridgeApp/AppStore.swift apps/macos/Sources/BridgeApp/AccountsStore.swift \
 apps/macos/Sources/BridgeApp/ModelCatalogStore.swift apps/macos/Sources/BridgeApp/ProxyStore.swift \
 apps/macos/Sources/BridgeApp/SourcePoolStore.swift scripts/macos-source-pool-check.swift \
 -o artifacts/source-pool-verification/macos-pool-check
artifacts/source-pool-verification/macos-pool-check
