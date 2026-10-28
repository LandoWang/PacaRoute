using System.Text.Json;
using CopilotBridge.Cli.Desktop;
using CopilotBridge.Cli.Hosting.ClientConfig;
using CopilotBridge.Cli.Models;
using Xunit;

namespace CopilotBridge.UnitTests;

public sealed class DesktopContractTests
{
    [Fact]
    public void ModelSelection_PreservesCodexCommentsProvidersAndPermissions()
    {
        const string original = "# personal comment\nmodel = \"old\" # keep\napproval_policy = \"on-request\"\n[model_providers.other]\nbase_url = \"https://example.test\"\n";
        var updated = DesktopClientConfiguration.WithModel("codex", original, "gpt-6.1-sol");
        Assert.Contains("# personal comment", updated);
        Assert.Contains("# keep", updated);
        Assert.Contains("approval_policy = \"on-request\"", updated);
        Assert.Contains("[model_providers.other]\nbase_url = \"https://example.test\"", updated);
        Assert.Contains("model = \"gpt-6.1-sol\"", updated);
        Assert.Equal(updated, DesktopClientConfiguration.WithModel("codex", updated, "gpt-6.1-sol"));
    }

    [Fact]
    public void ModelSelection_PreservesClaudeUnrelatedSettings()
    {
        const string original = "{\"env\":{\"ANTHROPIC_AUTH_TOKEN\":\"preserve-me\",\"EXTRA\":\"yes\"},\"permissions\":{\"allow\":[\"Read\"]}}";
        var updated = DesktopClientConfiguration.WithModel("claude-code", original, "claude-opus-5.5");
        using var doc = JsonDocument.Parse(updated);
        Assert.Equal("preserve-me", doc.RootElement.GetProperty("env").GetProperty("ANTHROPIC_AUTH_TOKEN").GetString());
        Assert.Equal("yes", doc.RootElement.GetProperty("env").GetProperty("EXTRA").GetString());
        Assert.Equal("claude-opus-5.5", doc.RootElement.GetProperty("env").GetProperty("ANTHROPIC_MODEL").GetString());
        Assert.Equal("Read", doc.RootElement.GetProperty("permissions").GetProperty("allow")[0].GetString());
    }

    [Theory]
    [InlineData("model\"\napproval_policy=\"never")]
    [InlineData("")]
    [InlineData("has spaces")]
    public void ModelSelection_RejectsConfigInjection(string id) =>
        Assert.Throws<ArgumentException>(() => DesktopClientConfiguration.WithModel("codex", "", id));

    [Fact]
    public void Apply_RejectsStalePreviewWithoutTouchingExternalChanges()
    {
        var directory = Path.Combine(Path.GetTempPath(), "bridge-desktop-test-" + Guid.NewGuid());
        Directory.CreateDirectory(directory);
        try
        {
            var path = Path.Combine(directory, "config.toml");
            File.WriteAllText(path, "external edit");
            var plan = new ConfigPlan("codex", ConfigScope.Global, path, "new", "old", []);
            Assert.Throws<InvalidOperationException>(() => DesktopClientConfiguration.Apply(plan,
                DesktopClientConfiguration.Fingerprint("old")));
            Assert.Equal("external edit", File.ReadAllText(path));
            Assert.Empty(Directory.GetFiles(directory, "*.bak"));
        }
        finally { Directory.Delete(directory, recursive: true); }
    }

    [Fact]
    public void Apply_CreatesPrivateIndependentBackupsAndPreservesOriginalBytes()
    {
        var directory = Path.Combine(Path.GetTempPath(), "bridge-desktop-test-" + Guid.NewGuid());
        Directory.CreateDirectory(directory);
        try
        {
            var path = Path.Combine(directory, "settings.json");
            File.WriteAllText(path, "original secret");
            var first = DesktopClientConfiguration.Apply(new ConfigPlan("claude-code", ConfigScope.Global,
                path, "first", "original secret", []), DesktopClientConfiguration.Fingerprint("original secret"));
            var second = DesktopClientConfiguration.Apply(new ConfigPlan("claude-code", ConfigScope.Global,
                path, "second", "first", []), DesktopClientConfiguration.Fingerprint("first"));
            Assert.NotEqual(first, second);
            Assert.Equal("original secret", File.ReadAllText(first!));
            Assert.Equal("first", File.ReadAllText(second!));
            Assert.Equal("second", File.ReadAllText(path));
            if (!OperatingSystem.IsWindows())
            {
                Assert.Equal(UnixFileMode.UserRead | UnixFileMode.UserWrite, File.GetUnixFileMode(first!));
                Assert.Equal(UnixFileMode.UserRead | UnixFileMode.UserWrite, File.GetUnixFileMode(path));
            }
        }
        finally { Directory.Delete(directory, recursive: true); }
    }

    [Fact]
    public void DirectProxy_BypassesEvenPublicInternetAddresses() =>
        Assert.True(DesktopNetwork.CreateProxy("direct", null).IsBypassed(new Uri("https://api.github.com")));

    [Theory]
    [InlineData("file:///tmp/proxy")]
    [InlineData("http://user:secret@localhost:7890")]
    [InlineData("http://localhost:7890/path")]
    public void CustomProxy_RejectsUnexpectedSchemesAndEmbeddedSecrets(string address) =>
        Assert.Throws<ArgumentException>(() => DesktopNetwork.CreateProxy("custom", address));

    [Fact]
    public void CustomProxy_UsesSelectedEndpoint()
    {
        var proxy = DesktopNetwork.CreateProxy("custom", "socks5://127.0.0.1:1080");
        Assert.Equal(new Uri("socks5://127.0.0.1:1080"), proxy.GetProxy(new Uri("https://api.github.com")));
    }

    [Fact]
    public void UnknownQuotaFields_StayNull()
    {
        var quota = JsonSerializer.Deserialize("{\"quota_snapshots\":{\"premium_interactions\":{\"unlimited\":false}}}",
            JsonContext.Default.CopilotAccountUsage)!;
        var premium = quota.QuotaSnapshots!["premium_interactions"];
        Assert.Null(premium.Remaining);
        Assert.Null(premium.PercentRemaining);
        Assert.False(premium.Unlimited);
    }

    [Fact]
    public void ErrorOutput_RedactsCommonCredentialShapes()
    {
        var safe = DesktopTelemetry.Sanitize("Authorization: Bearer private-token ghp_placeholder sk-placeholder");
        Assert.DoesNotContain("private-token", safe);
        Assert.DoesNotContain("ghp_placeholder", safe);
        Assert.DoesNotContain("sk-placeholder", safe);
    }
}
