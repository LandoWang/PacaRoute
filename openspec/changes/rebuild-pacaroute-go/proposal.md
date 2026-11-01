# PacaRoute: Go gateway, CLI and UI

Replace the distributed .NET runtime with one native Go executable, named
pacaroute. Keep the macOS SwiftUI application and its local session reader, add
an embedded browser management UI for server deployments, and expose the same
management operations to the CLI. Preserve the aurora alpaca artwork.

A Client represents an authenticated caller, independently of the Codex/Claude
protocol adapter. Client keys resolve identity server-side. Ordered client rules
map protocol and model alias to a source/model or weighted pool. Session affinity
is scoped by client, protocol and pool. Public listeners require client keys;
administration requires a separate administrative credential.

Configuration changes publish validated snapshots. Adding accounts/sources,
pausing a source, editing routes/proxies and changing pool weights do not restart
the listener. In-flight responses finish on their selected source. Future requests
respect pause/removal and the configured failover policy. Credentials stay behind
the authentication facade, persist encrypted, and never enter public configuration,
request logs or client configuration. Account isolation and rotating refresh-token
coordination must be preserved during migration.

Native Responses and Anthropic paths preserve unknown fields and tool payloads.
Explicit cross-protocol adapters handle Claude-to-Responses and Responses-to-Claude.
Carry forward probe-grounded Copilot compatibility rules, bounded streaming,
cancellation, usage accounting, config preview/backups and rollback protection.

The user explicitly requests Go; this supersedes .NET-specific implementation
requirements in AGENTS.md. Retain the .NET source as migration/reference material
until parity is verified, but do not ship it as the final backend.
