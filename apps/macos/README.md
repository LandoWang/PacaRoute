# Copilot Bridge for macOS

A native SwiftUI desktop app around the existing Native AOT bridge. Supports
Apple Silicon and macOS 14 or later; no .NET runtime is required by the built app.

## Features

- GitHub device-code sign-in, cancellation, account refresh, and sign-out.
- Global Codex / Claude Code configuration with a readable preview, change
  detection, atomic writes, and individual private backups.
- Default model selection from the intersection of live Copilot availability
  and this bridge's known native model profiles.
- Local request counts, input/output/cache tokens, model breakdown, and failures
  for today, seven days, or thirty days.
- Copilot account quota, remaining percentage, overages, and reset date when
  GitHub provides them. Missing quota is unknown, never an invented zero.
- Sanitized request/error summaries and access to the original bridge logs.
- System/environment proxy, direct connection, or a custom HTTP/HTTPS/SOCKS5
  proxy. Network settings are process-local and never change macOS proxy settings.
- Start/stop controls, port conflict detection, and a menu bar item. Closing the
  window keeps the app running; quitting stops only its own child processes.

## Build and open

Requires Xcode Command Line Tools (including Swift) and the .NET SDK selected by
the repository's `global.json`. A full Xcode installation is not needed.

```bash
scripts/build-macos-app.sh
open 'artifacts/macos/Copilot Bridge.app'
```

Set `BRIDGE_DOTNET` to the SDK executable if it is not on PATH. The script also
checks `~/.dotnet/dotnet`. It publishes both bridge executables, builds the Swift
UI, generates an icon, verifies ad-hoc code signatures, and produces
`artifacts/macos/Copilot-Bridge-macOS-arm64.zip`.

The resulting app is a local development build, **not Developer ID signed or
Apple notarized**. Distribution beyond local testing needs the owner's Apple
signing identity and notarization setup. Copy the app to `~/Applications` for a
stable Dock / Spotlight entry.

## First use

1. Open **Account**, choose GitHub sign-in, and complete device authorization in
   the browser. The app displays and can copy the verification code.
2. Choose proxy settings under **Network** if needed, then start Bridge.
3. Open **Clients & Models**, refresh available models, select a default, and
   inspect the preview before applying it.
4. Start a new Codex / Claude Code session. Some client configuration layers or
   explicit session options can override the global model.
5. Inspect local usage and explicitly refresh GitHub quota under **Usage**.

The app manages `~/.codex/config.toml` (or an inherited `CODEX_HOME`) and
`~/.claude/settings.json`, reusing the bridge's existing merge code. It preserves
unrelated settings, providers, permissions, and comments in untouched TOML
regions. A timestamped `.bridge-*.bak` sibling contains the previous bytes. To
restore manually, stop any client editing that file and copy the desired backup
over its original path. No configuration is applied merely by launching the app.

See the [official Codex provider configuration reference](https://learn.chatgpt.com/docs/config-file/config-advanced#custom-model-providers)
for provider / command-auth semantics and global configuration precedence.

## Runtime and data

The app copies its bundled engine into
`~/Library/Application Support/Copilot Bridge/runtime/`. This gives client
command-auth a stable executable path and keeps credentials, logs, and usage
outside the signed application bundle. This directory is separate from any
existing CLI installation; credentials are not silently imported.

The existing credential service handles encryption, OAuth, and refresh. The UI
never receives bearer tokens. Desktop `appsettings.json` disables interactive
self-update and request-body tracing. Engine upgrades come with app rebuilds;
both packaged executables stay together. The main window has no HTTP admin API.

Daily `desktop-activity/*.jsonl` files contain completed request metadata and
sanitized error templates only. The UI reads the most recent 31 log-day files;
files are retained on disk until manually removed. `count_tokens` estimates do
not inflate generation totals. Missing usage is called out, and cache values
are shown separately because the two upstream protocols count input differently.
Local token totals are **not a billing estimate** and do not include other
bridge installations or other Copilot clients.

GitHub quota uses the existing reference implementation's
`GET /copilot_internal/user` surface through the authentication facade. It is an
internal GitHub endpoint: an unsupported account, denied scope, or unavailable
field is surfaced as unavailable with a link to GitHub. Live authentication,
quota and model access ultimately depend on the user's account and network.

## Verification

```bash
dotnet test tests/CopilotBridge.UnitTests
swift run --package-path apps/macos BridgeCoreChecks
python3 scripts/test-macos-control.py

# Native pipe streaming / cancellation contract, using only local child processes:
swift build --package-path apps/macos --target BridgeCore
swiftc -I apps/macos/.build/arm64-apple-macosx/debug/Modules \
  apps/macos/.build/arm64-apple-macosx/debug/BridgeCore.build/Models.swift.o \
  apps/macos/Sources/BridgeApp/Runtime.swift scripts/macos-process-check.swift \
  -o artifacts/macos-process-check
artifacts/macos-process-check
```

Swift checks are a standalone executable so they work on Command Line Tools
installations without XCTest. The Python check copies the Debug engine into a
temporary directory, uses local upstream fixtures and the repository's existing
Debug-only isolation hooks, and exercises the real CLI and HTTP server. It never
authenticates to GitHub, reads real credentials, or applies real client settings.
These are control/transport checks, not a claim that a real paid Copilot model
or a headless client has completed a live coding task.

## Desktop protocol

`copilot-bridge desktop <action>` emits one JSON object per line. Actions:
`status`, `account`, `models`, `login`, `logout`, `configure`. Login first emits a
`device_code` event, then a final `result`. Errors return `ok: false` and a nonzero
exit code. Configuration previews contain only changed-key descriptions, path,
and a fingerprint; applying requires the matching `--fingerprint` and `--apply`.
Full merged configuration content is never sent to the UI.

`COPILOT_BRIDGE_DESKTOP=1` enables summary telemetry and a structured `ready`
event after Kestrel has started. `COPILOT_BRIDGE_PROXY_MODE` accepts `system`,
`direct`, or `custom`; custom uses `COPILOT_BRIDGE_PROXY_URL`. The legacy CLI
behaves as before when these environment variables are absent.
