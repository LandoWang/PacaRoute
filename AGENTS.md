# AGENTS.md

## Project and build

PacaRoute is a Go 1.26 agent request gateway with a CLI, authenticated management
API, shared React web UI and optional Go macOS/Windows system tray. The release runtime
is Go. Historical .NET sources under `src/` and their tests remain reference
material; .NET/AOT/source-generation rules apply only when editing that legacy code.

- `go build ./cmd/pacaroute`
- `go test -race ./...`
- `go vet ./...`
- `npm --prefix apps/web ci && npm --prefix apps/web test`
- `bash scripts/build-web.sh`
- `bash scripts/build-pacaroute-desktop.sh`
- `bash scripts/build-pacaroute.sh artifacts/pacaroute-cli`

Go code is in `internal/pacaroute`; `cmd/pacaroute` is the thin CLI entry point.
The React UI is in `apps/web`, compiled independently and embedded from
`internal/pacaroute/web`. Local management/configuration code and the desktop tray are in
`internal/pacaroute`. Historical SwiftUI sources remain in `apps/macos`. Architecture is documented in `docs/pipeline-design.md`
and operational behavior in `docs/pacaroute.md`. Track changes with OpenSpec under
`openspec/changes/`; keep progress out of this file.

## Invariants

- Write repository docs, comments and commit messages in English; match the user's
  language in conversation. Preserve unrelated working-tree changes.
- Tests assert contracts, not implementation details. State the required behavior
  first and mutation-check important new regressions. Do not weaken failed assertions
  to make a test pass.
- Read the protocol research and relevant read-only `references/` before changing
  Copilot headers, authentication or model behavior. Never edit `references/`.
  Model-profile coercions must come from probe/captured-request evidence, never
  family-name guesses. Preserve unknown native protocol fields and upstream errors.
- Credential records, encryption, login, refresh, exchange and rejection belong
  exclusively behind `Auth`. Request handlers receive immutable leases. Never print
  credentials, copy refresh-bearing tokens for QA, or put secrets in argv/logs.
  The existing credential lock/encryption format is a compatibility contract.
- Config snapshots are immutable. Mutations validate references and commit under
  the shared revision/lock/transaction mechanism. Invalid edits keep the last valid
  snapshot. Removing/pausing routes affects future requests, not active streams.
- Authenticate a network Client before resolving its routes. Caller-controlled
  Client-ID headers must never authorize a route. Sticky scope includes Client,
  Agent, Pool and Session. No cross-source failover after downstream output begins;
  source-bound response references cannot migrate to another source.
- Missing usage remains missing. Final SSE usage replaces earlier snapshots.
  Do not repeatedly scan whole log/session files in refresh loops.
- Never kill a service by image name. Identify and stop only the QA PID started by
  the current task. Never bind/kill the user's 8765 instance. Isolate test ports,
  client homes, dispatch databases and workspaces.
- Never stage `.codex/config.toml`, credentials, personal traces, `references/`,
  local artifacts or user configuration. Push only to the private remote when
  authorized; do not push this fork to public `origin`.

## Real-client verification

Use `.agents/skills/real-client-verify/SKILL.md` whenever changing client protocol,
routing, streaming or promised model compatibility. Its evidence requirements
remain mandatory for Go; use the Go subprocess actuators instead of launching .NET.
A native/client compatibility change is not verified by unit tests or HTTP 200.

- Run real Codex app-server on a multi-tool task and continuation. Read the run's
  isolated `logs_2.sqlite`: actual matched calls/results, no aborted dispatch and
  no `tools::router` fatal, `incompatible payload`, `Missing namespace` or
  `Polymorphism_`. Codex CLI alone does not populate that dispatch database.
- Run real Claude Code on a multi-tool task; inspect tool_use/tool_result pairs and
  the final complete answer. For Claude-to-GPT, run the translated path explicitly.
- The task must exercise the changed path. Custom/namespace/grammar tool behavior
  additionally needs captured wire contracts where the headless client does not
  emit those shapes. A faithful Grok passthrough rejection is not a promise that
  Grok implements native Codex agent behavior.
- Examine only this run's manifests/traces. If live validation is unavailable,
  report the affected path as UNVERIFIED rather than presenting synthetic tests
  as a real-client pass.
- Mirror changes to repo-owned skills between `.agents/skills` and `.claude/skills`
  except self-referential paths. `CLAUDE.md` imports this file, not a second copy.
