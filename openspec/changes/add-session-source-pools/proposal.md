# Weighted source pools with persistent session affinity

Add named source pools alongside individual sources. Each pool targets Codex or
Claude Code and maps one client-facing model to source/model members with relative
integer weights. Choose once per new session, persist a hashed session binding,
and keep existing sessions on their member when only weights change. Zero weight
drains new sessions without moving existing ones. Pool edits reload without
restarting Bridge; individual source credentials and proxy bindings stay in their
existing services.

The user explicitly enabled failover. Retry authentication, throttling, transient
server and transport failures on another eligible member before forwarding a body.
After output starts, never splice or replay another response; mark the member
unhealthy for the next request. Limit attempts to one per eligible member and honor
bounded cooldowns. Require a real client session identifier; never group unrelated
sessions by IP address, agent configuration, model, prompt or request ID.

Use /pools/{id}/{agent} routes and a separate source-pools.json configuration. Reuse
existing source routes through a proxy-free loopback transport in the same Bridge
process, preserving the established credential, protocol and telemetry paths.
Only the member's model changes in replayable request bodies. Server-only response
references cannot silently migrate to an unrelated provider: surface a clear error
when complete history is unavailable. Request usage belongs to the actual member;
failover notices are metadata events and do not add an extra request count.

Validate weighted selection, session separation, restart persistence, draining,
failover/cooldowns, cancellation, response-byte fidelity and local-only transport.
Run a real headless client on a multi-step tool task through a pool, including a
forced unavailable member, and verify client dispatch logs before shipping.
