#!/bin/bash
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_dir"
swift build --package-path apps/macos --target LocalSessions
build_dir="$(swift build --package-path apps/macos --show-bin-path)"
mkdir -p artifacts/local-session-verification
swiftc -I "$build_dir/Modules" -I apps/macos/Sources/CSQLite "$build_dir"/LocalSessions.build/*.swift.o \
  apps/macos/Sources/BridgeApp/SessionsStore.swift scripts/macos-history-analysis-check.swift \
  -o artifacts/local-session-verification/analysis-check
artifacts/local-session-verification/analysis-check "${1:?source ID}" "${2:?model}" "${3:?port}"
