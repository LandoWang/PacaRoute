- [x] Add offline backup listing and fingerprinted restoration, with private snapshots.
- [x] Add an Agent-page backup picker and confirmation for both clients.
- [x] Verify original bytes, absent files, stale previews, invalid input and isolation.
- [x] Build and inspect the native app and package the updated application.

Verification: 56 focused .NET checks, 7 Swift contract checks, Debug CLI recovery
for both clients, Native AOT Codex recovery, and a real macOS UI restore against
a disposable configuration all passed. Removing the preview guard made both
concurrency tests fail. The restored UI fixture matched its original bytes and
retained the pre-restore snapshot. App and DMG/ZIP build 5 were verified and the
local app bundle updated without restarting either live proxy.
