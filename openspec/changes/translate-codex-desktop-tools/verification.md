# Desktop protocol compatibility verification — 2026-11-04 UTC

## Root causes and repair

`ResponsesToMessages` accepted top-level tool declarations but rejected the
desktop `input[].type: additional_tools` item. Both Codex-to-Opus (Messages) and
Codex-to-Argon (Chat through the same intermediate translator) failed locally.
The repair registers the item's tools through the existing function/custom/
namespace mapping. Tools remain structured declarations, with schemas and call
identities retained, and never become developer prose. Native Responses input
remains opaque. Invalid arrays and translated-name collisions fail explicitly.

A separate paired probe of Copilot `gpt-5.3-codex` returned HTTP 400 for
`reasoning.context: all_turns`, naming `auto` and `current_turn` as supported.
Otherwise identical requests with either supported value returned 200/completed.
Only this exact Copilot Responses model now maps `all_turns` to `auto`; the
upstream selects its supported context policy. This is not equivalent to a
promise of all-turn reasoning. Other models/providers and unrelated fields
remain unchanged. Probe evidence: `artifacts/additional-tools-fix/context-probe/`.

## Contract checks

- Replay the committed desktop registration capture on native Responses,
  Messages and Chat, preserving every tool and native input value.
- Preserve namespaced function schemas and custom `functions.exec` JavaScript
  through registration, upstream call, restored custom call and client result.
- Preserve top-level declarations alongside additional registrations; reject
  invalid arrays, unsupported tools and name collisions.
- Scope the context coercion to the exact model and provider; retain supported,
  unknown and null values, reasoning siblings and the original failover body.
- Both overlay mutations (disable registration handling; disable context
  normalization) fail their contract tests.
- `go test -race ./...`, `go vet ./...` and `git diff --check` pass.

## Signed package: real-client evidence

The modern bundled Codex app-server naturally emits the desktop registration,
including `functions.exec` with its custom grammar. Final runs use the actual
client request without an injection proxy. Each Codex run completes a multi-tool
task, restarts its isolated QA gateway, and completes a user continuation.
Dispatch evidence comes from each run's own `logs_2.sqlite` and matching wire
calls/results. The signed package's native GPT run also passes the previous
message identity verifier: four starts/completions, no mismatches/duplicates,
and each final canary exactly once.

| Route | Matched tool round trips | Dispatch rows | Router fatals | Result |
| --- | ---: | ---: | ---: | --- |
| Codex to gemini-4-argon | 5 | 273 | 0 | PASS, two turns |
| Codex to claude-opus-5.5 | 6 | 200 | 0 | PASS, two turns |
| Codex to gpt-5.3-codex | 9 | 339 | 0 | PASS, two turns |
| Codex to gpt-6.1-sol | 6 | 291 | 0 | PASS, two turns |
| Claude native Opus 5.5 | 4 | N/A | N/A | PASS, complete answer |
| Claude to GPT Astra | 4 | N/A | N/A | PASS, complete answer |

Argon and Opus retain `namespace: functions` on all custom exec calls; the
upstream receives `functions__exec`. Every GPT-5.3 client request carries
`context: all_turns` and every corresponding Copilot request carries `auto`.
Local evidence: `artifacts/additional-tools-fix/signed-final-verdict.json`,
`path-exercise.json` and `signed-sol-identity.json`.

Exact manifests under `artifacts/pacaroute-verification/`:

- `live-appserver-20261103-234846-a91e99/appserver.manifest.json` (Argon).
- `live-appserver-20261103-234846-48ac67/appserver.manifest.json` (Opus).
- `live-appserver-20261103-234846-ffa986/appserver.manifest.json` (GPT-5.3).
- `live-appserver-20261103-234749-d87fbf/appserver.manifest.json` (Sol).
- `live-claude-native-20261103-234749-4d4fef/manifest.json`.
- `live-claude-cross-20261103-234749-2ee91f/manifest.json`.

## Installation

The verified package is installed at `/Users/lando/Applications/PacaRoute.app`.
Executable SHA-256:
`3e6fb6ce52363cb7d182061e519b9280a220522c22abf68abdf0b51f9d6c7d1f`.
The app passes `codesign --verify --deep --strict`; the prior bundle remains at
`artifacts/additional-tools-fix/PacaRoute-before-desktop-tools.app` for rollback.

Using the user's existing explicit restart authorization, the identified old
PID 43313 exited gracefully and PID 85542 started from the installed bundle on
8765. Readiness is true. User configuration revision remained 60 throughout;
no routes or credentials were changed. `installation.json` and `activation.json`
in the evidence directory record the package and activation details.

After activation, the existing production route returned the exact live canary
with HTTP 200 and one completed response. All 13 message identity references
used the same message ID. Evidence: `installed-live-probe.json` in that directory.

The tests cover model-specific conversations and continuations. Replaying a
foreign provider's encrypted reasoning into Argon remains intentionally rejected;
this repair does not claim cross-provider reasoning migration or native MCP
execution compatibility.


## Merge verification — 2026-11-08 (America/Los_Angeles)

Revalidated the reviewed source against private main `5fd1b2f` before merging
message identity, Copilot Argon, and desktop tool/context compatibility. The
runtime source is unchanged from the reviewed snapshot. The refreshed embedded
web assets reproduce exactly from the lockfile.

- Go race tests, vet and source diff whitespace checks pass. Generated Vite
  bundles retain upstream license-comment whitespace and are excluded from the
  source whitespace check.
- Web: 58 tests pass, one is skipped; TypeScript/Vite and embedding pass.
- macOS arm64/amd64 desktop packages are built and ad-hoc signature verification
  passes. Windows amd64 desktop and five CLI targets (macOS arm64/amd64,
  Linux arm64/amd64 and Windows amd64) build successfully. Cross-built targets
  were not executed on their native operating systems.
- Five overlay mutations are caught: missing message-ID wiring, missing replay
  scope, accepting premature Chat EOF, dropping desktop registrations and
  disabling the exact-model reasoning context correction.

Fresh real-client runs use one immutable CLI binary, isolated service ports,
client homes, workspaces and dispatch databases. Every Codex route completes
both a multi-tool task and a continuation after its QA gateway restarts. Every
selected Codex dispatch database has zero router fatals, and stdout has no abort
markers. Claude transcripts contain matched tool results and complete answers.

| Route | Matched tool round trips | Dispatch rows | Result |
| --- | ---: | ---: | --- |
| codex-argon | 5 | 394 | PASS |
| codex-opus | 6 | 238 | PASS |
| codex-gpt53 | 9 | 502 | PASS |
| codex-sol | 6 | 307 | PASS |
| claude-native | 4 | N/A | PASS |
| claude-gpt | 4 | N/A | PASS |
| claude-argon | 4 | N/A | PASS |

Argon and Opus requests naturally carry desktop `additional_tools` registrations,
with `functions` namespace custom calls preserved. GPT-5.3 is exercised using the
modern desktop client model: all captured client contexts are `all_turns` and
all requests to the actual GPT-5.3 upstream carry `auto`. The first run using the
older GPT-5.3 client profile lacked this field and was replaced by the selected
path-exercising run below. Native Sol retains `all_turns` and passes message
identity checks: four starts/completions, no unmatched or duplicate events, and
exactly one final answer per turn.

Verified CLI SHA-256: `e8c3fe9681932a89b0cc821b85cf47975aae91c518e5b010d496c8d976be1d32`.
Evidence summaries remain local in `artifacts/merge-review/`: `live-verdict.json`,
`path-exercise.json`, `message-identity.json` and `mutations/verdict.json`.
Selected manifests:

- `artifacts/pacaroute-verification/live-appserver-20261108-183015-2cef2f/appserver.manifest.json`.
- `artifacts/pacaroute-verification/live-appserver-20261108-183015-00fbd8/appserver.manifest.json`.
- `artifacts/pacaroute-verification/live-appserver-20261108-183237-d5ca06/appserver.manifest.json`.
- `artifacts/pacaroute-verification/live-appserver-20261108-183037-20bcfc/appserver.manifest.json`.
- `artifacts/pacaroute-verification/live-claude-native-20261108-183040-e4bedd/manifest.json`.
- `artifacts/pacaroute-verification/live-claude-cross-20261108-183106-145d6b/manifest.json`.
- `artifacts/pacaroute-verification/live-claude-cross-20261108-183110-49c7ba/manifest.json`.

This merge does not install a package or replace the production listener.

### Known baseline audit gaps

Seven existing audit repros were adapted to the current package boundaries with
an isolated Go overlay. Forwarded local-Client authentication passes; six other
contracts still fail. Their implementation files are byte-identical to private
main `5fd1b2f`, so these remain separate follow-up fixes:

- Reject forwarded requests at the local management boundary.
- Authenticate the selected local management endpoint before disclosing its key.
- Preserve missing usage through streaming protocol translation.
- Emit a translated tool completion only once for a repeated upstream stop.
- Reject a completed stream with an unfinished tool call.
- Preserve large integer values when editing unrelated Claude JSON settings.

No audit fix, legacy .NET work, credential record or raw trace is part of this merge.
