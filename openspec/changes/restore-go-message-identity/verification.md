# Message identity verification — 2026-11-03

## Evidence and scope

The installed Go gateway relayed Copilot's SSE frames unchanged. A baseline
request contained 29 distinct opaque message IDs for 29 lifecycle references to
one output slot. The archived September C# fix already documented this provider
behavior; its canonical-message-ID correction was absent from the Go relay.

The repair applies only to native Codex requests served by Copilot Responses.
It retains the added message's ID for that output index in content events,
item completion and the terminal output array. Reasoning, tool-call and response
IDs retain their upstream values. Other providers and translated Claude paths
remain unchanged. Existing Gemini work is preserved.

## Contract and static checks

- The historical sanitized rolling-ID capture fails before the correction and
  passes after it, with every non-message-identity JSON value preserved.
- Stable streams and ordinary provider passthrough remain byte-for-byte identical.
- Tests cover multiple messages, commentary/final phases, intervening tool items,
  completed/incomplete/failed terminals, missing added IDs, deterministic replay,
  multiline SSE, CRLF metadata and large integer precision.
- Gateway tests verify source scoping and unchanged raw upstream traces.
- `go test -race ./...`, `go vet ./...` and `git diff --check` pass.
- A Go overlay mutation disabling the gateway's normalizer fails
  `TestNativeCopilotRelayKeepsMessageIdentityAndRawTrace/copilot` as intended.

## Real-client results

Each run used an isolated service port, client home, workspace and dispatch DB.
Credentials were accessed through the existing authority without copying records.

| Run | Tool round trips | Started/completed message mismatch | Result |
| --- | ---: | ---: | --- |
| Original desktop binary, Codex two turns | 6 | 4 / 4 messages | Identity FAIL |
| Repaired binary, Codex two turns | 6 | 0 / 4 messages | PASS |
| Signed desktop package, Codex two turns | 6 | 0 / 4 messages | PASS |
| Repaired binary, Claude native | 4 | N/A | PASS |
| Repaired binary, Claude to GPT | 4 | N/A | PASS |

The signed desktop package completed both the initial multi-tool task and the
continuation after a test-service restart. Each final canary appeared once.
The client dispatch DB contained 296 rows and zero router fatals. The repaired
message events had no orphan starts, unmatched completions or unmatched deltas.

Artifacts (local, excluded from version control):

- Baseline: `artifacts/pacaroute-verification/live-appserver-20261103-220332-ced42c/`.
- Repaired: `artifacts/pacaroute-verification/live-appserver-20261103-220821-282bf7/`.
- Signed package: `artifacts/pacaroute-verification/live-appserver-20261103-220953-ef59b8/`.
- Claude native: `artifacts/pacaroute-verification/live-claude-native-20261103-220821-a46fc7/`.
- Claude cross: `artifacts/pacaroute-verification/live-claude-cross-20261103-220821-f1a314/`.
- Verdicts and rollback bundles: `artifacts/message-identity-fix/`.

Installed application: `/Users/lando/Applications/PacaRoute.app`.
Signed executable SHA-256:
`04e3e99d5e0b5de8c540bb459cc06a9dbbbc074275d024a6f93d80ab8adeca1d`.

The installed package passed `codesign --verify --deep --strict`. Installation
preserved rollback bundles. After the user's explicit restart authorization,
PID 12596 exited gracefully and PID 43313 launched from the verified installed
bundle on port 8765. The health endpoint reported ready.

A live probe through the restarted 8765 service selected Copilot and returned
HTTP 200, exactly one completed response, and the expected `message-id-ok` text.
All nine message identity references used the same added ID. Evidence:
`artifacts/message-identity-fix/installed-live-probe.json`.
