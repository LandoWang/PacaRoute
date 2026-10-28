using System.Diagnostics;
using System.Net;
using CopilotBridge.Cli.Sources;
using Xunit;

namespace CopilotBridge.UnitTests;

public sealed class ProxyProfileContractTests
{
    private static ProxyRegistry Registry() => new(new ProxyConfiguration {
        Profiles = [new("office", "Office", "custom", "http://127.0.0.1:7788")],
        Bindings = new() { ["azure-one"] = "office", ["github-two"] = "direct" }, AccountProxyId = "office" });
    [Fact]
    public void SourceBindingsAreIndependentAndMissingBindingsUseSystem()
    {
        var registry = Registry();
        using var proxy = registry.CreateHandler(registry.ForSource("azure-one"));
        using var direct = registry.CreateHandler(registry.ForSource("github-two"));
        using var system = registry.CreateHandler(registry.ForSource("new-source"));
        Assert.Equal(new Uri("http://127.0.0.1:7788"), proxy.Proxy!.GetProxy(new Uri("https://example.com")));
        Assert.False(direct.UseProxy);
        Assert.True(system.UseProxy); Assert.Null(system.Proxy);
        Assert.False(proxy.AllowAutoRedirect);
    }
    [Theory]
    [InlineData("http://user:secret@localhost:80")]
    [InlineData("file:///tmp/proxy")]
    [InlineData("https://host/path")]
    [InlineData("https://host?key=value")]
    public void InvalidProxyCannotStartOrExposeCredentials(string address) => Assert.Throws<ArgumentException>(() =>
        new ProxyRegistry(new() { Profiles = [new("bad", "Bad", "custom", address)] }));
    [Fact]
    public void MissingProfileCannotSilentlyFallBackToDirect() => Assert.Throws<ArgumentException>(() =>
        new ProxyRegistry(new() { Bindings = new() { ["work"] = "deleted" } }));
    [Fact]
    public void AzureSubprocessGetsSourceChoiceWithoutChangingParentEnvironment()
    {
        var registry = Registry(); var before = Environment.GetEnvironmentVariable("HTTPS_PROXY");
        var start = new ProcessStartInfo(); registry.Apply(start, registry.ForAccount);
        Assert.Equal("http://127.0.0.1:7788", start.Environment["HTTPS_PROXY"]);
        registry.Apply(start, "direct");
        Assert.False(start.Environment.ContainsKey("HTTPS_PROXY")); Assert.Equal("*", start.Environment["NO_PROXY"]);
        registry.Apply(start, "system");
        Assert.Equal(before, (start.Environment.TryGetValue("HTTPS_PROXY", out var restored) ? restored : null));
        Assert.Equal(before, Environment.GetEnvironmentVariable("HTTPS_PROXY"));
    }
}
