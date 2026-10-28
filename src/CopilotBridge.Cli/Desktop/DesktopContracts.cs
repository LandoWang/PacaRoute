namespace CopilotBridge.Cli.Desktop;

internal sealed record DesktopReply
{
    public bool Ok { get; init; } = true;
    public string Event { get; init; } = "result";
    public string? Error { get; init; }
    public bool? LoggedIn { get; init; }
    public string? Login { get; init; }
    public string? UserCode { get; init; }
    public string? VerificationUri { get; init; }
    public double? ExpiresIn { get; init; }
    public List<DesktopClient>? Clients { get; init; }
    public List<DesktopModel>? Models { get; init; }
    public CopilotAccountUsage? Quota { get; init; }
    public string? QuotaError { get; init; }
    public string? Path { get; init; }
    public string? Fingerprint { get; init; }
    public IReadOnlyList<string>? Changes { get; init; }
    public string? BackupPath { get; init; }
}

internal sealed record DesktopClient(string Id, string Path, bool Connected, bool Drifted, string? Model, string? Error, string Source = "copilot");
internal sealed record DesktopModel(string Id, string Name, string? Vendor, bool Claude, bool Codex);

internal sealed record CopilotAccountUsage
{
    public string? CopilotPlan { get; init; }
    public string? QuotaResetDate { get; init; }
    public Dictionary<string, CopilotQuota>? QuotaSnapshots { get; init; }
}

internal sealed record CopilotQuota
{
    public double? Entitlement { get; init; }
    public double? Remaining { get; init; }
    public double? QuotaRemaining { get; init; }
    public double? PercentRemaining { get; init; }
    public double? OverageCount { get; init; }
    public bool? Unlimited { get; init; }
}

internal sealed record DesktopActivity
{
    public string Id { get; init; } = Guid.NewGuid().ToString("N");
    public DateTimeOffset Timestamp { get; init; } = DateTimeOffset.UtcNow;
    public required string Kind { get; init; }
    public string? Model { get; init; }
    public string? Client { get; init; }
    public string? Source { get; init; }
    public int? InputTokens { get; init; }
    public int? OutputTokens { get; init; }
    public int? CacheReadTokens { get; init; }
    public int? CacheWriteTokens { get; init; }
    public int? Status { get; init; }
    public long? DurationMs { get; init; }
    public string? Error { get; init; }
}
