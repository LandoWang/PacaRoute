# Reduce desktop activity refresh CPU

Live sampling of build 14 shows AgentLink spending sustained CPU decoding the
current daily telemetry file. Every append invalidates the whole-file cache;
per-record ISO date formatter creation and repeated front deletion of Data add
cost, while timer/manual refreshes can overlap and unchanged polls redraw the UI.

Tail logs serially from saved byte offsets, preserve incomplete lines, reuse date
parsers, and publish only changed records. Retain all existing source, time, token
and unknown-usage semantics. Reset on truncation/replacement, remove deleted logs,
and retain the existing 366-file history window. Do not restart the user's live
Bridge during diagnosis or validation.

Persist the parsed records and partial-line checkpoints in a local SQLite index.
Restore it across app restarts and rebuild it from original logs on corruption.
