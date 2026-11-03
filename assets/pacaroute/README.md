# PacaRoute artwork

The user selected the existing PacaCrew cyber alpaca with neon glasses for
PacaRoute on 2026-11-02. Reuse the supplied artwork without recoloring or a DEV
badge. The imported source assets came from PacaCrew's `frontend/assets/`:

- `icon.png` -> `app-icon.png`: 1024px macOS-style app icon with transparent margins.
- `icon.icns` -> `AppIcon.icns`: matching macOS iconset, used directly by the
  desktop packager so it cannot regenerate the old procedural mark.
- `pacacrew-mark.png` -> `apps/web/src/assets/pacaroute-mark.png`: 512px mark,
  bundled with a content hash for the web UI and peer account/source icons.
- `icon.ico` -> `internal/pacaroute/trayassets/mark.ico`: Windows tray and installer
  icon, also distributed as `PacaRoute.ico` for shortcuts.

The 64px favicon and 36px color tray PNG are Lanczos reductions of the 512px mark.
The tray retains the artwork's colors. Historical SwiftUI artwork remains part
of the legacy application, not the Go desktop build.
