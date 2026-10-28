namespace CopilotBridge.Cli.Hosting.ClientConfig;

/// <summary>
/// The connection facts a client configurator writes into a client's config,
/// derived once from <c>appsettings.json</c> (plus an optional <c>--port</c>
/// override) so every configurator and <c>config status</c> share one source of
/// truth. Purely data — no I/O.
/// </summary>
/// <param name="Port">The TCP port the bridge listens on: the CLI <c>--port</c>
/// override if given, else <c>Server:Port</c> from appsettings (default 8765).</param>
internal sealed record BridgeConnection(int Port, string SourceId = "copilot")
{
    private string Prefix => SourceId == "copilot" ? "" : $"/sources/{Uri.EscapeDataString(SourceId)}";

    /// <summary>Base URL Claude Code points <c>ANTHROPIC_BASE_URL</c> at.</summary>
    public string ClaudeCodeBaseUrl => $"http://localhost:{Port}{Prefix}/cc";

    /// <summary>Base URL the Codex provider block points <c>base_url</c> at.</summary>
    public string CodexBaseUrl => $"http://localhost:{Port}{Prefix}/codex";
}
