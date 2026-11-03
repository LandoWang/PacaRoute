# Portable configuration and multi-model source pools

PacaRoute configurations should move between installations without carrying
credentials or colliding with the receiving workspace. Fresh installations should
start without pre-created accounts. Pools should act as sources exposing multiple
models, with independent source/deployment throttles and proportional allocation.

## Behavior

- Versioned JSON exports include accounts, sources, proxies/bindings, pools,
  Clients/routes, enabled Agents and paused state. They exclude credentials, Client
  hashes, admin keys, machine settings, usage, affinity and error history.
- Management UI/API preview an additive or replacing import before applying it.
  Add preserves existing objects and renames conflicting labels. Every imported
  identity receives a fresh namespace; all internal references are rewritten.
  Replace substitutes the configuration graph without importing credential state.
- Imports require account authorization and generate new network Client keys
  through Auth. A colliding local Client becomes an authenticated network Client.
  Azure CLI identities import as isolated accounts to prevent unintended reuse.
- Fresh accounts/sources are empty. Referenced legacy implicit identities are
  materialized with their original IDs, preserving credential ownership and locks.
- Multi-model pools expose the union of member aliases (concrete model by default).
  Selection, failover and session affinity are scoped to the requested public model.
  Legacy single-alias pools keep their behavior and affinity keys.
- Throttling is keyed by source plus concrete deployment. Each has its own deadline,
  honoring bounded HTTP Retry-After hints. Surviving members within the same model
  keep their relative weights. Model-level exhaustion remains visible even while
  other models in the pool can receive requests.

## Constraints

No upstream credentials are exported. Pool Agent/protocol compatibility is unchanged.
Source-bound references never migrate, and started responses are never replayed.
Imports commit using the existing SQLite revision guard; generated Client secrets
are hash-versioned and remain behind Auth. The global login Proxy is preserved on
add; individual source Proxy bindings are always imported.

## Peer discovery and Client handoff

Treat a remote PacaRoute as an account authenticated by a remote Client Key. Its
source discovers only that Client's public model aliases and per-Agent access.
Responses and Messages are forwarded natively; upstream accounts/pools remain
opaque. A bounded hop header prevents forwarding cycles and derived session IDs
isolate outer Clients. Remote sources can participate in local pools.

Client QR codes contain a versioned connection payload: service URL, Client Key,
and optional display name. Image and camera decoding run in the browser; imported
connection information is reviewed before saving the account. Local Clients
have no portable Key and cannot generate a sharing QR. Configuration exports
retain peer metadata but never copy Client/provider credentials.

## Independent management and serving listeners

Desktop keeps a loopback management listener alive independently of its Agent
listener. Tray and authenticated management controls can open, close or change
the serving port. Closing admission drains accepted requests without resetting
Auth, routing affinity, cooldowns, usage or configuration. Port conflicts preserve
the existing listener. Management and Agent surfaces cannot be accessed through
each other's ports. Settings preserve the desktop serving state across launches.
CLI deployments opt in through `serve --admin-port PORT`; `--no-serve` starts
management with inference disabled. Legacy single-port CLI behavior is retained.
