using System.Text;
using System.Text.Json;
using CopilotBridge.Cli.Endpoints.ClaudeCode;
using CopilotBridge.Cli.Hosting.ClientConfig;
using CopilotBridge.Cli.Models;
using CopilotBridge.Cli.Models.Anthropic.Request;
using CopilotBridge.Cli.Models.Common;
using CopilotBridge.Cli.Pipeline;
using CopilotBridge.Cli.Pipeline.Routing;
using CopilotBridge.Cli.Pipeline.Strategies.Codex;
using CopilotBridge.Cli.Sources;
using Xunit;

namespace CopilotBridge.UnitTests;

/// <summary>Source is independent of harness and opaque deployment IDs; Copilot
/// policies must never be applied to an Azure source, even for an identical model ID.</summary>
public sealed class SourceHarnessContractTests
{
    private static ModelSource Source(string model = "gpt-5.3-codex") => new()
    {
        Id = "azure-dev", Name = "Azure dev", Endpoint = "https://example.openai.azure.com/",
        Deployments = [model],
    };

    [Fact]
    public void SourceJsonKeepsDocumentedDefaults()
    {
        var sources = JsonSerializer.Deserialize("[{\"id\":\"azure-dev\",\"name\":\"Dev\",\"endpoint\":\"https://example.openai.azure.com\",\"auth_mode\":\"api-key\",\"deployments\":[\"test\"]}]", JsonContext.Default.ModelSourceArray)!;
        Assert.Equal("azure-openai", sources[0].Kind);
        Assert.Equal("api-key", sources[0].AuthMode);
    }

    [Theory]
    [InlineData("https://example.openai.azure.com")]
    [InlineData("https://example.openai.azure.com/openai/v1/")]
    public void ResourceEndpointAndV1EndpointResolveToOneResponsesUrl(string endpoint)
    {
        var source = new SourceRegistry([Source() with { Endpoint = endpoint }]).Sources.Single();
        Assert.Equal("https://example.openai.azure.com/openai/v1/responses", source.ResponsesUri.AbsoluteUri);
    }

    [Theory]
    [InlineData("http://example.openai.azure.com")]
    [InlineData("https://secret@example.openai.azure.com")]
    [InlineData("https://example.openai.azure.com/?api-key=secret")]
    [InlineData("https://example.openai.azure.com/openai/deployments/wrong")]
    public void RejectsCredentialBearingAndWrongProtocolEndpoints(string endpoint)
        => Assert.Throws<ArgumentException>(() => new SourceRegistry([Source() with { Endpoint = endpoint }]));

    [Fact]
    public void BothHarnessesBindToTheSameSourceWithoutChangingCopilotDefaults()
    {
        Assert.Equal("http://localhost:8765/sources/azure-dev/codex", new BridgeConnection(8765, "azure-dev").CodexBaseUrl);
        Assert.Equal("http://localhost:8765/sources/azure-dev/cc", new BridgeConnection(8765, "azure-dev").ClaudeCodeBaseUrl);
        Assert.Equal("http://localhost:8765/codex", new BridgeConnection(8765).CodexBaseUrl);
        Assert.Equal("http://localhost:8765/cc", new BridgeConnection(8765).ClaudeCodeBaseUrl);
    }

    [Theory]
    [InlineData("my-company-deployment")]
    [InlineData("gpt-5.3-codex")]
    [InlineData("gpt-4-1-2026-01-01")]
    public void AzureRoutingPreservesOpaqueDeploymentAndNeverRunsCopilotLocations(string model)
    {
        var ctx = new BridgeContext<MessagesRequest>
        {
            Source = Source(model),
            Request = new BridgeRequest<MessagesRequest> { Method = "POST", Path = "/cc/v1/messages",
                Body = new MessagesRequest { Model = model, Messages = [], OutputConfig = new OutputConfig { Effort = "max" } } },
        };
        var rules = new RoutesConfig { Locations = [new RouteLocation {
            When = new MatchExpression { Model = "*" }, Use = new LocationUse { Model = "claude-opus-5" } }] };
        var target = CountTokensTestServices.Planner(rules).Plan(ctx);
        Assert.Equal(BackendVendor.AzureResponses, target.Vendor);
        Assert.Equal(model, target.ModelId);
        Assert.Equal(model, ctx.Request.Body.Model);
        Assert.Equal("max", ctx.Request.Body.OutputConfig?.Effort);
    }

    [Fact]
    public void UnknownDeploymentCannotFallBackToCopilot()
    {
        using var request = JsonDocument.Parse("{\"model\":\"other\"}");
        Assert.Throws<ArgumentException>(() => SourceRouting.ReadModel(request.RootElement, Source()));
    }

    [Fact]
    public void AzureTranslationDoesNotApplyCopilotServiceTierStoreOrToolCoercions()
    {
        using var extensions = JsonDocument.Parse("""
            {"service_tier":"default","store":true,"tools":[{"type":"image_generation"},{"type":"custom","name":"exec","format":{"type":"text"}}]}
            """);
        var ir = new MessagesRequest { Model = "gpt-5.3-codex", Messages = [],
            OutputConfig = new OutputConfig { Effort = "provider-specific" },
            ProviderExtensions = new ProviderExtensions { ByProvider = new() { [ProviderExtensions.OpenAiNamespace] = extensions.RootElement.Clone() } } };
        var result = ResponsesRequestBuilder.Build(ir, new CodexModelProfileCatalog(), false,
            out _, out _, out var mutations, applyCopilotPolicy: false);
        using var doc = JsonDocument.Parse(result.Body);
        Assert.Equal("default", doc.RootElement.GetProperty("service_tier").GetString());
        Assert.True(doc.RootElement.GetProperty("store").GetBoolean());
        Assert.Equal(2, doc.RootElement.GetProperty("tools").GetArrayLength());
        Assert.Equal("provider-specific", doc.RootElement.GetProperty("reasoning").GetProperty("effort").GetString());
        Assert.Equal(ResponsesRequestMutation.None, mutations);
    }

    [Fact]
    public void ChunkedStreamCountsUsageAndReportsIncompleteTransport()
    {
        var summary = new RequestSummary { Kind = "responses", Source = "azure-dev", StatusCode = 200 };
        var observer = new ResponsesUsageObserver(summary, true);
        var data = Encoding.UTF8.GetBytes("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":123,\"output_tokens\":45,\"input_tokens_details\":{\"cached_tokens\":67}}}}\n\n");
        foreach (var b in data) observer.Append([b]);
        observer.Complete();
        Assert.Equal(123, summary.Usage.InputTokens);
        Assert.Equal(45, summary.Usage.OutputTokens);
        Assert.Equal(67, summary.Usage.CacheReadInputTokens);
        Assert.Equal(200, summary.StatusCode);
        var broken = new RequestSummary { Kind = "responses", StatusCode = 200 };
        var truncated = new ResponsesUsageObserver(broken, true);
        truncated.Append("data: {\"type\":\"response.created\"}\n"u8);
        truncated.Complete();
        Assert.Equal(502, broken.StatusCode);
    }
}
