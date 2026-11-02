# Recover from streamed upstream failures and expose cooldown state

Started streams could fail without cooling down their sticky source. Azure's
`error` followed by `response.failed` was truncated at the first frame, making
Codex report a missing completion instead of the provider's reason. Operators
could not see source/model cooldowns or exhausted pools.

Preserve semantic failure terminals, record actionable bounded diagnostics,
cool down failed pool source/models for subsequent client retries, and expose
runtime admission state in authenticated management and the UI. Keep no-replay,
source-bound continuation and client-cancellation invariants.
