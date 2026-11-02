## ADDED Requirements

### Requirement: Stream failures affect subsequent pool admission
An upstream stream failure SHALL cool down the selected source/model for 30
seconds. The router SHALL NOT replay a response after output begins. Subsequent
client requests SHALL obey configured failover and source-bound reference rules.

#### Scenario: Partial output followed by EOF
- **WHEN** an upstream emits partial output then closes without a terminal event
- **THEN** the partial output and a protocol-appropriate failure reach the client
- **AND** no second source is called in that request
- **AND** a full-history client retry may select another eligible pool member.

#### Scenario: Caller disconnects
- **WHEN** the client cancels or downstream writing fails
- **THEN** the request is recorded as a client disconnect
- **AND** its source is not penalized.

### Requirement: Preserve semantic failure details
Native failure frames and unknown fields SHALL remain intact. A Responses `error`
SHALL NOT hide a subsequent `response.failed`. Waiting for that terminal SHALL be
bounded; absent a terminal, a failure SHALL be emitted without inventing success.

#### Scenario: Azure nested error followed by terminal failure
- **WHEN** Azure sends `error` then `response.failed`
- **THEN** both frames reach Codex unchanged
- **AND** the activity record retains the provider's error code and message.

### Requirement: Operators can observe cooldowns and pool exhaustion
Authenticated management SHALL expose active source/model cooldown reason,
failure time and expiry, plus pool membership eligible for new sessions. The UI
SHALL refresh this state and distinguish it from manual pause and proven health.

#### Scenario: Every pool member is unavailable
- **WHEN** cooldown, pause or zero weight excludes every member
- **THEN** the UI names the exhausted pool in a global alert
- **AND** displays zero eligible members and zero effective allocation.

#### Scenario: Cooldown expires
- **WHEN** a source/model's cooldown deadline passes
- **THEN** it becomes eligible subject to pause/weight settings
- **AND** the UI does not claim an upstream recovery probe succeeded.
