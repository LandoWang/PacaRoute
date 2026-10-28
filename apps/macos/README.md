# AgentLink for macOS

A native SwiftUI desktop app around the existing Native AOT bridge. Supports
Apple Silicon and Intel Macs running macOS 14 or later; no .NET runtime is required.

## Features

- Source × Agent selection: saved Azure OpenAI sources plus built-in Copilot;
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
  and failures for today, seven days, thirty days, or a custom date range.
  Hourly/daily timelines include empty buckets and follow the local time zone.
- Copilot account quota, remaining percentage, overages, and reset date when
  GitHub provides them. Missing quota is unknown, never an invented zero.
- Sanitized request/error summaries and access to the original bridge logs.
- Reusable Proxy profiles (system, direct, HTTP/HTTPS/SOCKS5), independently
  bound to each source. Account-only login/discovery has its own proxy choice.
  Network settings are process-local and never change macOS proxy settings.
- Start/stop controls, port conflict detection, and a native menu bar item with left/right-click start/stop, refresh and today's usage. Closing the
  window keeps the app running; quitting stops only its own child processes.

## Build and open

Requires Xcode Command Line Tools (including Swift) and the .NET SDK selected by
the repository's `global.json`. A full Xcode installation is not needed.

```bash
scripts/build-macos-app.sh
open 'artifacts/macos/arm64/AgentLink.app' # Use x64 instead on Intel.
```

Set `BRIDGE_DOTNET` to the SDK executable if it is not on PATH. The script also
checks `~/.dotnet/dotnet`. By default it produces separate Apple Silicon and
Intel apps, each containing the matching Swift UI, Native AOT bridge and updater.
It generates an icon, verifies every architecture and ad-hoc code signature,
and produces ZIP and drag-to-Applications DMG packages:

- `artifacts/macos/arm64/AgentLink-macOS-arm64.{zip,dmg}` for Apple Silicon.
- `artifacts/macos/x64/AgentLink-macOS-x64.{zip,dmg}` for Intel.

Pass `arm64` or `x64` as the second argument (after the output directory) to build
only that architecture. `split` is the default; optional `universal` combines both
architectures into one app with `lipo` and names its packages `AgentLink-macOS-universal`.
Cross-compilation requires a macOS SDK and the .NET Native AOT toolchain. Intel
execution can be checked on an Intel Mac or under Rosetta on Apple Silicon.

The resulting app is a local development build, **not Developer ID signed or
Apple notarized**. Distribution beyond local testing needs the owner's Apple
signing identity and notarization setup. Copy the app to `~/Applications` for a
stable Dock / Spotlight entry.

## First use

1. Open **Accounts** and sign in. GitHub device login creates a Copilot source
   after authorization. Azure accounts can add several resource sources.
2. Add reusable profiles under **Proxy** if needed, then select a Proxy on each
   **Sources** card. New sources default to system networking.
3. Start Bridge from the menu bar or **Bridge Service Settings**.
4. Open **Agent**, select each client's source and model, and preview/apply its
   configuration. Start a new client session to use the changed configuration.
5. Inspect local time-series usage and explicitly refresh GitHub quota in **Usage**.

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
while a configured Agent uses it. Two processes are persistent (app and bridge);
CLI helpers, including Azure token acquisition, are temporary. Desktop `appsettings.json` disables interactive
self-update and request-body tracing. Engine upgrades come with app rebuilds;
both packaged executables stay together. The main window has no HTTP admin API.

Daily `desktop-activity/*.jsonl` files contain completed request metadata and
sanitized error templates only. The UI reads the most recent 366 log-day files;
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
python3 scripts/test-source-proxies.py

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
Separate Apple Silicon and Intel distributions support macOS 14+; choose the
package matching the recipient's Mac. An optional universal build supports both.

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


## Multiple accounts and resource sources

Use **Accounts → Add GitHub login** for GitHub device authorization. No password,
PAT or manually entered username is required. After authorization the app reads
the GitHub username and adds its Copilot source. Repeat for another GitHub identity.
Each saved identity has its own encrypted credential directory, token refresh and
model catalog; logging out affects only that identity. The original default Copilot
login and legacy client URLs remain available.

Add an Azure account, complete device login, then select a subscription and AOAI
resource. The app reads that resource's OpenAI deployments. Repeat to add multiple
resources beneath the same account. New Azure identities have isolated CLI config
directories; **Local Azure CLI** explicitly preserves existing terminal sessions.
Manual resource entry also supports API keys in Keychain without Azure CLI login.
Resource discovery does not grant inference permissions or retrieve resource keys.

Agent selectors show **account / resource × model**. Model availability and Copilot
quota belong to their selected account; local usage retains each source ID and can
also be aggregated. An existing resource without account metadata is treated as
Local Azure CLI, preserving its URL, model selection and historical usage.
Account and resource mutations require a stopped app-owned bridge. The app never
stops the separately installed CLI bridge on port 8765.

The sidebar order is Overview, Accounts, Sources, Agent, Usage, Proxy, Bridge
Service Settings, and Errors. A persistent top-right Service Settings button opens the port,
and service lifecycle controls from any page.


Account-routing verification (macos.3): real Codex 0.141.0 with gpt-6-astra and
Claude Code 2.1.306 with claude-opus-5.5 completed multi-file tool tasks through a
named GitHub source. Codex evidence included three matching call/output pairs and
166 client log rows with zero router fatals; Claude recorded 4 tool round trips.
The installed Claude 2.1.179 was rejected by the existing model version gate; a
temporary 2.1.306 installation passed without updating the user's global CLI.
Azure subscription/resource/deployment discovery passed against the selected
subscription; live Azure inference remains UNVERIFIED for the permission reason above.


## Port editing and first-run guide

The guide starts with Accounts, then Sources, then starting Bridge, then Agent
configuration. A new GitHub device login creates its Copilot source only after
successful authorization. Pending login profiles do not enable service startup.
Azure accounts can add several resource sources after login or use manual API keys.

Pressing Start applies the port value currently entered in Service
Settings, even if Save was not pressed separately. An occupied new port is rejected
before stopping the existing owned service or replacing saved settings. Success is
reported only after the new listener signals readiness. Agent config files remain a
separate preview/apply action; changing a service port does not silently rewrite them.
The separately installed legacy bridge is never stopped by these controls.

The app-store/network contract check uses isolated preferences and application data,
starts the actual bundled AOT engine on free ephemeral ports, and never invokes a
client configuration write or a remote inference request:

```bash
bash scripts/test-macos-network.sh "artifacts/macos-network/AgentLink.app/Contents/Resources"
```

It checks edited-port startup, persisted settings, real HTTP listener movement,
occupied-port preservation, failed-start reporting, and account-before-source state.


## AgentLink identity and source proxies

The display name, window title, menu-bar item and distribution archives are
AgentLink. The bundle ID, executable names, Keychain service and Application
Support directory intentionally retain their existing identities, preserving
accounts, preferences, client command-auth paths and history across upgrades.

`COPILOT_BRIDGE_PROXIES_FILE` points to `proxies.json` beside `sources.json`.
Its `profiles` array contains public ID/name/mode/address metadata, `bindings`
map source IDs to profile IDs, and `account_proxy_id` selects transport for
account-only login, quota and Azure discovery. Built-ins are `system` and
`direct`. The former global desktop proxy migrates atomically to existing
source bindings on first launch. New sources use `system`. Referenced proxies
cannot be deleted; unknown references fail closed before startup. Stop the
owned service before editing profiles or bindings. No request switches a
process-wide proxy. A failed explicit proxy never falls back to direct.

The menu bar remains available after closing the main window. Both left and
right mouse buttons open the same native menu, with service controls and usage
readback that do not activate the window. Page shortcuts reopen that page.

Proxy-routing verification (macos.6): real Codex gpt-6-astra and Claude Code
claude-opus-5.5 completed multi-file tool tasks through two different explicit
CONNECT proxies in one bridge process, sharing a GitHub identity. Both proxies
carried GitHub auth and Copilot model connections. Codex recorded four matching
tool round trips and nine passing task tests, with zero router fatals in the
client log window (158 rows); Claude recorded three round trips and fourteen
passing task tests. Disposable credential copies were removed after the run.

`test-source-proxies.py` additionally checks 24 concurrent same-model Azure
fixture requests across two proxies and direct networking, credential isolation,
and fail-closed behavior for an unavailable proxy. The Swift calendar/migration
checks and .NET profile checks include deliberate rejected mutations.

The native menu contract (`scripts/test-macos-menu.sh <app-resources>`) verifies
both mouse event masks, status-item persistence, navigation and actual AOT
start/stop while the main window is closed. Removing right-click handling makes
that test fail. UI review also covers creating a Proxy, independently binding an
Azure source, and switching hourly/daily usage charts in a disposable app.
