#!/bin/bash
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
output_dir="${1:-$repo_dir/artifacts/pacaroute-cli}"
mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd)"
cd "$repo_dir"
for target in darwin-arm64 darwin-amd64 linux-arm64 linux-amd64; do
    target_os="${target%-*}"; target_arch="${target#*-}"
    mkdir -p "$output_dir/$target"
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags='-s -w' -o "$output_dir/$target/pacaroute" ./cmd/pacaroute
    cp THIRD_PARTY_NOTICES.md "$output_dir/$target/THIRD_PARTY_NOTICES.md"
    cp docs/pacaroute.md "$output_dir/$target/README.md"
    tar -czf "$output_dir/PacaRoute-$target.tar.gz" -C "$output_dir/$target" pacaroute THIRD_PARTY_NOTICES.md README.md
done
