# Source pool verification

Build 16 uses engine `0.5.28-macos.10`. Both macOS architectures are separate
Native AOT distributions. The unchanged updater and icon are reused from verified
build 15.

- 43 focused .NET contracts passed, covering source pools, source/harness routing
  and desktop configuration. Mutations that reassign every request or cool a
  source after client cancellation made the corresponding contracts fail.
- macOS store checks passed for wire format, source membership, Agent selection,
  live edits and deletion guards. Both Swift release architectures compiled.
  The actual SwiftUI pool section and editor were rendered and visually inspected.
- Real Codex CLI on `gpt-6-astra` completed seven matched tool round trips across
  initial execution, live weight editing and restart/resume. A deliberately
  unauthenticated member failed over to authenticated Copilot; the persisted
  journal contained exactly the failed and replacement bindings. Setting the
  replacement weight to zero and restarting Bridge did not move the session.
- Real Codex app-server repeated failover and restart with six matched tool round
  trips. Its explicitly flushed, isolated SQLite dispatch log contained 1,000
  events, zero ERROR rows and zero router fatal rows. Codex CLI itself does not
  populate that log, so its empty database was not treated as passing evidence.
- Real Claude Code on `claude-opus-5.5` completed three matched tool round trips
  after the same forced unavailable-member failover, with a successful final
  result. All client tool results were cross-checked against captured loopback
  request/response bytes. No installed credential files were copied.

Local evidence lives under `artifacts/source-pool-verification/`, including
`live-verdict.json`, the client-owned logs, wire captures, screenshots, contract
logs and mutation failures. Distribution hashes and architecture/signature checks
are recorded in `artifacts/agentlink-source-pools-release/verification.json`.

Live model execution used Copilot. Azure member validation and route/model
selection were covered by contracts; this verification does not claim a live
cross-provider Azure migration. Server-only history references cannot move to
another source and return a clear 409. Failover never splices another provider
into a response after output has begun.
