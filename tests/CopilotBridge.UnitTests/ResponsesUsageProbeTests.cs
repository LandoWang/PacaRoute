using System.Text;
using CopilotBridge.Cli.Models;
using Xunit;

namespace CopilotBridge.UnitTests;

public class ResponsesUsageProbeTests
{
    [Theory]
    [InlineData("response.completed")]
    [InlineData("response.incomplete")]
    [InlineData("response.failed")]
    public void TerminalUsage_UsesCumulativeTotals_WithoutAddingSubsets(string type)
    {
        var snapshot = new UsageSnapshot();
        var data = """
            {"type":"TERMINAL","response":{"usage":{"input_tokens":1200,"output_tokens":150,"input_tokens_details":{"cached_tokens":1000},"output_tokens_details":{"reasoning_tokens":120}}}}
            """.Replace("TERMINAL", type, StringComparison.Ordinal);
        UsageProbe.TryUpdateFromResponsesEvent(type, data, snapshot);
        UsageProbe.TryUpdateFromResponsesEvent(null, data, snapshot);
        UsageProbe.TryUpdateFromResponsesEvent("", data, snapshot);
        Assert.Equal(1200, snapshot.InputTokens);
        Assert.Equal(150, snapshot.OutputTokens);
        Assert.Equal(1000, snapshot.CacheReadInputTokens);
        Assert.Null(snapshot.CacheCreationInputTokens);
    }

    [Fact]
    public void BufferedMissingCounters_StayUnknown_AndExplicitZeroStaysZero()
    {
        var snapshot = new UsageSnapshot();
        UsageProbe.TryReadResponsesBuffered("""{"usage":{"output_tokens":0}}"""u8, snapshot);
        Assert.Null(snapshot.InputTokens);
        Assert.Equal(0, snapshot.OutputTokens);
        Assert.Null(snapshot.CacheReadInputTokens);
    }

    [Theory]
    [InlineData("not json")]
    [InlineData("null")]
    [InlineData("{\"usage\":null}")]
    [InlineData("{\"usage\":{\"input_tokens\":-1}}")]
    [InlineData("{\"usage\":{\"input_tokens\":\"invalid\"}}")]
    public void InvalidUsage_DoesNotErasePreviouslyObservedCounters(string body)
    {
        var snapshot = new UsageSnapshot { InputTokens = 23, OutputTokens = 8 };
        UsageProbe.TryReadResponsesBuffered(Encoding.UTF8.GetBytes(body), snapshot);
        Assert.Equal(23, snapshot.InputTokens);
        Assert.Equal(8, snapshot.OutputTokens);
    }

    [Fact]
    public void ProvisionalAndMalformedEvents_DoNotInventUsage()
    {
        var snapshot = new UsageSnapshot();
        UsageProbe.TryUpdateFromResponsesEvent("response.created",
            """{"response":{"usage":{"input_tokens":0,"output_tokens":0}}}""", snapshot);
        UsageProbe.TryUpdateFromResponsesEvent("response.completed", "not json", snapshot);
        UsageProbe.TryUpdateFromResponsesEvent("response.completed", """{"response":{"usage":null}}""", snapshot);
        Assert.False(snapshot.HasAny);
    }
}
