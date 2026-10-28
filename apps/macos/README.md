# Copilot Bridge for macOS

A native SwiftUI desktop app around the existing Native AOT bridge. Supports
Apple Silicon and macOS 14 or later; no .NET runtime is required by the built app.

## Features

- Source × harness selection: saved Azure OpenAI sources plus built-in Copilot;
  Codex and Claude Code independently bind to a source and model/deployment.
- GitHub device-code sign-in, cancellation, account refresh, and sign-out.
- Azure OpenAI v1 Responses with API keys in macOS Keychain or short-lived Entra
  tokens from the installed Azure CLI. Deployment names are preserved verbatim.
- Global Codex / Claude Code configuration with a readable preview, change
  detection, atomic writes, and individual private backups.
- Default model selection from the intersection of live Copilot availability
  and this bridge's known native model profiles.
- Two usage levels: each source card shows its local daily totals and opens its
  own report; the Usage page aggregates all sources or filters one source.
  Reports include request counts, input/output/cache tokens, source/model breakdown
  and failures for today, seven days, or thirty days.
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
`artifacts/macos/Copilot-Bridge-macOS-arm64.zip` and a drag-to-Applications DMG.

The resulting app is a local development build, **not Developer ID signed or
Apple notarized**. Distribution beyond local testing needs the owner's Apple
signing identity and notarization setup. Copy the app to `~/Applications` for a
stable Dock / Spotlight entry.

## First use

1. Open **Sources**. For Copilot, manage the GitHub sign-in under **Account**.
   For Azure OpenAI, add a source with a resource endpoint, deployment names,
   and either an API key or an Azure CLI identity (optionally scoped to a
   subscription UUID). Azure-only operation does not require GitHub login.
2. Choose proxy settings under **Network** if needed, then start Bridge.
3. Open **Harness**, select a source and default model for each client, and
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

The existing credential service handles Copilot encryption, OAuth, and refresh.
The UI never receives Copilot bearer tokens. Azure source metadata is saved in
`~/Library/Application Support/Copilot Bridge/sources.json`. API keys are stored
in the login Keychain (`com.lando.copilot-bridge.azure`) and read only into the
owned bridge child's stdin pipe, never argv, UserDefaults or client config.
Azure CLI mode invokes `az account get-access-token` and caches the lease until
shortly before expiration. This requires data-plane Responses permissions;
being able to list a deployment does not imply inference access. Authentication
errors do not trigger resource creation, role assignment or fallback to another source.

Source changes require stopping the bridge first. A source cannot be deleted
while a configured harness uses it. Two processes are persistent (app and bridge);
CLI helpers, including Azure token acquisition, are temporary. Desktop `appsettings.json` disables interactive
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
python3 scripts/test-source-harness.py

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
`status`, `account`, `models`, `login`, `logout`, `configure`. `models` and
`configure` accept `--source <id>` (default `copilot`). A source-specific client
URL is `/sources/<id>/codex` or `/sources/<id>/cc`. Login first emits a
`device_code` event, then a final `result`. Errors return `ok: false` and a nonzero
exit code. Configuration previews contain only changed-key descriptions, path,
and a fingerprint; applying requires the matching `--fingerprint` and `--apply`.
Full merged configuration content is never sent to the UI.

`COPILOT_BRIDGE_DESKTOP=1` enables summary telemetry and a structured `ready`
event after Kestrel has started. `COPILOT_BRIDGE_PROXY_MODE` accepts `system`,
`direct`, or `custom`; custom uses `COPILOT_BRIDGE_PROXY_URL`. The legacy CLI
behaves as before when these environment variables are absent.


## Azure compatibility and limits

Native Codex → Azure forwards Responses JSON and SSE without Copilot model
coercions. Claude Code → Azure uses the existing Anthropic ↔ Responses
translators, with Copilot profiles disabled. Both require a deployment that
supports Responses and the client's tools. The source picker lists configured
deployments; it does not claim that every deployment supports every client feature.
Azure `count_tokens` is a conservative local admission estimate, labeled with
`X-Bridge-Token-Count: local-estimate` and excluded from generation totals.
Azure billing and subscription quotas are not provided by this endpoint; the
Copilot quota panel always refers only to the GitHub account.

Live Azure/harness compatibility is **UNVERIFIED** for this build: the selected
subscription identity can enumerate deployments but the attempted Responses
calls returned `401 PermissionDenied` (missing `responses/write`), and resource
key retrieval returned `AuthorizationFailed`. Offline source-routing, native wire
fidelity, translation, configuration and isolation checks pass; they do not
substitute for the real-client execution gate in `AGENTS.md`. Re-run real Codex
and Claude Code tool tasks after configuring a source with inference access.


## Distribution

Distribute one `.dmg` or `.zip`, not a separate bridge deployment. The `.app`
contains the native SwiftUI executable, the Native AOT bridge, and its updater.
Recipients do not install .NET or Docker. API-key Azure sources have no Azure CLI
dependency; only the Azure CLI authentication option requires `az` and a login.
The supported distribution is currently Apple Silicon, macOS 14+.

For broader distribution, sign the nested executables and app with an Apple
Developer ID Application identity and hardened runtime, notarize with
`xcrun notarytool submit --wait` using a local keychain credential profile, then
staple and validate the notarization ticket. Rebuild the DMG/ZIP from the signed,
stapled app and publish those artifacts in the owner's GitHub Releases. The
build script currently uses ad-hoc signing and does not claim notarization.
Private GitHub releases require recipients to have access to that repository.

Only bundle resources are packaged. Application Support data, GitHub credentials,
Azure API keys and client configurations are never copied into distribution
artifacts. The app creates its own per-user state at first launch.
