# Verification

- Go: `go test -race ./...` and `go vet ./...` passed.
- Web: 26 tests passed; TypeScript/Vite build and embedded assets passed.
- All 17 mutation cases were caught, including Client-scoped peer discovery,
  derived session isolation, separate listener surfaces, known Agent grants,
  legacy local authentication, quota admission and actual-model pricing.
- Service contracts cover immediate admission stop, stream draining, config edits
  while stopped, retained Auth/router/usage/cooldowns, port conflicts and durable
  serving preferences.
- QR tests perform actual QR encoding/pixel decoding, Unicode round trips and
  strict endpoint/payload validation.
- Browser checks used only synthetic Clients and isolated service homes. They
  covered Client QR download, importing a QR screenshot, reviewing the account,
  discovering only granted model aliases, saving the peer source and using
  management while inference was disabled. Screenshots are under the ignored
  `artifacts/peer-service-ui/` directory.

## Latest package: Client Agents and quota ledger

The final macOS arm64 binary SHA-256 is
`68b47a49b961016cea7eab01857846da5f78fe6e1549531015899d6c95dcad08`.
Packages are under `artifacts/pacaroute-0.9.0-quota/`.
The explicit evidence verdict is
`artifacts/client-agent-ui/quota-real-client-verdict.json`.

- Codex app-server: `live-appserver-20261102-182253-7f4268/appserver.manifest.json`;
  two completed turns, six matching wire tool round trips, six peer-hop round
  trips, 1,000 isolated dispatch rows, zero router fatals, eight priced requests.
- Claude native: `live-claude-native-20261102-182253-9cbb80/manifest.json`;
  four matching transcript/wire tool pairs, complete answer, actual
  `claude-opus-5.5`, four peer-hop round trips, five priced requests.
- Claude-to-GPT: `live-claude-cross-20261102-182253-2f752a/manifest.json`;
  five matching transcript/wire tool pairs, complete answer, actual
  `gpt-6-astra`, five peer-hop round trips, six priced requests.

Each manifest is under `artifacts/pacaroute-verification/`. All three verdicts
passed. Every accepted request finalized with reserve/settle/release events;
synthetic USD 1/million rates produced exact expected integer ledger amounts.
Those synthetic rates and generous limits were confined to isolated QA homes.
No production model prices or Client limits were invented.

Contract checks cover atomic concurrent admission (five accepted out of 32),
restart persistence, duplicate settlement, unknown usage/prices, actual-target
pricing and rejection before an upstream call. Browser QA saved a synthetic
Client's 24-hour limits and a source's input/cache/output/cache-write rates,
then reopened both views. Screenshots are in `artifacts/client-agent-ui/`.

The verified arm64 app was installed at `/Users/lando/Applications/PacaRoute.app`.
Management is available on 63653, with serving disabled. AgentLink remains on
8765 and the earlier PacaRoute remains on 63652; client configuration was not
switched. A prior migration produced an upstream 400 whose cause remains
unresolved, so the active conversation was not moved again. The installed app,
settings and metadata were backed up before replacement.

## Earlier real-client release

The macOS arm64 app binary SHA-256 is
`f37fc1e6d4de95b5cf88b92ff2472a16e459bcaa7bd95244a5ed49ebc848b617`.
The real-client actuators exercised configuration export/import, multi-model
pool selection, an extra PacaRoute hop and a separate management listener.
Codex additionally exercised a bridge restart and service stop/start before
continuation. Each run retained the original upstream Auth authority; no
refresh-bearing credential was copied.

- Codex app-server: two completed turns, ten matching wire tool round trips,
  1,000 isolated dispatch rows, zero router fatals, ten peer-hop round trips.
- Claude native (`claude-opus-5.5`): four matching tool pairs, complete answer,
  four peer-hop round trips.
- Claude-to-GPT (`gpt-6-astra`): four matching tool pairs, complete answer,
  four peer-hop round trips.
- All successful peer requests recorded input/output usage.

Initial concurrent QA runs used the same temporary Keychain account ID and
encountered duplicate-item errors. The peer actuator now isolates its identities
and injects only its static QA Client Key through Auth's existing stdin lease
mechanism. This avoids QA writes to the user's Keychain. Provider refresh tokens
remain untouched.

Per-run manifests and the verdict JSON remain local under
`artifacts/pacaroute-verification/` and `artifacts/peer-service-ui/`.

## AgentLink authentication compatibility

The installed migration build has SHA-256
`6c107706533b33ff3fcf0ad9efe6f6bfcb9ee05bd0498a22e61ab251ef51c47a`.
AgentLink's public `copilot-bridge-provider` sentinel is accepted only through
the explicitly enabled loopback Client path. Network callers, forwarded requests,
public service URLs and disabled or deleted local Clients remain rejected.

Fresh runs against this installed binary used isolated service ports and client
homes, with the existing Auth authority referenced in place:

- Codex app-server used command-backed legacy authentication, completed two turns
  across a bridge restart, and produced six matching wire tool round trips.
  Its 1,000 isolated dispatch rows contained zero router fatals; stdout contained
  no execution-abort markers. The generated result was checked independently.
- Claude Code completed four matching wire and transcript tool round trips on
  `claude-opus-5.5`, with a complete final answer and checked result file.
- The selected manifests and binary hash checks passed in
  `artifacts/peer-service-ui/resumed-migration-verdict.json`.

These runs verify the installed binary and legacy authentication path. They do
not establish ownership or readiness of a separately controlled production port.

## AgentLink migration regression

Live migration exposed an existing Codex provider helper that emits the public
`copilot-bridge-provider` sentinel. It is now accepted under the same explicit
loopback/local-Client gate as other local placeholders. Network callers, forwarded
requests, disabled/deleted Clients and public services still cannot use it.

The final migration binary SHA-256 is
`6c107706533b33ff3fcf0ad9efe6f6bfcb9ee05bd0498a22e61ab251ef51c47a`.
The real Codex app-server ran the legacy provider auth-command configuration:
two complete turns, seven matched wire tool round trips, 1,000 isolated dispatch
rows and zero router fatals. The production AgentLink helper was also used against
the installed application and completed a live Astra response with token usage.
The local-auth change has a boundary regression test and a mutation that removes
sentinel compatibility. Both the race suite and vet passed after this change.
Earlier peer/client evidence above refers to the preceding release binary; the
migration package adds only this local-auth compatibility fix.

## Distribution and limits

Separate macOS arm64 and amd64 DMGs and a Windows x64 ZIP were built in
`artifacts/pacaroute-0.9.0-quota/`. macOS bundles are ad-hoc signed, not notarized.
Intel and Windows builds were cross-compiled but not run on physical machines.
Camera scanning is implemented with explicit browser permission and cleanup;
physical-camera validation was not performed. Image scanning was verified.
Production service replacement is a separate approved operation; verification
never stops unrelated listeners or binds port 8765.

## Navigation-only follow-up

Moved Proxies immediately before Clients and replaced the Client navigation mark
with the vendored Codex SVG. Frontend type checking, production build and all 26
web tests passed. The installed arm64 desktop was rebuilt and signed; its SHA-256
is `5752983ea278da607c121c799b827413eaf73dfd6cd98751efc2f8de16ed3dda`. Browser inspection at localhost:63653 confirmed both navigation
order and the active icon. Screenshot: `artifacts/navigation-ui/clients-sidebar.png`.
No routing or protocol code changed; the real-client evidence above still refers
to its recorded quota package. Service state and existing inference listeners were
preserved.
