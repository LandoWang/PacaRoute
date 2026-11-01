using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using CopilotBridge.Cli.Models;

namespace CopilotBridge.Cli.Sources;

internal sealed record PoolBinding
{
    public required string Key { get; init; }
    public required string MemberId { get; init; }
    public required string SourceId { get; init; }
    public required string Model { get; init; }
}

/// <summary>One append per new binding/failover; existing sessions require no disk access.
/// The journal stores hashes and routing metadata, never prompts or raw session identifiers.</summary>
internal sealed class PoolHistoryRequiredException : InvalidOperationException { }

internal sealed class PoolAffinityStore : IDisposable
{
    private readonly object _gate = new();
    private readonly Dictionary<string, PoolBinding> _bindings = new(StringComparer.Ordinal);
    private readonly Dictionary<string, DateTimeOffset> _cooldowns = new(StringComparer.Ordinal);
    private readonly FileStream _journal;
    private readonly Func<int, int> _draw;
    private readonly Func<DateTimeOffset> _now;

    public PoolAffinityStore(string path, Func<int, int>? draw = null, Func<DateTimeOffset>? now = null)
    {
        _draw = draw ?? RandomNumberGenerator.GetInt32;
        _now = now ?? (() => DateTimeOffset.UtcNow);
        Directory.CreateDirectory(Path.GetDirectoryName(Path.GetFullPath(path))!);
        _journal = new FileStream(path, FileMode.OpenOrCreate, FileAccess.ReadWrite, FileShare.Read);
        try
        {
            if (!OperatingSystem.IsWindows()) File.SetUnixFileMode(path, UnixFileMode.UserRead | UnixFileMode.UserWrite);
            using var bytes = new MemoryStream();
            _journal.CopyTo(bytes);
            var content = bytes.GetBuffer().AsSpan(0, checked((int)bytes.Length));
            var start = 0;
            for (var i = 0; i < content.Length; i++)
            {
                if (content[i] != (byte)'\n') continue;
                if (i > start)
                {
                    var binding = JsonSerializer.Deserialize(content[start..i], JsonContext.Default.PoolBinding)
                        ?? throw new InvalidDataException("Invalid session affinity journal.");
                    if (string.IsNullOrEmpty(binding.Key) || binding.Key.Length != 64 || !binding.Key.All(Uri.IsHexDigit)
                        || string.IsNullOrEmpty(binding.MemberId) || string.IsNullOrEmpty(binding.SourceId) || string.IsNullOrEmpty(binding.Model))
                        throw new InvalidDataException("Invalid session affinity key.");
                    _bindings[binding.Key] = binding;
                }
                start = i + 1;
            }
            // A crash before the terminating newline did not commit an assignment.
            _journal.SetLength(start); _journal.Position = start;
        }
        catch { _journal.Dispose(); throw; }
    }

    internal static string Key(SourcePool pool, string session) => Convert.ToHexString(
        SHA256.HashData(Encoding.UTF8.GetBytes(pool.Agent + "\n" + pool.Id + "\n" + session)));

    public PoolMember Select(SourcePool pool, string session, ISet<string>? excluded = null, bool allowReassign = true)
    {
        lock (_gate)
        {
            var key = Key(pool, session);
            if (_bindings.TryGetValue(key, out var binding))
            {
                var pinned = pool.Members.FirstOrDefault(member => member.Id == binding.MemberId
                    && member.SourceId == binding.SourceId && member.Model == binding.Model);
                if (pinned is not null && excluded?.Contains(pinned.Id) != true
                    && (!pool.Failover || !Cooling(pool, pinned))) return pinned;
            }
            if (!allowReassign) throw new PoolHistoryRequiredException();
            var candidates = pool.Members.Where(member => member.Weight > 0
                && excluded?.Contains(member.Id) != true && !Cooling(pool, member)).ToArray();
            if (candidates.Length == 0) throw new InvalidOperationException("No source pool member is currently available. Retry after its cooldown.");
            var position = _draw(candidates.Sum(member => member.Weight));
            var selected = candidates[^1];
            foreach (var member in candidates)
            {
                if (position < member.Weight) { selected = member; break; }
                position -= member.Weight;
            }
            var next = new PoolBinding { Key = key, MemberId = selected.Id, SourceId = selected.SourceId, Model = selected.Model };
            var data = JsonSerializer.SerializeToUtf8Bytes(next, JsonContext.Default.PoolBinding);
            var length = _journal.Length;
            try
            {
                _journal.Position = length; _journal.Write(data); _journal.WriteByte((byte)'\n'); _journal.Flush(flushToDisk: true);
            }
            catch { _journal.SetLength(length); throw; }
            _bindings[key] = next;
            return selected;
        }
    }

    public void Failed(SourcePool pool, PoolMember member, TimeSpan? retryAfter = null)
    {
        lock (_gate)
            _cooldowns[CooldownKey(pool, member)] = _now().AddSeconds(Math.Clamp(retryAfter?.TotalSeconds ?? 30, 1, 900));
    }
    private bool Cooling(SourcePool pool, PoolMember member) =>
        _cooldowns.TryGetValue(CooldownKey(pool, member), out var until) && until > _now();
    private static string CooldownKey(SourcePool pool, PoolMember member) =>
        pool.Id + "/" + member.Id + "/" + member.SourceId + "/" + member.Model;
    public void Dispose() { lock (_gate) _journal.Dispose(); }
}
