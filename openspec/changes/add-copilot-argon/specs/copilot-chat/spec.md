# Copilot Chat conversion

## ADDED Requirements
### Requirement: Native Go Argon execution
Codex and Claude clients SHALL execute multi-turn tools through the exact Copilot Argon model, without Python or a separate proxy.
#### Scenario: Tool continuation
- GIVEN a completed Argon tool response with opaque reasoning
- WHEN the same client session returns tool outputs
- THEN tool identities and opaque reasoning reach Copilot intact, including after a gateway restart.
#### Scenario: Stream failure
- WHEN a Chat stream ends without a finish reason and terminal marker
- THEN no successful downstream terminal or executable incomplete tool is emitted and no in-stream failover occurs.
#### Scenario: Isolation
- WHEN another client, session, source, account, endpoint or model replays opaque state
- THEN the request is rejected before inference.
#### Scenario: Usage
- WHEN intermediate usage is replaced by final usage
- THEN only the final snapshot is settled, with cached input counted once; absent usage remains absent.
