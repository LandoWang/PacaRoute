# PacaRoute 0.8.3 verification

## Scope

Unified Client routing and connection scripts with encrypted, recoverable Client
keys; visible Copilot quota and an explanation of empty local usage. No protocol
translation or provider credential format changes.

## Automated contracts

- `go test -race ./...` and `go vet ./...`: pass.
- Web tests: 11 passing; production TypeScript/Vite build and embed: pass.
- Client-key lifecycle tests cover admin-only recovery, encrypted storage,
  restart, rotation/auth revocation, duplicate create, deletion, legacy import,
  namespace isolation, malformed hashes and mismatched encrypted records.
- Mutation: removing key-hash validation fails the legacy import contract.
- Bash test executes metacharacter-bearing synthetic keys through stdin,
  without prompts, command injection or putting the key in CLI arguments.
- PowerShell exports are checked for literal quoting and stdin delivery; Windows
  execution is not claimed from this macOS host.

## Real clients on the final macOS arm64 package

Binary SHA-256: `bd35a47cfc053e76a7b53c7937e2d47e96860295fbcdcdc1a5d6b7cdf2d58a83`.
Both actuators create an authenticated Client, recover its key via management,
render the real web script, and execute it against isolated Agent homes.

- Codex app-server: `live-appserver-20261102-012733-69a487/appserver.manifest.json`:
  two completed turns, six matching wire tool roundtrips, zero router fatals in
  the isolated dispatch database. Pool failover and restart continuation pass.
- Claude Code native: `live-claude-native-20261102-012733-fd9942/manifest.json`:
  four matching tool roundtrips and a complete final answer.
- Verdict stored locally in the release directory; no credentials or private
  traces are included in source control or distributable packages.

## Browser and packages

- Existing browser refreshed onto 0.8.3; standalone Routes navigation absent.
- Legacy Routes URL redirects to Clients and retains the Client selection.
- Remote script preview redacts the key while copied Bash includes it.
- Windows script preview renders with a redacted key and no interactive prompt.
- Live service verification request recorded exactly 15 input and 9 output tokens.
- Copilot upstream quota renders independently, including unlimited quotas,
  premium percentage, credit units and reset date.
- macOS arm64/amd64 apps, ZIPs and DMGs, Windows x64 desktop/CLI, standalone
  cross-platform CLI and Web UI ZIP built. macOS bundles ad-hoc signed/verified;
  these are not notarized releases.
