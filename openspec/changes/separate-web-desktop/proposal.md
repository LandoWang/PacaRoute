# Shared web management and lightweight desktop tray

Replace the mixed SwiftUI/embedded management experience with one React and TypeScript application using Tailwind and shadcn-style Radix primitives. Keep the Go gateway and its management API independently usable. Embed the compiled frontend in release binaries; use a same-origin development proxy during frontend development.

Ship an optional Go desktop build for macOS and Windows with a native tray/menu-bar, browser launch, owned service lifecycle and startup integration. Keep headless CLI deployment independent of GUI libraries. Preserve the existing Swift app as a compatibility option while moving local Agent configuration and session reading into explicitly local management endpoints. Keep credentials at their original authority; do not copy refresh-bearing records.

Client routes and weighted pools must have visual editors. The existing gateway protocol and affinity semantics remain unchanged. MCP is an optional future adapter, not part of this migration.

Accounts identify providers (GitHub, OpenAI, Anthropic, Azure); sources identify products/resources. One provider account can back multiple resource sources. Use device login or account-level API keys, with resource overrides, and discover Azure deployments through the selected identity. Default forms require only provider/login or API key and resource/model selection; generate names and IDs, infer provider endpoints, and put optional configuration under Advanced.
