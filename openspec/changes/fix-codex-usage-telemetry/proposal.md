# Record Codex token usage in desktop telemetry

Copilot Codex requests produce request-count events with null token fields because
its endpoint never populates the request summary's usage snapshot. Observe the
outgoing Responses usage in buffered responses and terminal stream events before
summary emission, independently of tracing. Preserve the response bytes, cumulative
counter semantics and unknown values; never add cached input to total input.
Historical missing counters cannot be reconstructed from request counts.
