# PacaRoute 0.7.0 verification

Verified on 2026-11-02 using Go 1.26.1 and the final packaged macOS arm64 executable.
The React frontend and Go native tray share an HTTP management API; the old SwiftUI
and .NET runtime sources are not included in the new desktop distribution.

## Automated and browser contracts

- 36 Go contract tests passed with `go test -race ./...`; `go vet ./...` passed.
- Seven frontend route/provider contracts and TypeScript/Vite production build passed.
- Deliberate mutations bypassing the local authority gate, reusing browser tickets,
  and retaining stale account credentials were all detected by their contract tests.
- Browser checks covered four provider types, automatic IDs/names, resource/account
  filtering, source pause/resume, reference-protected removal, visual Client routes,
  one-time Client keys, and isolated Agent preview/apply with automatic model selection.
- Live Azure discovery listed the selected subscription's resources and five GPT
  deployments in one resource. Saving the selection preserved its subscription and
  resource IDs. No inference requests were sent to that discovered resource.
- GitHub Copilot, Foundry, GitHub, Azure, OpenAI and Anthropic icons are local assets.
  The final packaged server served all six SVG assets with the correct MIME type.
- The final desktop executable started one Go service process on a task-owned port.
  Live port replacement/reconnection and restoration were checked; browser tickets
  were single-use. The user's separate 8765 service and Agent configurations were preserved.
- Native tray menu-click automation could not attach to the windowless tray app in
  this environment; physical menu interaction remains UNVERIFIED.

## Real client verdicts

| Route | Matched tool round trips | Client evidence | Verdict |
| --- | ---: | --- | --- |
| Codex → Copilot GPT 6 Astra | 6 | Two completed app-server turns, 1000 dispatch rows, no router fatal | PASS |
| Claude Code → Copilot Claude Opus 5.5 | 3 | Three transcript tool pairs and complete final answer | PASS |
| Claude Code → Copilot GPT 6 Astra | 3 | Three transcript tool pairs and complete final answer | PASS |

The Codex task continued the same session after a service restart. Evidence was
read from each run's isolated client dispatch database/transcript, not inferred
from HTTP success. Credential authorities were reused without copying refresh-bearing
records. OpenAI/Anthropic account-key inheritance, rotation and model catalogs have
contract coverage; direct live API credentials for those two providers were unavailable.

Final manifests under ignored `artifacts/pacaroute-verification/`:

- `live-appserver-20261102-001929-fbcc45/appserver.manifest.json`
- `live-claude-native-20261102-001929-03f9d6/manifest.json`
- `live-claude-cross-20261102-001929-c55d21/manifest.json`

All three record this exact bundled arm64 executable SHA-256:

`9e88929b0718c85f9fa7408187454fcf9079ef456b84aca3b559914eca647ec6`

## Packages and limits

Separate macOS arm64 and amd64 apps, ZIPs and verified DMGs, Windows x64 portable
ZIP (GUI + CLI executables), and a standalone static Web ZIP were produced under
`artifacts/pacaroute-desktop-release/`. Package hashes are in its `SHA256SUMS.txt`.
The macOS apps are ad-hoc signed, not notarized. Windows binaries are unsigned;
the Inno Setup installer definition and private CI job are provided, but no local
Windows installer build or physical Windows/Intel test is claimed. Native Windows
tray/DPAPI and physical Intel execution remain UNVERIFIED.

The browser QA instance uses its own metadata and client homes. Credentials added
there stay in that directory. Raw traces, screenshots, local identifiers and personal
configuration are excluded from Git.
