# Design

Only the exact probed Copilot model selects Chat Completions; other native paths retain their current behavior. Translate requests and responses explicitly, preserve tool identity (including Codex namespace/custom tools), finish/error semantics, image inputs and token usage. Opaque reasoning travels in an authenticated encrypted replay capsule in Responses reasoning items / Messages redacted-thinking blocks. Capsules are bound to Client, Agent, source endpoint/account/model and session, with a stable installation key, permitting restarts while rejecting cross-scope reuse. Unsupported source-bound references or unrepresentable content fail explicitly.

The gateway uses the existing Auth lease and Copilot headers. Streaming waits for the Chat finish reason and `[DONE]`, retaining final usage and withholding executable tool completion until the upstream turn completes. Limits bound buffered tool arguments and opaque state. No production listener or configuration is modified during testing.

Evidence: artifacts/argon-probe/catalog.json, tool-request.json, tool-response.txt, continuation-request.json, continuation-response.txt (local only). Catalog visibility in PacaRoute is separate from Codex desktop picker metadata; exact-model inference is the acceptance target.
