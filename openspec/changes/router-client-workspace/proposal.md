# Router workspace and durable configuration

Organize PacaRoute around Accounts, Sources, Pools, Clients and Routes. Remove
standalone Agent and history/session pages; retain per-source Proxy bindings and
session affinity within the router. Clients enable Codex and/or Claude Code and
provide portable connection scripts. Routes map each Client/Agent to a source or
pool independently of connection setup.

Persist validated configuration snapshots and revision numbers in SQLite. Import
legacy split JSON or its committed recovery journal once, retaining original
files. Secrets remain under Auth. Fresh databases contain a local-only default
Client with both Agents enabled but no routes. Imported installations preserve
legacy local direct routing visibly until an explicit route edit replaces it.
Deletion of default persists and closes unauthenticated local access.

Account login offers built-in GitHub device authorization or a custom OAuth app,
and shared Azure CLI identity or isolated device authorization. Provider-issued
refresh metadata remains exclusively managed by Auth.

Required validation: migration/restart/deletion, stale and invalid writes,
Agent authorization before models and routing, no network/forwarded local bypass,
script quoting/config preservation, web build, and real Codex/Claude routing.
