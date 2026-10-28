#!/bin/bash
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
resources="${1:?Pass app Contents/Resources}"
cd "$repo_dir"
swift build --package-path apps/macos --target BridgeCore
build_dir="$(swift build --package-path apps/macos --show-bin-path)"
mkdir -p artifacts
swiftc -I "$build_dir/Modules" "$build_dir"/BridgeCore.build/*.swift.o \
 apps/macos/Sources/BridgeApp/Runtime.swift apps/macos/Sources/BridgeApp/SourceKeychain.swift \
 apps/macos/Sources/BridgeApp/AppStore.swift apps/macos/Sources/BridgeApp/AccountsStore.swift \
 apps/macos/Sources/BridgeApp/ProxyStore.swift apps/macos/Sources/BridgeApp/StatusMenuController.swift \
 apps/macos/Sources/BridgeApp/AgentLinkMark.swift \
 scripts/macos-menu-check.swift -o artifacts/macos-menu-check
artifacts/macos-menu-check "$resources"
