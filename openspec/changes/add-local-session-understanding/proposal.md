# Local session understanding

AgentLink currently manages client defaults and Bridge traffic but cannot explain a
client's existing conversation. Add a read-only Sessions page for local Codex and
Claude Code histories, independent of Bridge availability. Show recorded messages,
tools, errors and model/provider evidence. Offer explicitly requested analysis of
the selected, bounded history through a chosen existing source and model.

Do not resume clients, execute historical instructions, mutate client stores, or
infer an actual upstream source from the current client configuration. No new daemon
or credentials are required. Analysis is separate from browsing and clearly identifies
its source, model and coverage. Keep the existing separate macOS architecture packages.
