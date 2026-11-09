## Why

The Go native SSE relay lost the message identity correction already specified by
`codex-response-fidelity` and implemented in the legacy C# adapter. Copilot emits
rotating opaque IDs for the same output slot, so Codex retains the streamed item
and its completed snapshot as two visible messages. A real desktop-bundled Codex
app-server baseline reproduced four completed messages with no matching start ID.

## What Changes

- Restore a per-stream canonical ID for native Copilot Responses message items,
  anchored to `response.output_item.added` and its `output_index`.
- Preserve event order, framing metadata, unknown fields, usage, phases, and all
  non-message IDs. Keep raw upstream traces truthful and other providers unchanged.
- Reuse the sanitized historical capture as a Go regression, cover terminal and
  multiple-item behavior, and assert lifecycle identity from real Codex events.

## Scope

No routing, model catalog, authentication or UI changes. Existing local Gemini
work remains intact. Testing uses isolated ports, client homes and dispatch logs.
