using System.Text.Json;
using System.Text.RegularExpressions;
using Microsoft.AspNetCore.Http;

namespace CopilotBridge.Cli.Sources;

internal static partial class PoolSessionIdentity
{
    public static string? Read(string agent, IHeaderDictionary headers, JsonElement body)
    {
        // Codex's thread-id identifies the conversation across process/session resumes.
        // Claude sends the same session UUID in its header and metadata.user_id.
        var names = agent == "codex"
            ? new[] { "X-AgentLink-Session", "thread-id", "session-id", "session_id" }
            : new[] { "X-AgentLink-Session", "X-Claude-Code-Session-Id" };
        foreach (var name in names)
            if (headers.TryGetValue(name, out var value) && value.Count == 1 && Valid(value[0])) return value[0];
        if (agent != "claude-code" || !body.TryGetProperty("metadata", out var metadata)
            || metadata.ValueKind != JsonValueKind.Object || !metadata.TryGetProperty("user_id", out var user)) return null;
        if (user.ValueKind == JsonValueKind.Object) return FromObject(user);
        if (user.ValueKind != JsonValueKind.String || user.GetString() is not { Length: <= 4096 } text) return null;
        try { using var decoded = JsonDocument.Parse(text); return FromObject(decoded.RootElement); }
        catch (JsonException)
        {
            var match = LegacySession().Match(text);
            return match.Success ? match.Groups[1].Value : null;
        }
    }
    private static string? FromObject(JsonElement value) => value.ValueKind == JsonValueKind.Object
        && value.TryGetProperty("session_id", out var session) && session.ValueKind == JsonValueKind.String
        && Valid(session.GetString()) ? session.GetString() : null;
    private static bool Valid(string? value) => !string.IsNullOrWhiteSpace(value) && value.Length <= 512
        && !value.Any(char.IsControl);
    [GeneratedRegex(@"(?:^|_)session_([A-Za-z0-9-]{1,128})$")]
    private static partial Regex LegacySession();
}
