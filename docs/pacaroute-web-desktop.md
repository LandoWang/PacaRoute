# PacaRoute web and desktop architecture

PacaRoute 0.7 uses one React/TypeScript frontend and a Go management API. The desktop distribution adds a small native system-tray/menu-bar controller to the Go process; it opens the management interface in the default browser. It does not use Wails, Electron or a bundled browser. Closing the browser does not stop the gateway.

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

The default account form asks only for a provider and, for OpenAI/Anthropic, an API key. GitHub/Azure proceed directly to device authorization. Names and stable IDs are generated automatically; optional labels and IDs are under Advanced. Add source starts with account selection, then infers the resource type. Azure picks the default subscription when available and discovers resources/deployments. API resources use official endpoints and discover models automatically. Custom endpoints, credentials, Proxy and manual model IDs remain available under Advanced; discovering models never silently rewrites saved routes.

An OpenAI account cannot be attached to an Anthropic source. Legacy `api-key` account records remain compatible but are no longer the default new-account choice. GitHub Device Login authenticates the account; use Add source to connect its Copilot resource. The legacy default Copilot source remains available. OpenAI and Anthropic account keys are set once on the account; their sources inherit that key unless a resource-specific credential overrides it. Key rotation invalidates leases without restarting the service. CLI equivalents are `pacaroute key set --account ACCOUNT` and `pacaroute key set --source SOURCE`, both reading the secret from stdin.

Azure Accounts exposes subscription discovery, resource discovery and deployment selection. The resulting source retains `subscription_id` and `resource_id`. Existing sources can rediscover deployments and explicitly update their configured model list. A source without a resource ID can match its endpoint within its saved subscription. Discovery requires an Azure CLI login with resource read permission; an inference API key alone does not grant management-plane deployment discovery. OpenAI and Anthropic resources can discover the model catalog through their authenticated API. A discovered model ID does not guarantee every Agent/tool capability; only configured selections are exposed for routing.

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
| POST | `/api/clients` | Create Client and display its key once |
| POST | `/api/clients/{id}/rotate` | Rotate Client key |
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
- `/api/local/sessions`, `/session`: local Codex/Claude history. Codex reads its existing SQLite indexes read-only; JSONL catalogs cache file metadata and read bounded pieces. History is paginated, directory-contained and read only when requested.
- `/api/local/analyze`: explicitly sends currently loaded history (bounded to approximately 48,000 characters) to the selected source. Transcript text is treated as untrusted evidence. It is not sent merely by opening history.
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

See [release verification](pacaroute-web-desktop-verification.md) for exact client evidence and platform limits.
