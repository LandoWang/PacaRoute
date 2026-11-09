# Copilot Argon via native Go conversion

PacaRoute routes the exact `gemini-4-argon` model on a Copilot source to
`/chat/completions`. Codex keeps its Responses interface and Claude Code keeps
Messages. The existing Copilot account, Auth lease, routing pools and quota
accounting are used. There is no LiteLLM process, Vertex API key or new Source kind.
Other Copilot models retain their existing routing.

## Wire evidence

A live 2026-11-03 probe using the existing account confirmed:

- The model is enabled and advertises only `/chat/completions`.
- A function call followed by its result succeeds. Assistant state contains
  `reasoning_opaque`, which must be replayed as opaque provider data.
- A streamed continuation emits content, a finish chunk with opaque state and
  final usage, then `[DONE]`. Initial usage can be all zeros.
- Catalog efforts are low, medium and high. Live clients exercise low (Codex)
  and high (Claude). Other effort values are forwarded, never guessed/clamped.

The local probe requests, responses and catalog are in `artifacts/argon-probe/`.
They are not repository fixtures or credentials. The existing protocol research
in `docs/copilot-api-research.md` identifies Copilot's Chat endpoint and headers;
this adapter uses the existing Auth implementation for those headers.

## Conversation fidelity

The adapter retains namespace/function identities and wraps Codex custom-tool
input in a JSON function schema, restoring it as a custom call on the way back.
Text and image inputs are converted; non-text tool results, provider-managed
web/search tools and unsupported input items fail explicitly. Tool schema fields
named `cache_control` are preserved; only actual protocol metadata is omitted.
Mid-conversation Messages system instructions retain their position.

Opaque assistant metadata is AES-GCM encrypted into a versioned replay capsule.
Responses carries it as a reasoning item's `encrypted_content`; Messages carries
it in `redacted_thinking.data`. The installation key and authenticated route scope
bind the capsule to Client, Agent, session, source/account/endpoint, pool and model.
It survives gateway restarts; moving history to another scope fails explicitly.
The capsule travels in the client's history, so no unbounded server-side history
cache is needed. Keep the installation data when continuing existing sessions.

Codex text deltas are forwarded as they arrive. Tool calls become executable only
after a valid finish reason and `[DONE]`; length-truncated calls never execute.
Claude content is buffered through the terminal chunk so opaque state can precede
the final answer. Emitting opaque state after answer text makes the tested Claude
CLI produce an empty final `result`, despite displaying the text earlier. The
adapter preserves both state and a complete final answer at this latency cost.
Response buffering is capped at 32 MiB and tool arguments/replay state at 8 MiB.

Final usage replaces interim snapshots. Responses input includes cached tokens;
Messages uncached input subtracts cache reads. Activity/quota accounting receives
Chat's original total input, with no cache double-counting. Missing usage is null.
Upstream HTTP errors retain their status/body. Interrupted streams fail without
replaying tools or switching upstreams after output has started.

## Use

In PacaRoute, use the existing GitHub Copilot source. Add a Client route or pool
member targeting `gemini-4-argon` for Codex and/or Claude Code. Codex may request
the exact ID; Claude can keep its configured client model alias while the route
selects Argon. Save through the existing revisioned configuration UI/API.

This is exact-ID inference support. It does not add a fabricated Codex desktop
model metadata resource. Top-level Codex tool declarations retain namespace and custom-tool dispatch identities. Desktop input registrations are handled by a separate compatibility change. Native MCP execution and non-text
tool-result compatibility are not claimed by this verification. Chat has no supported `messages/count_tokens` mapping;
that endpoint returns 422 rather than a fabricated estimate. Existing Responses
`previous_response_id` and `item_reference` cannot cross to Chat; clients must
replay their history.

## Validation

Contract tests cover opaque replay, scope isolation, restart, cached usage,
namespace/custom tools, parallel/fragmented calls, system-message order,
length limits and missing/error terminals. Mutation tests verify that removing
scope binding or accepting premature EOF causes the corresponding tests to fail.
Real-client evidence and exact binary hashes are recorded in local manifests;
see `artifacts/argon-final-verdict.json` for the final run verdicts.

Final verification on 2026-11-03 used the existing Copilot account through Auth,
isolated QA listeners and client homes, and the same immutable Go binary for
both clients. Captured upstream requests target `gemini-4-argon`.

| Check | Result |
| --- | --- |
| `go test -race ./...` and `go vet ./...` | PASS |
| Mutation checks: remove replay scope; accept EOF without `[DONE]` | Both detected |
| Codex app-server: multi-tool task, gateway restart, user continuation | PASS: 6 matched wire tool roundtrips, 2 completed turns |
| Codex isolated dispatch database | 250 rows, 0 router fatals |
| Claude Code: translated Messages multi-tool task | PASS: 4 matched wire and transcript tool roundtrips, complete final answer |
| Binary hash unchanged throughout both live runs | PASS |

The local binary is `artifacts/pacaroute-argon-final`, SHA-256
`922c2ac5b6f3517c5d942e9a688010a4a0564e004bda715754e97a7076da1b61`.
Its manifests under `artifacts/pacaroute-verification/` are:

- `live-appserver-20261103-213522-6cacad/appserver.manifest.json`
- `live-claude-cross-20261103-213522-183b8e/manifest.json`

These are local development evidence, not a released desktop package. The
production listener, installed binary and user routing configuration were not
replaced by this verification.
