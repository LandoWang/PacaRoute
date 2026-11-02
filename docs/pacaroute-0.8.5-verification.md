# PacaRoute 0.8.5 verification

Verified on macOS arm64 on 2026-11-02. The packaged desktop binary SHA-256 is
`dbebc13eff3e894584aef525821a17b82b1c04474eff324eb4f2d988d97beaee`.
Private traces and credentials are not committed.

## Model selection and discovery

- The live Copilot account returned 42 enabled model entries; 0.8.4 had exposed
  only 14 after its compatibility filter. Management now retains all enabled
  entries and reports Agent compatibility separately. Disabled account-policy
  entries remain excluded.
- Browser checks confirmed 42 source-catalog rows, checkbox selection and save,
  persistence of three Claude models, Opus as the connection default, and one
  script option per canonical model. Selecting Sonnet generated its exact model
  in the connection command without duplicate dotted/hyphenated aliases.
- The running Client configuration kept its independent Codex pool. An unselected
  synthetic model received HTTP 403 before an upstream call.
- Advanced aliases, multi-source rules and fixed-model routes remain explicit.

## Real-client evidence from the packaged binary

| Path | Local manifest suffix | Wire tool roundtrips | Client evidence |
| --- | --- | ---: | --- |
| Claude Code `sonnet` → Copilot `claude-sonnet-5.5` | `live-claude-native-20261102-020133-d0f27b/manifest.json` | 4 | 4 transcript pairs, complete final answer, exact upstream model |
| Claude Code → Copilot `claude-opus-5.5` | `live-claude-native-20261102-020133-786739/manifest.json` | 4 | 4 transcript pairs, complete final answer, exact upstream model |
| Codex app-server → Copilot `gpt-6-astra`, pool restart and continuation | `live-appserver-20261102-020134-3bb671/appserver.manifest.json` | 13 | two completed turns; 1,000 bounded SQLite rows inspected; zero router fatals |

The verdict tool checked the binary hash against each manifest and required the
actual upstream model on Claude runs. Sonnet used an exact `claude-sonnet-5-5`
rule, not an Opus or wildcard fallback. These observations do not verify every
catalog model or promise Chat Completions compatibility.

## Usage and regression checks

- Source/model totals reconcile to source and overall totals under the same
  source/Client/hour filters. Tests cover missing usage, failures, legacy entries
  without a model, repeated refresh, appended records and restart reconstruction.
- Browser checks confirmed nested model rows and input → cached → output ordering.
  Live historic model totals reconcile with existing overall counters.
- `go test -race ./...`, `go vet ./...`, 16 frontend tests and TypeScript/Vite
  production build passed. Mutations that dropped the model index dimension,
  restored the Opus-only filter, or substituted wildcard routes were detected.
- Separate Apple Silicon and Intel macOS bundles, Windows x64 portable packages,
  platform CLI archives and embedded WebUI were built. Physical Intel/Windows
  tray behavior was not tested; macOS signing remains ad-hoc, without notarization.

## Gemini 4 Argon exploratory upstream probe

The account catalog advertised `gemini-4-argon` as enabled with only
`/chat/completions`, tool calls, streaming and low/medium/high reasoning effort.
Three direct upstream Chat Completions requests through Auth succeeded: a plain
answer, a forced `get_probe_number` function call, and a matching tool result
containing 37 followed by the expected final result 74. The assistant message's
opaque `reasoning_opaque` field was preserved between tool-call and continuation.
No credential or raw opaque state is committed. This proves account access and a
basic Chat Completions tool roundtrip only; Codex/Claude Code integration remains
UNVERIFIED and requires a separate Chat Completions adapter. The existing Agent
routes were not changed to this model.
