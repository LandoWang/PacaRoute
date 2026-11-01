using System.Text.Json;
using System.Text.RegularExpressions;
using CopilotBridge.Cli.Models;

namespace CopilotBridge.Cli.Sources;

internal sealed record SourcePool
{
    public required string Id { get; init; }
    public required string Name { get; init; }
    public required string Agent { get; init; }
    public required string Model { get; init; }
    public bool Failover { get; init; } = true;
    public required PoolMember[] Members { get; init; }
}

internal sealed record PoolMember
{
    public required string Id { get; init; }
    public required string SourceId { get; init; }
    public required string Model { get; init; }
    public int Weight { get; init; } = 1;
}

/// <summary>Only pool metadata reloads; source credentials remain in their existing facade.</summary>
internal sealed partial class SourcePoolRegistry(SourceRegistry sources, string? path)
{
    private readonly object _gate = new();
    private SourcePool[] _pools = [];
    private (long Length, DateTime Modified)? _stamp;

    public static string? ConfigurationPath => Environment.GetEnvironmentVariable("COPILOT_BRIDGE_POOLS_FILE");
    public SourcePool? Find(string id) => Snapshot().FirstOrDefault(pool => pool.Id == id);

    public SourcePool[] Snapshot()
    {
        lock (_gate)
        {
            if (string.IsNullOrEmpty(path) || !File.Exists(path)) { _stamp = null; return _pools = []; }
            var file = new FileInfo(path);
            var stamp = (file.Length, file.LastWriteTimeUtc);
            if (_stamp == stamp) return _pools;
            if (file.Length > 1_048_576) throw new ArgumentException("Source pool configuration is too large.");
            var pools = JsonSerializer.Deserialize(File.ReadAllBytes(path), JsonContext.Default.SourcePoolArray) ?? [];
            Validate(pools, sources);
            _pools = pools; _stamp = stamp;
            return pools;
        }
    }

    internal static void Validate(SourcePool[] pools, SourceRegistry sources)
    {
        if (pools.Length > 128 || pools.Any(pool => pool is null) || pools.Select(pool => pool.Id).Distinct(StringComparer.Ordinal).Count() != pools.Length)
            throw new ArgumentException("Pool IDs must be unique; at most 128 pools are supported.");
        foreach (var pool in pools)
        {
            if (!Identifier().IsMatch(pool.Id) || string.IsNullOrWhiteSpace(pool.Name) || pool.Name.Length > 100
                || pool.Agent is not ("codex" or "claude-code") || !ModelId().IsMatch(pool.Model)
                || pool.Members is not { Length: > 0 and <= 32 }
                || pool.Members.Any(member => member is null)
                || pool.Members.Select(member => member.Id).Distinct(StringComparer.Ordinal).Count() != pool.Members.Length
                || !pool.Members.Any(member => member.Weight > 0))
                throw new ArgumentException("Enter a pool name, Agent, model and uniquely identified members with positive total weight.");
            foreach (var member in pool.Members)
            {
                if (!Identifier().IsMatch(member.Id) || !Identifier().IsMatch(member.SourceId)
                    || !ModelId().IsMatch(member.Model) || member.Weight is < 0 or > 1_000_000)
                    throw new ArgumentException("Invalid pool member or weight (0–1000000).");
                if (member.SourceId == "copilot") continue;
                var source = sources.Find(member.SourceId) ?? throw new ArgumentException("A pool member references an unknown source.");
                if (source.Kind == "azure-openai" && !source.Deployments.Contains(member.Model, StringComparer.Ordinal))
                    throw new ArgumentException("Select a deployment configured on the member's Azure source.");
            }
        }
    }

    [GeneratedRegex(@"\A[a-z][a-z0-9-]{0,62}\z")]
    private static partial Regex Identifier();
    [GeneratedRegex(@"\A[A-Za-z0-9][A-Za-z0-9._-]{0,159}\z")]
    private static partial Regex ModelId();
}
