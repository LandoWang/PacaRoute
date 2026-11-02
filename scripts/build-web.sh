#!/bin/bash
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_dir/apps/web"
npm ci --no-fund --no-audit
npm run build
npm run embed
