# PacaRoute Web

One React/TypeScript management interface for browsers, macOS and Windows. All operations go through the HTTP API; there is no Wails/Electron bridge.

- `npm ci`: install the locked build dependencies.
- `PACAROUTE_API=http://127.0.0.1:8766 npm run dev`: local Vite server with same-origin API proxy.
- `npm run test`: route/account editor contracts.
- `npm run build`: independent static deployment in `dist/`.
- `npm run embed`: copy the compiled frontend into the Go release assets.

Use a separate backend data directory during development. The backend must already be running. Provider credentials and Client API keys are never persisted by the frontend. Management uses an HttpOnly cookie.

See `../../docs/pacaroute-web-desktop.md` for the HTTP contract, local-only capabilities and packaging.
