# PacaRoute web and desktop architecture

PacaRoute 0.8 uses one React/TypeScript frontend and a Go management API. The desktop distribution adds a small native system-tray/menu-bar controller to the Go process; it opens the management interface in the default browser. It does not use Wails, Electron or a bundled browser. Closing the browser does not stop the gateway.

## Run and distribute

- macOS: open `PacaRoute.app`. The alpaca appears in the menu bar. Apple Silicon and Intel bundles are separate; macOS 14 or newer is required.
- Windows: extract the portable ZIP and open `PacaRoute.exe`. The alpaca appears in the notification area (possibly inside the overflow menu). `pacaroute-cli.exe` provides the console commands. An Inno Setup installer definition is included in `scripts/pacaroute-windows.iss`.
- Server: run `pacaroute serve --bind 0.0.0.0 --port 8766 --public-url https://gateway.example`. Put TLS termination in front of the listener.
- Local command line: `pacaroute serve --port 8766 --allow-local --local-management`, then `pacaroute ui --port 8766`.

The tray offers Open management, Start/Stop service, Login startup and Quit. Desktop mode owns one gateway in the same process, binds loopback and enables local Agent connections. Stop closes its listener and allows active requests up to 30 seconds to finish. Open management starts a stopped service. An occupied port is reported without terminating another process. Port edits in Service settings establish the replacement listener first; if binding fails, the current listener stays alive. Successful edits persist and reconnect the browser with a fresh one-use login ticket.

Desktop configuration defaults to port 8766. The existing macOS `Copilot Bridge` data directory is reused when present; Windows uses `%LOCALAPPDATA%\PacaRoute`. `--home` overrides metadata; `--credential-root` can reference an existing credential authority without copying credentials. Quit the old native AgentLink/PacaRoute before sharing its data directory with desktop mode. For trials, use an independent `--home` and port. Existing Agent configuration is never changed simply by launching the app.

macOS API keys in the desktop binary (including its CLI commands) use the existing Keychain service. Windows vault writes use current-user DPAPI; historical portable encrypted records remain readable. CLI/server vault storage on Unix retains its previous authenticated encryption format. Refresh-bearing records and their locks remain under Auth, never under the web UI or tray code. Desktop sign-in opens a URL containing a random one-use ticket in the fragment, removes it immediately and exchanges it for an HttpOnly, SameSite cookie. The persistent management key is not passed in URLs or stored in browser localStorage.

## Accounts and resources

Account cards use provider marks; source cards use product marks (including GitHub Copilot and Azure Foundry). Icons are bundled locally. Accounts are provider identities: **GitHub, OpenAI, Anthropic, Azure**. The account ID is independent of every resource/source ID.

| Account provider | Resource/source product | Authentication |
| --- | --- | --- |
| GitHub | GitHub Copilot | Device Login |
| OpenAI | OpenAI API / compatible resource | Account API key; optional source override |
| Anthropic | Anthropic API | Account API key; optional source override |
| Azure | Foundry / Azure OpenAI resource | Azure CLI identity or resource API key |

The default account form asks only for a provider and, for OpenAI/Anthropic, an API key. GitHub offers built-in device authorization or a custom OAuth App client ID. Azure offers shared CLI identity (default) or isolated device authorization. Names and stable IDs are generated automatically; optional labels and IDs are under Advanced. Add source starts with account selection, then infers the resource type. Azure picks the default subscription when available and discovers resources/deployments. API resources use official endpoints and discover models automatically. Custom endpoints, credentials and manual model IDs remain available under Advanced; source rows expose a compact Proxy selector; discovering models never silently rewrites saved routes.

An OpenAI account cannot be attached to an Anthropic source. Legacy `api-key` account records remain compatible but are no longer the default new-account choice. GitHub Device Login authenticates the account; use Add source to connect its Copilot resource. The legacy default Copilot source remains available. OpenAI and Anthropic account keys are set once on the account; their sources inherit that key unless a resource-specific credential overrides it. Key rotation invalidates leases without restarting the service. CLI equivalents are `pacaroute key set --account ACCOUNT` and `pacaroute key set --source SOURCE`, both reading the secret from stdin.

Azure Accounts exposes subscription discovery, resource discovery and deployment selection. The resulting source retains `subscription_id` and `resource_id`. Existing sources can rediscover deployments and explicitly update their configured model list. A source without a resource ID can match its endpoint within its saved subscription. Discovery requires an Azure CLI login with resource read permission; an inference API key alone does not grant management-plane deployment discovery. OpenAI and Anthropic resources can discover the model catalog through their authenticated API. A discovered model ID does not guarantee every Agent/tool capability; only configured selections are exposed for routing.

## Clients, routes and configuration

Navigation is Overview, Accounts, Sources, Pools, Clients, Usage, Proxy,
Service settings and Errors. There is no separate Agent or session-history page.
Session affinity remains a router capability; no local chat history is read.

Fresh installations contain a local-only `default` Client with Codex and Claude
Code enabled and no routes. Choose a target per enabled Agent directly in its Client card, then
open the adjacent connection script. Remote Clients receive their own keys.
An explicit empty `enabled_agents` list denies all Agent access; omitted legacy
lists enable both. The gateway checks this before model listing and route lookup.
Deleting `default` permanently removes loopback keyless access, including across
restart. Public/forwarded requests cannot use that identity.

Client cards generate Bash or PowerShell scripts for `pacaroute connect`. These
run on the caller's computer, not the service host. macOS scripts also find an
installed `/Applications/PacaRoute.app`; Windows scripts find `pacaroute-cli.exe`
beside the script or on PATH. The CLI 0.8+ needs no running local service. It reads
the Client key from stdin, creates an exact backup, safely edits TOML/JSON and
preserves unrelated settings. The caller's config stores its Client key with
private file permissions. Downloaded/copied scripts include the Client key; previews
redact it. New and rotated Client keys are encrypted behind Auth in a separate
hash-versioned namespace. Administrators can import a legacy key after its hash is
validated; missing keys are never silently rotated. Restore
is available on the local default card or through the CLI. Source changes later
happen solely in Clients; advanced ordered model-alias rules remain supported.

Router configuration lives in `<home>/pacaroute.sqlite`: a schema-versioned,
revisioned document preserves atomic cross-entity invariants. Writes validate
and compare revisions in one SQLite commit. Requests use immutable memory
snapshots; other CLI/process writes are detected within one second. Back up the
DB with SQLite's backup API, or stop the service before copying it and its WAL.
Do not edit tables directly; use the management API or `config show|apply`.

On first use, split JSON (or a committed old recovery journal) imports once.
Original files are retained but are no longer live inputs after migration. An
imported local Client visibly retains legacy direct routing until an explicit
route edit replaces it; this compatibility mode cannot be re-enabled. If an old
network Client already owns ID `default`, its identity is preserved and no local
Client is added. Reverting to an older binary reads only its old JSON state.
Provider keys, refresh tokens and encrypted credential files remain behind Auth,
not in SQLite. Service port/startup preferences and usage journals remain separate.

Shared Azure login reads the existing `AZURE_CONFIG_DIR` (or `~/.azure`) without
copying or logging out that cache. Isolated login uses a separate account folder.
Built-in GitHub login persists authorization and refreshes Copilot exchange leases;
custom OAuth login rotates a refresh token only when GitHub issues one.

## Frontend development and separate hosting

```sh
# Terminal 1; choose an independent data directory during development.
go run ./cmd/pacaroute serve --home ./artifacts/dev-state --port 8766 --local-management --allow-local

# Terminal 2
cd apps/web
npm ci
PACAROUTE_API=http://127.0.0.1:8766 npm run dev
```

The Vite server binds `127.0.0.1` and proxies `/api` to the selected backend. It rewrites the development Origin to the backend origin; it does not enable broad backend CORS. Sign in with the development data directory's management key. `status.gateway_url` supplies Agent connection addresses, independently of the frontend development port.

`npm run build` produces `apps/web/dist`, a standalone static frontend. `npm run embed` copies that output to the Go embed directory; packaging invokes both. Node is a build-time dependency only. The browser has no direct dependency on Go structs or desktop bindings: `apps/web/src/api.ts` is its typed HTTP contract. `status.api_version` is currently 1. Components use React, Radix/shadcn-style primitives, Tailwind and Lucide.

For independently hosted static files, serve `dist` and proxy `/api` plus gateway paths (`/codex`, `/cc`, `/sources`, `/pools`, `/v1`) under the same public origin. Set `--public-url` to that exact origin. Cookie management intentionally requires same-origin requests. CLI and other management consumers can use the management bearer key. There is no requirement to run a frontend process in production.

## Management API

`Authorization: Bearer <admin.key>` or the authenticated HttpOnly cookie is required except for login. Cookie writes also require a matching Origin. Full configuration writes require the exact `revision`; conflicts return 409 and leave the previous snapshot unchanged.

| Method | Path | Purpose |
| --- | --- | --- |
| POST | `/api/login` / `/api/logout` | Browser management session |
| GET | `/api/status` | Version, capabilities, gateway address and lifecycle |
| GET / PUT | `/api/config` | Read or atomically replace metadata |
| POST | `/api/clients` | Create Client and persist its encrypted key |
| POST | `/api/clients/{id}/rotate` | Rotate Client key |
| GET / PUT | `/api/clients/{id}/key` | Recover current key / import a matching legacy key (admin only, no-store) |
| POST | `/api/sources/{id}/pause` | Hot pause/resume |
| PUT | `/api/accounts/{id}/key` | Save provider account key |
| PUT | `/api/keys/{source}` | Save source credential override |
| GET | `/api/models?source=ID` | Configured/compatible source catalog |
| POST | `/api/model-discovery` | Discover a draft source's models without saving it |
| GET | `/api/sources/{id}/discover-models` | Rediscover deployed/available models |
| GET / POST | `/api/accounts/{id}/…` | Status, login, quota, Azure subscriptions/resources/deployments |
| GET | `/api/usage?from=RFC3339&to=RFC3339&source=ID&client=ID` | Time, source and Client filtering |
| GET | `/api/activity` / `/api/errors` | Recent sanitized request records |

Local endpoints additionally require explicit local management, a loopback peer and loopback Host, and no public origin. Merely knowing the admin key does not expose host files remotely.

- `/api/local/agents`, `/configure`, `/backups`, `/restore`: local Agent configuration, fingerprinted preview/apply and exact-byte backups.
- `/api/local/service`, `/startup`: desktop-only listener/startup preferences.
- `/api/local/browser-ticket`, `/api/login/ticket`: authenticated desktop/browser handoff; one use and a 60-second lifetime.

Local features operate on the **service host**. They are unavailable in remote management mode. Client routes and pools have visual editors, including ordered matching, model aliases, weights and failover; no JSON editing is required. Usage remains based on upstream-reported tokens, including explicit missing-usage counts.

MCP is not part of this release. It can later adapt the same management capabilities without coupling the web UI to an Agent protocol. The older SwiftUI sources are retained for compatibility and unrelated ongoing work; the new desktop packages do not execute them.

## Build and verification

```sh
bash scripts/build-web.sh
PACAROUTE_WEB_READY=1 bash scripts/build-pacaroute-desktop.sh
PACAROUTE_WEB_READY=1 bash scripts/build-pacaroute.sh
go test -race ./...
go vet ./...
npm --prefix apps/web test
```

macOS bundles are ad-hoc signed, not notarized. Windows portable binaries are unsigned. Cross-compilation is not evidence of physical Intel or Windows tray behavior; record those separately. `.github/workflows/pacaroute.yml` supplies native Windows/macOS/Linux checks and a Windows installer build when run on the private repository.

See [0.8.6 stream recovery and cooldown verification](pacaroute-0.8.6-verification.md), [0.8.5 model selection verification](pacaroute-0.8.5-verification.md) and [0.8 release verification](pacaroute-router-workspace-verification.md) for exact client evidence and platform limits. The [0.7 report](pacaroute-web-desktop-verification.md) remains historical evidence.

### Usage and upstream quota

Local usage records only requests received by the running service's address and
data root. Agents connected to another bridge port or directly to a provider do
not appear here. Empty results show the current gateway address and a link to
Client setup. Copilot account quota is shown only in Accounts → account details;
it is fetched on opening and explicitly refreshed, not on the usage polling interval. Unlimited quotas are not shown as zero, and token-billed quota
credits are not labeled as inference tokens. Pool percentages exclude paused
sources while retaining configured weights for future re-enablement.

### Multiple models per Client and Agent

In Client → Agent, select a source and use **Client chooses models** to check
multiple models. The connection default is saved as the first canonical route
and used in generated scripts. It does not rewrite other requests: selected
models get exact-match rules, and unselected models are rejected. Fixed-model
mode explicitly rewrites every request to one model; source pools still expose
one model alias. Advanced multi-source, custom-alias and fallback rules remain
in the advanced editor and are never silently simplified.

Copilot discovery displays every enabled model returned by the account catalog.
Models without a supported Agent path remain visible but disabled in multi-select.
The reviewed Responses profile gate remains intact; native Messages models are
no longer limited to Opus. The observed Claude Code aliases `claude-sonnet-5-5`
and `claude-opus-5-5` get exact per-Client routes to Copilot's dotted IDs. Other
provider deployment names are not normalized. Explicit Sonnet 5 remains distinct.

### Usage by source and model

Usage includes source totals and a model breakdown for the actual upstream model,
under the same time/source/Client filters. Columns and overview cards place cached
input immediately after input and before output. Cached tokens are cache reads
already included in input; they must not be added to input again. Missing usage
is counted separately and is not estimated. Existing activity logs populate the
model index once at startup, then only newly appended bytes are consumed.
