# PacaRoute

PacaRoute is a Go request gateway for Codex and Claude Code. The same executable
provides the CLI, authenticated management API and embedded React browser UI.
Version 0.7 adds a lightweight macOS/Windows system tray in the same Go process.
No Node, .NET runtime, separate bridge or embedded browser is needed at runtime.
See [web and desktop architecture](pacaroute-web-desktop.md) for current builds,
account setup, separate frontend development and distribution instructions.
Historical SwiftUI packaging instructions below apply to the 0.6 compatibility app.

## Run

Requires Go 1.26 to build, or use a prebuilt archive. macOS UI requires macOS 14+.
Apple Silicon and Intel applications are packaged separately.

```sh
go build -o pacaroute ./cmd/pacaroute
./pacaroute serve --port 8766
./pacaroute ui --port 8766
```

Open the printed URL and use the protected `admin.key` file in the data directory
to sign in to management. `ui` opens the management page; desktop/local-management mode uses a one-use browser login ticket. The
admin cookie is HttpOnly and SameSite Strict; cross-origin management writes are
rejected. The browser does not store the key in localStorage. For a private local
Agent setup, `serve --allow-local` permits loopback requests through the enabled
local `default` Client and its routes. The default CLI server requires Client keys. The Mac application enables this
local mode, while network callers always need their own Client keys. The public
`copilot-bridge-provider` discovery sentinel from AgentLink is accepted only through
this same local access gate, so existing Codex sessions survive migration. It is
never a network Client key.

`--home PATH` or `PACAROUTE_HOME` chooses the data directory. On macOS an initialized
`~/Library/Application Support/PacaRoute` workspace takes precedence. Otherwise an
existing `~/Library/Application Support/Copilot Bridge` directory is reused so saved
accounts and sources are retained; fresh installations use the PacaRoute directory.
Linux uses `$XDG_DATA_HOME/pacaroute` or `~/.local/share/pacaroute`. The native Mac
app follows the same selection. An initialized PacaRoute workspace can retain the
original credential authority through `desktop-settings.json`; provider credentials
need not be copied. Quit AgentLink before assigning its occupied port to PacaRoute.
Installing the new package does not change existing Agent settings. `--credential-root`
selects an existing credential authority independently of test metadata; do not copy
refresh-bearing credentials for tests.

## Accounts, sources and proxies

Fresh installations start with no accounts or sources. Add an account, authorize
it (device login, Azure CLI, or API key), then add the resources/models you need.
Existing configurations that referenced the legacy implicit GitHub/Azure identities
are materialized as ordinary stored entries without moving their credentials.

1. Add a GitHub account in Accounts, then use Device Login. Then explicitly add a Copilot
   source for that account. Existing referenced Copilot sources are retained during migration.
2. For Azure OpenAI / Foundry, choose an Azure account, a resource endpoint and GPT
   deployment names. Separate resources can use the same account. Resource endpoints
   are normalized to `/openai/v1`.
3. Choose each source's Proxy: environment/system, direct, HTTP(S), or SOCKS5.
4. Pause a source to exclude future requests. Already streaming requests finish.

```sh
printf '%s' '{"id":"work","name":"Work GitHub","kind":"github"}' | ./pacaroute accounts add
./pacaroute auth login --account work
printf '%s' '{"id":"work-copilot","name":"Work Copilot","kind":"copilot","account_id":"work","endpoint":"","deployments":[]}' | ./pacaroute sources add
./pacaroute models --source work-copilot
./pacaroute sources pause --id work-copilot
./pacaroute sources resume --id work-copilot
```

GitHub credentials use the previous encrypted Unix format. Plugin credentials use
Copilot exchange tokens; exchanged leases are encrypted and cached across restarts.
Refreshable custom OAuth credentials are rotated under the same stable credential
file lock used by the previous implementation. Runtime code receives immutable leases
through the auth facade. API keys can be submitted through the UI or stdin:

```sh
./pacaroute key set --source foundry < /protected/path/to/key
```

Never pass API keys as command arguments. Native Mac Azure keys stay in Keychain
and travel through a persistent private child-process pipe; changing a key does not
require restarting the service. Server API keys use the encrypted store. Machine/user
derived encryption is compatible with the previous Unix vault; it is not a hardware
security boundary against processes running as the same OS user. Minimal Linux containers without an OS machine-id keep a stable install identity in the persistent data volume.

Azure CLI authentication/resource discovery requires `az` on the host. Add an isolated
Azure account and use Device Login in the UI or run `pacaroute auth login --account ID`, or use the shared
`azure-local` identity signed in through `az login`. API-key sources do not require
Azure CLI. The minimal Docker image supports GitHub and API-key sources; it does not
bundle Azure CLI.

## Client routing and weighted pools

A Client is a person/caller, not an Agent product. Each Client gets a random API key;
only its SHA-256 hash is stored in `pacaroute.sqlite`. The key is shown once on creation
or rotation. Disabled/deleted Clients are rejected on their next request.

Routes are evaluated in order, matching Agent (`codex` or `claude-code`) and the
requested model alias (`*` matches any model). A route targets a source/model or
`pool:<pool-id>`. A caller cannot bypass its route using a source URL.

```json
{
  "id": "alice",
  "name": "Alice",
  "routes": [
    {"id":"coding","agent":"codex","match_model":"coding","source_id":"pool:balanced","model":"coding"},
    {"id":"claude","agent":"claude-code","match_model":"claude-opus-5.5","source_id":"copilot","model":"claude-opus-5.5"}
  ]
}
```

Pass that record on stdin to `pacaroute clients add`. Configure the caller's tools:

- Codex: `base_url = "https://gateway.example/codex"`, `wire_api = "responses"`,
  an `env_key` naming the environment variable containing its Client key, and the
  route's model alias.
- Claude Code: `ANTHROPIC_BASE_URL=https://gateway.example/cc`,
  `ANTHROPIC_AUTH_TOKEN=<Client key>`, and the route's model alias.

Pool members each specify a source, concrete model and integer weight. Weights
represent relative shares of **new sessions**, not individual requests. Weight 0
stops new assignments while preserving existing bindings. Binding scope includes
Client + Agent + Pool + Session. Codex thread headers and Claude session metadata
are recognized; callers can explicitly supply `X-PacaRoute-Session`. Without a
session identifier, requests are independently distributed.

Bindings are journaled by hash without prompts or raw session IDs. Existing local
AgentLink bindings are imported lazily; new authenticated Clients never inherit
another user's legacy binding. The index is bounded to 100,000 recent bindings
and expires inactive entries after 30 days. Journal compaction avoids rewriting
all sessions on every request.

Pools can permit failover for connection/authentication failures and HTTP
401/403/408/429/5xx. Up to three distinct members are attempted with cooldowns.
Failover happens only before downstream response headers/body are committed.
Interrupted streams are errors and are never stitched to another source. A
`previous_response_id` or `item_reference` continuation cannot move providers;
PacaRoute returns 409 when its original binding is unavailable. Use a fresh session
or full replayable history after such an error.

An upstream stream failure cools down its source/model for 30 seconds. The next
full-history client retry may select another pool member when failover is enabled;
PacaRoute never retries a partially delivered response internally. Downstream
cancellation or write failure does not penalize the upstream source.

## Hot configuration and local Agent configuration

CLI, web UI and management API share `pacaroute.sqlite`. All references are
validated before a revision-checked atomic write. Startup imports legacy split
JSON once and retains the original files. New requests use immutable snapshots;
external CLI writes appear within one second. Referenced accounts/sources/pools
cannot be deleted until their routes are updated. No service restart is required
for accounts, sources, Client routes, pause state, weights, credentials or proxies.

Use the Client page to generate a connection script for each enabled Agent. Scripts
call `pacaroute connect`, which needs no running local gateway, reads a remote
Client key on stdin, backs up the old TOML/JSON and preserves unrelated settings.
The script configures generic gateway URLs; target selection belongs to Routes.
New local `default` Clients must have routes assigned before use. Imported local
configurations retain visible legacy direct routing until explicitly replaced.
Deleting `default` permanently disables local keyless access.

See [the current workspace and migration contract](pacaroute-web-desktop.md#clients-routes-and-configuration).

## Usage and errors

Sources show active model cooldowns with the last error and expiry. Pools show
eligible/total members and effective new-session allocation, excluding cooled,
paused and zero-weight members. A global warning highlights active cooldowns; an
exhausted pool produces a red alert. Authenticated `GET /api/status` exposes the
same `routing` snapshot from memory, refreshed by the UI every five seconds while
visible. Cooldowns expire into eligibility, not proven health, and reset when the
service restarts. Historical failure records remain available in Errors.

Responses failure terminals are preserved, including Azure's `error` followed by
`response.failed`. Missing terminals are reported as failures with provider or
transport details, never as successful completions. See the
[0.8.6 verification report](pacaroute-0.8.6-verification.md) for evidence and limits.

The usage page provides whole-service totals, per-source totals and UTC hourly
buckets over a chosen time window. The API also supports filtering by Client.
Final stream usage updates earlier snapshots rather than summing duplicates.
Input totals include cached prompt tokens across providers; cache counters are a subset, not an extra total. Missing upstream usage remains missing, not an invented zero. Copilot account
quota is fetched separately from local request statistics. Azure billing/quotas
remain authoritative in Azure.

Request records include source, model, Client, tokens, duration and status. Prompts
and credentials are excluded. `--trace-dir PATH` explicitly enables request/response
body capture for isolated debugging; these files can contain conversation content.
Chat-history browsing and analysis are removed from the Go app. Session identifiers
are still used for sticky routing; conversation history is not indexed.

## Deploy and package

```sh
./pacaroute serve --bind 0.0.0.0 --port 8766 --home /var/lib/pacaroute \
  --public-url https://gateway.example
# Put TLS termination in front of this listener for network use.
docker build -f Dockerfile.pacaroute -t pacaroute .
docker run --rm -p 127.0.0.1:8766:8766 -v pacaroute-data:/data pacaroute
bash scripts/build-macos-app.sh artifacts/pacaroute-release split
bash scripts/build-pacaroute.sh artifacts/pacaroute-cli
```

When using a TLS reverse proxy, set `--public-url` to its exact public origin so
management accepts that origin and sets Secure cookies. Forwarded headers alone
do not establish trust.

Back up the data directory, keep its permissions private, and persist it across
container restarts. The Mac package uses ad-hoc signing for direct distribution;
Developer ID signing and Apple notarization are separate release steps. Intel
binaries can be built on Apple Silicon; actual Intel-hardware validation must be
reported separately from cross-compilation/Rosetta validation.

## Compatibility boundaries

Native Codex Responses and Claude Messages paths preserve opaque tool and extension
fields, with the existing probe-backed Copilot request constraints. Cross-protocol
routes translate text, images and function/custom-tool exchanges. Provider-encrypted
reasoning cannot be carried across different protocols; unsupported content/tool
kinds produce a clear error rather than silently deleting them. Copilot's encrypted
item IDs can change per SSE frame; translated streams group by stable output/content
indices so Claude receives complete answers.

The normal Copilot model picker intersects the live enabled catalog with the
previous gateway's reviewed profiles. Other native models can be routed explicitly
as passthrough; endpoint availability alone does not prove coding-agent behavior.
Codex-to-Claude is an advanced function/custom-tool route. Disable Codex's
provider-owned web search (`web_search = "disabled"`); unsupported server tools
are rejected explicitly. It is not advertised by the default Copilot picker.

Legacy .NET sources remain in the repository as a reference and for historical
contract tests. They are not built or invoked by the PacaRoute release.

## Portable configuration

In **Service settings → Configuration import/export**, download a versioned JSON
bundle and import it into another current PacaRoute installation. Exports contain
configuration only: no account tokens/API keys, Client key hashes, management keys,
usage/error history, session bindings, port settings or machine-specific files.

The import preview lists counts, renamed objects and post-import setup:

- **Add** preserves the current workspace. Colliding names gain a numeric suffix;
  imported IDs receive an independent namespace, and account/source/pool/Proxy/Client
  references are rewritten together. A second local Client becomes a network Client
  with a new key. Existing Client keys remain valid.
- **Replace** removes the current configuration graph and installs the imported one.
  Export the current configuration first if you want a portable copy. Existing
  network Client keys no longer authorize requests; imported Clients receive new keys.
- Both modes require reauthorizing accounts or entering keys. No imported identity
  inherits a destination vault credential. Shared Azure CLI accounts import in device
  login mode with an isolated Azure directory. In add mode the destination's global
  account-login Proxy stays selected; source-specific Proxy bindings are remapped.
- The file, import mode and revision are bound to the management preview. If another
  writer changes the configuration, preview again. Validation/storage failures do
  not publish a partially imported configuration. New requests observe the new
  snapshot; active streams retain their original snapshot.

```sh
pacaroute config export --file workspace.json
pacaroute config import --file workspace.json --mode add        # preview only
pacaroute config import --file workspace.json --mode add --apply
pacaroute config import --file workspace.json --mode replace --apply
```

Export refuses to overwrite an existing file. `--home` and `--credential-root` keep
their usual meanings. After import, authorize accounts and obtain new connection
scripts from Client; update the script's service address for the new installation.

## Multi-model pools and independent throttles

A pool can expose multiple public models with `multi_model: true`. Each member keeps
its concrete `model` (deployment) and can specify an `alias`; an omitted alias exposes
the concrete model name. `model` on the pool selects the connection-script default.
The existing pool `agent` still defines its supported Agent; this does not add new
cross-protocol compatibility.

```json
{
  "id": "large-source", "name": "Large source", "agent": "claude-code",
  "model": "astra", "multi_model": true, "failover": true,
  "members": [
    {"id":"a-astra","source_id":"foundry-a","model":"astra-deployment","alias":"astra","weight":5},
    {"id":"b-astra","source_id":"foundry-b","model":"astra-deployment","alias":"astra","weight":3},
    {"id":"c-astra","source_id":"foundry-c","model":"astra-deployment","alias":"astra","weight":2},
    {"id":"sonnet","source_id":"github-source","model":"claude-sonnet-5.5","weight":1}
  ]
}
```

Select the pool in Client and choose its exposed models. A request for `astra` only
considers that model group; Sonnet is never a fallback for Astra. Within a model,
new sessions use relative member weights and healthy existing sessions remain sticky.
If the weight-2 member is throttled, surviving 5:3 weights become 62.5%:37.5%. If the
weight-5 member is throttled, surviving 3:2 weights become 60%:40%. Admission returns
to the original proportions when its cooldown expires. Observed finite traffic can
differ from these probabilities.

Cooldown state is keyed by **source ID × concrete model**, independent of account,
pool and public alias. Two aliases pointing at the same source/deployment share its
throttle. Another deployment on the same source keeps its own state. HTTP 429 honors
`retry-after-ms`, `x-ms-retry-after-ms`, numeric `Retry-After`, or an HTTP-date, bounded
to 1 second–24 hours; absent/invalid hints use 30 seconds. Parallel failures cannot
shorten an existing deadline. Stream rate-limit errors without a retry hint use the
30-second fallback. Expiration permits retry; it does not prove upstream recovery.
The UI shows each model group's availability, normalized proportions, reason and
remaining time. Restart clears transient cooldowns but retains historical errors.

## Remote PacaRoute accounts and connection QR codes

Choose **Accounts → Add account → PacaRoute**, then enter the remote service URL
and its **Client Key** (not its management key). Alternatively choose the scan /
paste action and use a QR image, camera, or copied connection string. Camera
access requires browser support and a secure context such as HTTPS or localhost.
Images are decoded locally. Review the account form before saving, then select
models from the remote catalog. Refresh discovery when remote Client routes
change; adding models to the remote catalog never silently enables them locally.

Remote PacaRoute sources use ordinary source proxy bindings and may join pools.
Enable the known **PacaRoute** Agent on the upstream Client and assign its own
source/pool model routes. This grant is independent of direct Codex and Claude Code
grants. Its endpoints are `/pacaroute/codex/responses`, `/pacaroute/cc/messages`
and `/pacaroute/v1/models`; the catalog only publishes the PacaRoute routes and
their supported wire protocols. Native Agent paths and peer paths return 404 when
their corresponding grant is disabled. Authentication still runs first.
`GET /v1/models` is an alias of the peer catalog. Internal accounts, resources and
pools are not exposed. Discovery reads `peer_path` from the public service
descriptor, and saves the namespaced endpoint with the source. It never falls back
to native grants when a modern peer denies access. Older peers without that
capability can still be discovered through their original catalog endpoints.
Inference uses the matching native Agent protocol. Forwarding is limited to eight hops;
derived session identifiers separate different local Clients sharing one remote
Client Key. Usage is recorded at each hop independently.

A network Client with PacaRoute enabled uses its **Connection QR** dialog encodes its service URL and Client Key
as a version-1 `pacaroute://connect#...` payload. This is connection data, not a
browser navigation link. Change the URL to a reachable IP or domain before sharing
across devices. Anyone holding the QR has that Client's access. Key rotation
invalidates old QR codes. Local password-free Clients cannot generate portable QR
codes. Exported configuration includes remote account/source metadata but omits
keys; re-enter the remote Client Key after configuration import.

## Independent management and service ports

The desktop app now keeps two listeners in the same Go process:

- A persistent loopback management port for the web UI and authenticated APIs.
- An Agent service port for Client requests, model discovery and health checks.

Use **Open / Close model service port** in the tray, or the service settings page.
Management stays available while the service port is closed, including account
login, configuration changes and usage review. Changing a service port keeps the
management URL and login intact. Existing clients must use the new service address;
regenerate connection scripts/QR codes as needed. Saving a new service port while
stopped does not start it. Desktop remembers the serving state and both ports in
`desktop-settings.json`; application configuration remains in SQLite.

Stopping or moving a listener stops new admission and lets accepted requests
finish. Auth, affinity, cooldowns and usage share one unchanged application
instance. Exiting the entire application allows up to 30 seconds to drain before
forcing remaining connections closed. A port conflict keeps the old listener
running and is shown in management.

CLI deployments can opt in:

```sh
pacaroute serve --port 8766 --admin-port 8767
pacaroute serve --bind 0.0.0.0 --port 8766 --admin-port 8767 --no-serve
pacaroute ui --port 8767
```

The management listener binds to `127.0.0.1`; `--bind` applies only to inference.
Use the service address for Clients and remote PacaRoute connections. Management
endpoints are unavailable on the service port, and inference is unavailable on
the management port. `GET /api/service` reports serving, port, admin_port, draining
listeners and errors. Authenticated `PUT /api/service` accepts either
`{"serving": true|false}` or `{"port": N}`. `--no-serve` requires `--admin-port`.
The single-port CLI command remains supported for existing deployments.


## Client quotas and confirmed-usage billing

The Client editor supports rolling windows (minute, hour, day or 30 days), accepted
request counts, total input tokens including cache, and USD costs. Zero/empty limits
are unlimited. A 429 with Retry-After is returned before an upstream call when a
limit is exhausted. Request slots are admitted atomically; token/cost checks use
confirmed usage, so requests already in flight can exceed their limits. Disabled
Agent paths are checked before quota admission.
The overshoot has no fixed monetary bound in this policy: concurrent requests and
long outputs can make it substantial. Existing streams are not interrupted when
another request settles and exhausts the quota.

Each source has a model rate editor in USD per million tokens: ordinary input,
cache reads, output and optional cache writes. Rates are keyed by the actual source
model/deployment, not the pool alias or the requested client model. Cached input is
subtracted from ordinary input; cache-write pricing is separate when reported.
Missing rates or required usage remain unpriced, never a zero-cost estimate.
Copilot premium-request billing and provider balances are not inferred from tokens.
Configured USD totals are local estimates, not the provider's bill.

The metadata SQLite database also holds an indexed quota ledger and append-only
billing events. Every accepted request has a unique ID, a reserve event, an
idempotent settlement, and a release event. This version reserves request slots and
zero monetary credit; monetary reservation is a future policy. The settlement
retains its source, actual model, usage and rate snapshot. Historical prices are
never rewritten, and unfinished requests survive restart as pending reconciliation.
Statistics begin when this ledger is first enabled; old activity files are not
retroactively priced. Import/export carries quota policies and source rates, but
never transfers consumed usage or billing events. No central billing service is
contacted by this version.
