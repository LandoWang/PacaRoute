# Keep provider quota in Accounts

Remove the Copilot quota card and its fetch from Usage. Usage shows only gateway
request/token statistics. Keep the existing per-account quota details in Accounts,
including a manual refresh button. Delete the unused usage-page quota wrapper.

Validation passed: production TypeScript/Vite build; refreshed Usage contains no
quota card, while Accounts → account details loads quota and its refresh button.
Desktop and CLI packages rebuilt as 0.8.4; the local preview is updated.
