# Select multiple models per Agent and report per-model usage

## Problem
Copilot model discovery hid native Sonnet/Haiku and all models outside reviewed
Responses profiles. A wildcard Client route silently fixed Sonnet requests to
Opus. The UI could not select multiple models without advanced rules. Usage
aggregated sources without showing the actual model that consumed tokens.

## Change
- Show every enabled upstream catalog entry; separate compatibility flags from
  visibility. Keep unsupported entries disabled in the multi-select and retain
  the Agent-compatible subset for legacy client model discovery.
- Client × Agent can select a source, multiple allowed models and a connection
  default. Persist exact routes and observed Copilot aliases; no wildcard in
  this mode. Preserve advanced rules and explicit fixed-model/pool behavior.
- Hide duplicate observed aliases in generated script options for simple selections.
- Extend the incremental hourly usage index with source/Client/model dimensions.
  Expose source_models alongside source totals, with the same filters and missing
  usage accounting. Display source totals plus model rows, and cache after input.

## Verification
Catalog and routing contracts, source-model reconciliation across filters and
restarts, multi-select/script-default roundtrips and mutation checks. Verify the
packaged runtime with actual Claude Code Sonnet and Opus tool execution and a
Codex app-server task/continuation, then inspect this run's own evidence.
Keep private captures and credential material outside commits.
