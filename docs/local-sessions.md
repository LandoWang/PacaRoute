# AgentLink local sessions

The Sessions page reads Codex and Claude Code histories on demand. Browsing does
not start/resume agents or require Bridge. Overview shows recorded metadata,
available cumulative token counts, recent excerpts, and counts within loaded
history. History separates messages from tools, errors and context boundaries.
Search covers catalog titles/projects or text within already loaded history.

## Storage and coverage

- Codex: `CODEX_HOME` or `~/.codex`; versioned `state_*.sqlite` supplies the catalog
  and `thread_history_*.sqlite` supplies visible items. System SQLite uses
  `SQLITE_OPEN_READONLY` and `query_only`; no migrations or repairs run. JSONL under
  `sessions` and `archived_sessions` supports legacy histories. Database and byte
  cursors remain distinct, including threads missing a database projection.
- Claude Code: `CLAUDE_CONFIG_DIR` or `~/.claude`; `projects/**/*.jsonl`. Internal
  API request copies and reasoning are excluded. Tool results in `user` envelopes
  are not shown as human requests. Available `cost-state` snapshots are used once;
  thinking tokens are not added again to output. Missing usage remains unknown.
- History directory can select an alternate root. Client configuration and
  credentials are not read. Rollout pointers outside the selected root are refused.

Catalogs page 80 sessions; histories page 100 visible records. JSONL reads at most
4 MB per page, with byte-boundary continuation and incomplete append tolerance.
Individual texts are capped at 16,000 characters; SQL items over 2 MB become explicit
omission markers. File catalogs scan at most 5,000 candidates with bounded metadata
previews. Unsupported database schemas show an error. Format changes may require
adapter updates. Oversized individual JSONL records can be omitted with a notice.

`copilot-bridge` identifies an entry point, not a historically verified upstream.
Never attribute it to Copilot/Azure using today's defaults. Recorded model/status
does not prove a client is currently running or has loaded the latest configuration.
Client-recorded token counts are separate from Bridge totals to avoid double counting.

## Explicit analysis

Choose a source/model, inspect **Preview sent content**, then **Analyze session**.
This sends a single tool-free, non-stored Responses request to the existing loopback
Bridge source route. Bridge owns authentication and source-specific networking;
analysis does not change Agent defaults. Redirects are refused, the local HTTP
session is ephemeral, and it bypasses outbound proxies for the loopback connection.
Analysis consumes the selected provider's usage (currently labeled Responses/Codex
by Bridge telemetry, not attributed to the historical session).

The request prioritizes recent loaded evidence within 48,000 characters, with at
most 6,000 per record. Omitted older history and shortened records are disclosed.
Results cite visible `E` record numbers, distinguish claims from tool evidence,
and explain unresolved work and a handoff. Historical instructions are untrusted
evidence; no historical commands execute. Results remain in window memory and clear
when history changes. The model may make mistakes; references support verification.

Common tokens, authorization headers, key/value secrets, URL credentials and private
key blocks are redacted before display/analysis. This is best effort, not a guarantee
for arbitrary transcript secrets. Images are attachment markers. Full-history AI
indexing, live process control, exact upstream attribution and cross-client
continuation are outside this feature. No new daemon or credential store is added.

## Verification

```bash
swift run --package-path apps/macos LocalSessionChecks
swift run --package-path apps/macos LocalSessionChecks --local
swift run --package-path apps/macos BridgeCoreChecks
bash scripts/test-macos-history-analysis.sh <source-id> <model> <running-bridge-port>
```

Local inspection prints counts only. The analysis check sends a synthetic invoice
conversation with an unresolved failing test and an instruction-injection string
inside a tool result. Review the actual answer for failed-test evidence, distinction
between claims and proof, citations, and ignoring the injected instruction. It sends
no real user history. Verify both Copilot and Foundry routes. This feature adds an
application request; it does not change Codex/Claude protocol compatibility.
