# PacaRoute

A Go gateway that connects Codex and Claude Code to GitHub Copilot, Azure OpenAI /
Foundry and compatible model sources. Includes a CLI, embedded browser management
UI, and a native macOS application with the aurora alpaca menu-bar icon.

- Multiple accounts and model sources, with a separate proxy per source.
- Per-person Client keys and Agent/model routes.
- Weighted source pools, persistent session affinity and bounded failover.
- Live source/account/route changes without restarting the service.
- Whole-service and per-source usage, time charts, Copilot quotas and errors.
- Client-specific Agent connection scripts, configuration backups and restore.
- SQLite configuration with atomic revisions and one-time legacy JSON migration.
- Separate Apple Silicon and Intel application packages; standalone Linux/macOS CLI.

```sh
go build -o pacaroute ./cmd/pacaroute
./pacaroute serve --port 8766
./pacaroute ui
```

[Setup, migration, CLI, routing and deployment](docs/pacaroute.md) ·
[Pipeline design](docs/pipeline-design.md) ·
[Copilot protocol research](docs/copilot-api-research.md)

Build and validate:

```sh
go test -race ./...
bash scripts/build-macos-app.sh artifacts/pacaroute-release split
bash scripts/build-pacaroute.sh artifacts/pacaroute-cli
```

The Go release does not ship or launch the previous .NET Copilot Bridge. Historical
sources and captured protocol contracts remain as a migration reference. PacaRoute
continues the original Copilot Bridge work. Runtime dependency notices are in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
