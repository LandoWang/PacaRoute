using System.Text.Json;
using System.Text.RegularExpressions;
using CopilotBridge.Cli.Endpoints.ClaudeCode;
using CopilotBridge.Cli.Models;
using Serilog.Core;
using Serilog.Events;

namespace CopilotBridge.Cli.Desktop;

internal sealed partial class DesktopTelemetry : ILogEventSink
{
    private static readonly object Gate = new();
    private static bool Enabled => Environment.GetEnvironmentVariable("COPILOT_BRIDGE_DESKTOP") == "1";

    public static void Record(RequestSummary s)
    {
        if (!Enabled) return;
        Write(new DesktopActivity
        {
            Kind = s.Kind == "count_tokens" ? "estimate" : "request",
            Model = s.ResolvedModel ?? s.RequestedModel, Source = s.Source,
            Client = s.Kind == "responses" ? "Codex" : "Claude Code",
            InputTokens = s.Usage.InputTokens, OutputTokens = s.Usage.OutputTokens,
            CacheReadTokens = s.Usage.CacheReadInputTokens,
            CacheWriteTokens = s.Usage.CacheCreationInputTokens,
            Status = s.StatusCode, DurationMs = s.DurationMs,
            Error = s.StatusCode == 0 || s.StatusCode >= 400
                ? $"HTTP {s.StatusCode}; {s.UpstreamTimeout ?? "request failed"}" : null,
        });
    }

    public void Emit(LogEvent logEvent)
    {
        if (!Enabled || logEvent.Level < LogEventLevel.Warning
            || logEvent.MessageTemplate.Text.StartsWith("summary ", StringComparison.Ordinal)) return;
        // Record the template, never rendered properties or exception bodies: upstream
        // errors may embed request content, bearer tokens, or arbitrary response bytes.
        var source = logEvent.Properties.TryGetValue("ModelSource", out var value) && value is ScalarValue { Value: string id } ? id : null;
        Write(new DesktopActivity { Kind = "error", Source = source, Error = Sanitize(logEvent.MessageTemplate.Text) });
    }

    internal static string Sanitize(string value)
    {
        var safe = SecretPattern().Replace(value, "[redacted]");
        return safe.Length <= 1000 ? safe : safe[..1000];
    }

    internal static void Write(DesktopActivity activity)
    {
        try
        {
            lock (Gate)
            {
                var dir = Path.Combine(AppContext.BaseDirectory, "desktop-activity");
                Directory.CreateDirectory(dir);
                var path = Path.Combine(dir, $"{DateTime.UtcNow:yyyy-MM-dd}.jsonl");
                File.AppendAllText(path, JsonSerializer.Serialize(activity, JsonContext.Default.DesktopActivity) + "\n");
                if (!OperatingSystem.IsWindows()) File.SetUnixFileMode(path, UnixFileMode.UserRead | UnixFileMode.UserWrite);
            }
        }
        catch (Exception ex) when (ex is IOException or UnauthorizedAccessException)
        { /* Observability cannot break a model request. */ }
    }

    [GeneratedRegex(@"(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|sk-[A-Za-z0-9_-]+|(?i:Bearer)\s+\S+)")]
    private static partial Regex SecretPattern();
}
