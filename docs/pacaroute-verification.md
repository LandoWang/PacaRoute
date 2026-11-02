# PacaRoute 0.6.0 verification

For the newer shared Web UI/native tray distribution, see [0.7 verification](pacaroute-web-desktop-verification.md).

Verified on 2026-11-01. Native macOS build 17; Go 1.26.1. Release applications
were built from an isolated index export that excludes unrelated configuration-restore
and legacy .NET working changes. Existing installed services and Agent settings were preserved.

## Real clients against the packaged Go executable

| Route | Tool round trips | Client dispatch evidence | Result |
| --- | ---: | --- | --- |
| Codex → Copilot GPT 6 Astra | 6 | 1000 log rows, zero router fatals, two completed turns | PASS |
| Claude Code → Copilot Claude Opus 5.5 | 3 | 3 transcript pairs and a complete final answer | PASS |
| Claude Code → Copilot GPT 6 Astra | 3 | 3 transcript pairs and a complete final answer | PASS |
| Codex → Copilot Claude Opus 5.5 (function/custom tools) | 6 | 674 log rows, zero router fatals, two completed turns | PASS |
| Codex → Azure Foundry GPT 6 Astra (Azure CLI / Entra) | 6 | 1000 log rows, zero router fatals, two completed turns | PASS |

These five runs produced 32 successful upstream requests with complete input/output
token records and no false stream errors. Each Codex run executed scripts, read their
results, restarted the gateway and continued the same thread with an unchanged
sticky binding after the healthy member was assigned weight zero. A missing-credential
member exercised failover. Azure calls used the existing account authority through
the auth facade; no refresh-bearing credentials were copied.

The Codex-to-Claude leg disables provider-owned web search; it is an advanced route,
not an unrestricted promise of native Codex server-tool compatibility. Native custom
and namespace payloads, image tool results and translation details additionally have
wire-contract coverage. Real tools used by these headless runs are reflected in the
round-trip counts above. Forced auth rejection and refresh races use synthetic contracts.

Final run directories (under ignored `artifacts/pacaroute-verification`):

- codex-native: `live-appserver-20261101-130831-d56685`
- claude-native: `live-claude-native-20261101-130831-04e102`
- claude-cross: `live-claude-cross-20261101-130831-f085b8`
- codex-cross: `live-appserver-20261101-130905-a12e60`
- azure: `live-appserver-20261101-130905-d30e1d`

All five manifests record this exact bundled arm64 Go SHA-256:

`a25ace2104def039f125c11bd1e7b886cf419ec10f5b55787aeb338429b4b8fd`

Inspect explicit manifests with `scripts/verify-pacaroute-evidence.py`. It checks
wire call/result IDs, Claude transcripts, Codex dispatch logs and executable hashes.
Actuators are `scripts/verify-pacaroute-codex.py` and
`scripts/verify-pacaroute-claude.py`; they isolate ports, metadata and client homes.

## Contracts and interfaces

- `go test -race ./...`: 25 contract tests passed; `go vet ./...` passed.
- Four deliberate regressions were caught: waiting past a terminal SSE frame,
  grouping changing opaque item IDs, bypassing source pause, and dropping tool images.
- Six Swift core checks and local-session checks passed.
- Native menu harness passed left/right clicks, closed-window startup/stop,
  hot source edits, paused-source removal and a live Go health request.
- Browser UI passed management login, source pause/resume, removal while paused,
  Client creation with one-time key display and navigation through management pages.
- Packaged CLI passed account/source/Client CRUD, reference protection, pause/resume,
  models, usage and errors.
- Native sources and browser sources pages were rendered and reviewed.

## Distributions and limits

- Separate arm64 and x86_64 Swift + Go apps, ZIPs and DMGs were built. Each app
  contains only `pacaroute` in its bundled backend directory.
- Both app architectures and ad-hoc signatures verified; both DMGs verified.
- The x86_64 Go executable ran under Rosetta. Physical Intel Mac validation is
  **UNVERIFIED**. macOS 14 or newer is required.
- Standalone darwin/linux arm64/amd64 archives were produced. Linux executables
  are static ELF binaries. Container runtime validation is **UNVERIFIED** because
  the configured Docker daemon was not running. The Dockerfile is provided.
- Developer ID signing and Apple notarization have not been performed.
- Historical .NET sources remain as reference and are not shipped or invoked.

Distribution hashes are recorded locally in `artifacts/PacaRoute-SHA256SUMS.txt`.
Raw prompts, traces, credentials and local screenshots are not committed.
