# Design

Native Swift adapters use the system SQLite library in read-only mode for Codex
state/history databases and bounded JSONL reads for legacy Codex and Claude Code.
Directory roots are configurable (defaulting to CODEX_HOME / CLAUDE_CONFIG_DIR or
the standard home directories). Database columns are discovered before queries;
unsupported schemas produce explicit errors rather than an empty success.

Catalogs and content are loaded on demand off the main actor. Codex item pagination
uses rollout ordinals; JSONL pagination uses byte offsets and tolerates a trailing
incomplete append. Exclude reasoning and internal API request copies from the visible
timeline. Keep tool results distinct from human messages. Redact common credential
patterns before display or analysis and bound large tool outputs. Preserve unknown
provider attribution, partial coverage and recorded (not live) status in the UI.

Model analysis uses a stateless, tool-free Responses request to the selected source's
existing loopback Bridge route. The user sees the destination and a request preview
before clicking Analyze. A bounded transcript is supplied as untrusted evidence with
item references; the model must distinguish recorded conclusions from inference and
cite those references. Do not fetch models, access Keychain or send history simply
by opening the page. No analysis is persisted to the original client stores.

This feature does not promise live process control, exact historical upstream-source
attribution, per-source quota reconciliation, or automatic full-history AI indexing.
