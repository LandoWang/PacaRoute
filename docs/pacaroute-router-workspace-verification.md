# PacaRoute 0.8 router workspace verification

Date: 2026-11-02. Scope: explicit Clients and Agent allowlists, independent Pools
and Routes pages, portable connection scripts, account login methods, source Proxy
selection, removal of chat-history features, and SQLite configuration migration.

## Automated contracts

- 41 Go contracts pass under `go test -race ./...`; `go vet ./...` passes.
- 10 frontend tests pass on macOS; the Bash execution test is platform-specific
  and skipped on Windows. The frontend type check and production build pass.
- SQLite migration preserves legacy files, imports a committed legacy journal,
  rejects invalid writes, survives default-Client deletion/restart, preserves
  immutable snapshots and rejects stale/concurrent writers.
- Agent allowlists are checked before model listing and route resolution. A
  deleted local Client, non-loopback callers, forwarded requests and public
  deployment cannot acquire the default identity. A network Client named `local`
  has a separate affinity scope from the default local Client.
- Mutation checks removing the gateway Agent gate and stale revision rejection
  both failed their targeted contract tests. Source was restored before release
  builds and final tests.
- Connection planning preserves unrelated TOML/JSON, avoids leaking keys into
  previews, makes exact backups, and creates previously missing Claude settings.
  Bash script arguments survive shell metacharacters without command evaluation.

## Real clients on the final macOS arm64 app

Binary: `artifacts/pacaroute-0.8-release/macos-arm64/PacaRoute.app/Contents/MacOS/pacaroute`

SHA-256: `b9ba2419205866d6800b4a6f45a4465b70902a6929b17e17109aca337027a4b3`

All three actuators used the shipped `connect` command in isolated Agent homes.
No installed Agent configuration or service on port 8765 was modified. Refresh-
bearing credentials were not copied. The Azure legs reused the existing Azure
CLI identity through an account with `login_mode: azure-cli`.

| Path | Client-owned execution evidence | Verdict |
| --- | --- | --- |
| Codex app-server → Client route → weighted pool → existing Foundry Astra deployment | Two completed turns, six matching wire tool roundtrips, 1,000 dispatch log rows, zero tool-router fatals; bridge restart retains the binding after a SQLite weight change | PASS |
| Claude Code → Client route → Foundry Astra | Four wire and transcript tool roundtrips; complete final answer and correct computed result | PASS |
| Claude Code → Client route → Copilot Claude | Four wire and transcript tool roundtrips; complete final answer and correct computed result | PASS |

The tasks read input, write and execute a program, read its result separately and
verify the sum of unique values (27). Codex continues after a service restart and
adds the original input count (6). Foundry activity contains eight successful
Codex requests and five successful Claude requests with nonmissing input/output
usage. These are execution checks, not throughput or billing benchmarks.

Only these final run manifests were used for the release verdict:

- `artifacts/pacaroute-verification/live-appserver-20261102-004341-581d6b/appserver.manifest.json`
- `artifacts/pacaroute-verification/live-claude-cross-20261102-004341-776080/manifest.json`
- `artifacts/pacaroute-verification/live-claude-native-20261102-004341-2e7377/manifest.json`
- Verdict: `artifacts/router-workspace-qa/final-verdict.json`

## UI and distribution

The isolated browser preview verified the navigation without Agent/history pages,
source/model route selection and persistence, Client connection-script generation
for Bash and PowerShell, and the Azure CLI/device login choices. No browser errors
or warnings were captured. Screenshot: `artifacts/router-workspace-qa/clients.png`.
The preview retains previously created account metadata and references its existing
credential authority; it uses an independent SQLite database and port.

Apple Silicon and Intel macOS bundles and DMGs, Windows x64 portable ZIP, Linux/
macOS/Windows CLI archives, and the standalone web ZIP were built separately.
macOS bundles pass ad-hoc signature verification; notarization is not included.
Windows binaries are unsigned. Physical Windows/Intel tray interaction and
PowerShell execution remain UNVERIFIED locally. Custom GitHub OAuth forced token
rotation was not performed against a user's live rotating credential; synthetic
refresh/lease tests cover that contract.

The previous 0.7 Windows CI failure concerned the removed chat-history index
reader. It is not evidence about the new SQLite configuration store; the 0.8
private CI run is recorded separately after push.
