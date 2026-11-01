using System.Text.Json;
using CopilotBridge.Cli.Models;
using CopilotBridge.Cli.Sources;
using Microsoft.AspNetCore.Http;
using Xunit;

namespace CopilotBridge.UnitTests;

public sealed class SourcePoolAffinityTests : IDisposable
{
    private readonly string _root = Path.Combine(Path.GetTempPath(), "pool-affinity-" + Guid.NewGuid().ToString("N"));
    private string Journal => Path.Combine(_root, "sessions.jsonl");
    private static SourcePool Pool => new() { Id = "gpt", Name = "GPT", Agent = "codex", Model = "gpt-6-astra", Members = [
        new() { Id = "a", SourceId = "copilot", Model = "gpt-6-astra", Weight = 6 },
        new() { Id = "b", SourceId = "copilot", Model = "gpt-6-sol", Weight = 3 },
        new() { Id = "c", SourceId = "copilot", Model = "gpt-6.1-sol", Weight = 1 }] };

    [Fact]
    public void WeightsApplyToNewSessionsAndNeverToEachRequest()
    {
        var draws = 0;
        using var store = new PoolAffinityStore(Journal, total => draws++ % total);
        var counts = new Dictionary<string, int>();
        for (var i = 0; i < 100; i++)
        {
            var session = "conversation-" + i;
            var first = store.Select(Pool, session);
            counts[first.Id] = counts.GetValueOrDefault(first.Id) + 1;
            for (var turn = 0; turn < 4; turn++) Assert.Equal(first, store.Select(Pool, session));
        }
        Assert.Equal(100, draws);
        Assert.Equal(60, counts["a"]); Assert.Equal(30, counts["b"]); Assert.Equal(10, counts["c"]);
        Assert.DoesNotContain("conversation-", File.ReadAllText(Journal));
    }

    [Fact]
    public void RestartAndWeightChangesRetainExistingBindingsIncludingZeroWeight()
    {
        using (var first = new PoolAffinityStore(Journal, _ => 0)) Assert.Equal("a", first.Select(Pool, "existing").Id);
        var changed = Pool with { Members = Pool.Members.Select(m => m with { Weight = m.Id == "a" ? 0 : 1 }).ToArray() };
        using var reopened = new PoolAffinityStore(Journal, _ => 0);
        Assert.Equal("a", reopened.Select(changed, "existing").Id);
        Assert.Equal("b", reopened.Select(changed, "new").Id);
        Assert.Equal("b", reopened.Select(changed with { Id = "other" }, "existing").Id);
    }

    [Fact]
    public void FailureMovesOnlyToEligibleMembersAndNewBindingSurvivesRestart()
    {
        var now = DateTimeOffset.UtcNow;
        using (var store = new PoolAffinityStore(Journal, _ => 0, () => now))
        {
            var original = store.Select(Pool, "same");
            store.Failed(Pool, original, TimeSpan.FromSeconds(60));
            Assert.Equal("b", store.Select(Pool, "same").Id);
            Assert.Equal("b", store.Select(Pool, "another").Id);
            now = now.AddMinutes(2);
            Assert.Equal("b", store.Select(Pool, "same").Id);
            Assert.Equal("a", store.Select(Pool, "fresh").Id);
        }
        using var reopened = new PoolAffinityStore(Journal, _ => 0);
        Assert.Equal("b", reopened.Select(Pool, "same").Id);
    }

    [Fact]
    public void NoAvailableMemberDoesNotSpinOrIgnoreCooldown()
    {
        using var store = new PoolAffinityStore(Journal, _ => 0);
        foreach (var member in Pool.Members) store.Failed(Pool, member);
        Assert.Throws<InvalidOperationException>(() => store.Select(Pool, "new"));
    }

    [Fact]
    public void ConcurrentFirstRequestsUseOneBinding()
    {
        var draws = 0;
        using var store = new PoolAffinityStore(Journal, total => Interlocked.Increment(ref draws) % total);
        var selected = Enumerable.Range(0, 30).AsParallel().Select(_ => store.Select(Pool, "same").Id).ToArray();
        Assert.Single(selected.Distinct()); Assert.Equal(1, draws);
    }

    [Fact]
    public void InterruptedJournalTailCannotCorruptTheNextCommittedBinding()
    {
        using (var store = new PoolAffinityStore(Journal, _ => 0)) store.Select(Pool, "one");
        File.AppendAllText(Journal, "{\"key\":\"incomplete");
        using (var store = new PoolAffinityStore(Journal, _ => 7)) Assert.Equal("b", store.Select(Pool, "two").Id);
        using var reopened = new PoolAffinityStore(Journal, _ => 9);
        Assert.Equal("a", reopened.Select(Pool, "one").Id); Assert.Equal("b", reopened.Select(Pool, "two").Id);
    }

    [Fact]
    public void ClientSessionIdentityNeverUsesRequestIdOrPrompt()
    {
        using var empty = JsonDocument.Parse("{\"model\":\"gpt-6-astra\",\"input\":\"same prompt\"}");
        var headers = new HeaderDictionary { ["x-client-request-id"] = "changes-each-request" };
        Assert.Null(PoolSessionIdentity.Read("codex", headers, empty.RootElement));
        headers["session-id"] = "runtime-session"; headers["thread-id"] = "persistent-conversation";
        Assert.Equal("persistent-conversation", PoolSessionIdentity.Read("codex", headers, empty.RootElement));
        using var claude = JsonDocument.Parse("""{"metadata":{"user_id":"{\"device_id\":\"device\",\"session_id\":\"conversation\"}"}}""");
        Assert.Equal("conversation", PoolSessionIdentity.Read("claude-code", new HeaderDictionary(), claude.RootElement));
        using var legacy = JsonDocument.Parse("""{"metadata":{"user_id":"user_device_account__session_1234-5678"}}""");
        Assert.Equal("1234-5678", PoolSessionIdentity.Read("claude-code", new HeaderDictionary(), legacy.RootElement));
    }

    [Fact]
    public void PoolConfigurationUsesAotJsonAndValidatesMembership()
    {
        var json = JsonSerializer.Serialize(new[] { Pool }, JsonContext.Default.SourcePoolArray);
        var decoded = JsonSerializer.Deserialize(json, JsonContext.Default.SourcePoolArray)!;
        SourcePoolRegistry.Validate(decoded, new SourceRegistry([]));
        Assert.Equal(6, decoded[0].Members[0].Weight);
        Assert.Throws<ArgumentException>(() => SourcePoolRegistry.Validate([Pool with { Members = [Pool.Members[0] with { Weight = 0 }] }], new SourceRegistry([])));
        Assert.Throws<ArgumentException>(() => SourcePoolRegistry.Validate([Pool with { Members = [Pool.Members[0] with { SourceId = "unknown" }] }], new SourceRegistry([])));
    }
    public void Dispose() { if (Directory.Exists(_root)) Directory.Delete(_root, true); }
}
