# Translate Codex desktop tool registrations

Codex sends tools in an `input` item of type `additional_tools`. Native Responses
already preserves this item, but the Go cross-protocol translator rejected it.
Consequently Codex routes to Copilot Argon or Opus failed before inference.

Register those declarations through the existing function/custom/namespace tool
translator for both Messages and Chat targets. Preserve top-level declarations,
schemas, namespaced identities and raw custom tool inputs/results; never turn a
registration into developer prose. Native Responses stays unchanged.

The same incident exposed an independent Copilot `gpt-5.3-codex` constraint:
paired live probes reject `reasoning.context: all_turns` and accept `auto` and
`current_turn`. Normalize only that exact model's unsupported value to `auto`.
This delegates context policy to the upstream rather than promising all-turn
reasoning support. Preserve other models, providers and unknown fields.

No routing, credential or UI configuration changes are required.
