# PacaRoute 0.8.6 verification

Verified on macOS arm64 on 2026-11-02. Packaged desktop binary SHA-256:
`c01c327cf8e632edeae2d423a691677750dbdb1ee61e70a07b01409fb21af642`.
Personal traces, credentials and QA artifacts are not committed.

## Incident and scope

The original incident had six failed streamed requests on the same Azure source
after a successful tool turn. Version 0.8.5 retained only a generic interruption
message. Direct probes, including a reconstruction of the incident history,
subsequently completed; the exact cause of those original requests is unconfirmed.

A later real-client QA run captured an Azure Provisioned-Managed throughput
rejection. Its SSE wire contained a nested `error` (`rate_limit_reached`) followed
by `response.failed` (`rate_limit_exceeded`). The old relay returned after the
first frame, hiding the terminal event Codex needs. It also failed to cool down
the source after streaming errors, so client retries remained sticky to it.

The relay now preserves the native failure terminal and records bounded provider
code/message or transport/last-event details. After a bare Responses error, it
waits at most one second for a following failure terminal; otherwise it emits a
failure terminal with the diagnostic. Transport EOF similarly produces a failure,
never a fabricated completion. Upstream streaming failures cool down that
source/model for 30 seconds. A later client retry can fail over under the pool's
existing rules. No started stream is replayed internally, source-bound references
cannot migrate, and client cancellation/write failure does not cool down a source.

## Real-client evidence

All manifests below are under the private `artifacts/pacaroute-verification/`.
The two fault-injection runs used a separate QA entry point that installs a
process-local TLS trust bundle and calls the unchanged `pacaroute.Run`; they are
not represented as tests of the packaged entry point. No OS trust or user
credentials were changed/copied. That QA binary's hash is
`46ae3b8ba843e12c1aae0f6d086d733177271cbdf1be45df0eab2761cecbee6b`.

| Path | Manifest suffix | Evidence |
| --- | --- | --- |
| Partial text → forced EOF → live Copilot Astra | `live-appserver-20261102-105622-872b58/appserver.manifest.json` | one fixture call, next request on healthy member, reason visible to Codex, six matched tool roundtrips, two complete turns including restart/continuation |
| Captured Azure error shape → live Copilot Astra | `live-appserver-20261102-105809-f8d57a/appserver.manifest.json` | one fixture call, native failure reason visible to Codex, next request recovered, six roundtrips, two complete turns |
| Packaged Codex → Copilot Astra | `live-appserver-20261102-105809-4072d0/appserver.manifest.json` | six roundtrips, two complete turns including restart/continuation |
| Packaged Claude native → Copilot Opus 5.5 | `live-claude-native-20261102-105703-e81295/manifest.json` | four transcript/wire tool pairs and complete final answer |
| Packaged Claude translated → Copilot Astra | `live-claude-cross-20261102-105704-77e046/manifest.json` | four transcript/wire tool pairs and complete final answer |

Each Codex run's isolated SQLite dispatch database had 1,000 rows in the checked
window and zero router fatals/incompatible-payload/namespace/polymorphism errors.
The evidence script checks actual tool calls/results, client-visible failure
details, single failed attempt followed by recovery, completion and binary hash.
The overloaded Azure QA attempt is retained as a failed run, not a compatibility
pass. Synthetic replay plus the captured rejection do not establish spare Azure
capacity or eliminate future provider limits.

## Cooldown visibility

Authenticated `/api/status` includes an in-memory routing snapshot: source/model,
last failure reason/time, cooldown expiry, and eligible/total members per pool.
The frontend refreshes it every five seconds while visible. Sources show model
cooldowns; pools exclude cooled/paused/zero-weight members from displayed new
session allocation. A global warning shows cooldowns; zero eligible members
produces a red alert naming the exhausted pool.

Browser QA against an isolated packaged service confirmed 2/2 → 1/2 → 0/2,
50/50 → 0/100 → 0/0 allocation, error details, countdown and source-level badges.
Screenshots were visually inspected. Expiration is a return to eligibility, not
a health probe. Runtime cooldowns reset on service restart; historical errors
remain in Activity. This is UI visibility, not a background notification service
or a distributed circuit breaker.

## Regression and distribution

- Go race suite and vet passed; all 19 frontend tests and TypeScript/Vite build passed.
- Contracts cover native Azure frame fidelity, missing terminal, bounded error
  wait, downstream disconnect, source-bound continuation, disabled failover,
  model-scoped cooldown, authenticated health snapshots and expiry.
- Six mutation checks passed, including removing stream cooldown and dropping
  the Azure terminal frame.
- Separate Apple Silicon/Intel macOS packages, Windows x64 package and platform
  CLI archives were built. Intel/Windows physical tray behavior remains untested;
  macOS packages remain ad-hoc signed without notarization.
- The current local service was gracefully upgraded to this packaged binary;
  its configuration was verified unchanged.
