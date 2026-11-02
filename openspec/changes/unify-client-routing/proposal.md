# Unify Client routing and connection scripts

## Problem
Clients and Routes split one setup workflow across two pages. Connection scripts
also ask callers to obtain and paste a key separately.

## Change
Keep one Client page with per-Agent source/pool/model controls, advanced rules,
and connection scripts. Redirect legacy Routes links to Clients. Save newly
created/rotated Client keys encrypted through Auth, separately from source keys.
Administrators can recover the committed version or import a matching legacy key.
Scripts include the key, pass it through stdin, and redact the browser preview.
Local default remains keyless; no legacy key is rotated implicitly.

## Contracts
Configuration remains revisioned and contains only key hashes. Invalid/duplicate
creates do not change existing credentials. Failed commits leave an unreachable
credential version, not an overwritten active key. Admin authentication is
required to read/import keys, responses are not cacheable, and stored keys must
match the active hash. Removal makes retrieval unavailable; old credential files
remain encrypted and inaccessible through the API. In-flight requests and routing
semantics are unchanged.
