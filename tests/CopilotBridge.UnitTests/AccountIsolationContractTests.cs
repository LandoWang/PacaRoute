using System.Net;
using CopilotBridge.Cli.Auth;
using CopilotBridge.Cli.Copilot;
using CopilotBridge.Cli.Hosting.Options;
using CopilotBridge.Cli.Models.GitHub;
using CopilotBridge.Cli.Sources;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Options;
using Xunit;

namespace CopilotBridge.UnitTests;

/// <summary>Two identities may expose the same model IDs. Authentication, logout,
/// catalog caches and Azure CLI sessions must still belong to the selected account.</summary>
public sealed class AccountIsolationContractTests : IDisposable
{
    private readonly string _root = Path.Combine(Path.GetTempPath(), "bridge-accounts-" + Guid.NewGuid().ToString("N"));
    private AccountRegistry Registry() => new([
        new("github-one", "One", "github"), new("github-two", "Two", "github"),
        new("azure-one", "Work", "azure"), new("azure-two", "Personal", "azure")], _root);

    [Fact]
    public async Task LoginStoresAndLogoutAreIndependent()
    {
        var registry = Registry();
        Seed(registry, "github-one", "fixture-one"); Seed(registry, "github-two", "fixture-two");
        using var one = registry.CreateGitHub("github-one", new Factory());
        using var two = registry.CreateGitHub("github-two", new Factory());
        Assert.Equal("fixture-one", await one.EnsureGitHubTokenAsync());
        Assert.Equal("fixture-two", await two.EnsureGitHubTokenAsync());
        Assert.NotEqual(one.TokenLocation, two.TokenLocation);
        one.SignOut();
        Assert.False(one.IsAuthenticated);
        Assert.True(two.IsAuthenticated);
        Assert.Equal("fixture-two", await two.EnsureGitHubTokenAsync());
        Assert.False(File.Exists(one.TokenLocation));
        Assert.True(File.Exists(two.TokenLocation));
    }

    [Fact]
    public async Task CatalogRefreshAndRequestSelectionCannotLeakAcrossAccounts()
    {
        var registry = Registry();
        Seed(registry, "github-one", "fixture-one"); Seed(registry, "github-two", "fixture-two");
        var collection = new ServiceCollection().AddLogging();
        collection.AddSingleton<IHttpClientFactory, Factory>();
        collection.AddSingleton<CopilotHeaderFactory>();
        collection.AddSingleton(Options.Create(new UpstreamRetryOptions()));
        collection.AddSingleton(Options.Create(new UpstreamTimeoutOptions()));
        collection.AddSingleton(Options.Create(new CodexModelCatalogOptions()));
        using var provider = collection.BuildServiceProvider();
        using var fallback = registry.CreateGitHub("github-one", new Factory());
        using var services = new AccountServices(registry, fallback, provider);
        var one = services.For("github-one"); var two = services.For("github-two");
        var results = await Task.WhenAll(one.Overlay.GetAsync().AsTask(), two.Overlay.GetAsync().AsTask());
        Assert.Equal("one", Assert.Single(results[0].Models).Name);
        Assert.Equal("two", Assert.Single(results[1].Models).Name);
        Assert.Equal("shared-model", Assert.Single(results[0].Models).Id);
        Assert.Equal("shared-model", Assert.Single(results[1].Models).Id);
        Assert.Equal("one", Assert.Single((await one.Overlay.GetAsync()).Models).Name);
        var context = new HttpContextAccessor { HttpContext = new DefaultHttpContext() };
        var auth = new RequestAccountAuth(services, context);
        context.HttpContext.Items[typeof(ModelSource)] = Copilot("source-two", "github-two");
        Assert.Equal("fixture-two", await auth.EnsureGitHubTokenAsync());
        context.HttpContext.Items[typeof(ModelSource)] = Copilot("source-one", "github-one");
        Assert.Equal("fixture-one", await auth.EnsureGitHubTokenAsync());
        context.HttpContext = null;
    }

    [Fact]
    public void AzureResourcesShareTheirAccountSessionButNeverAnotherAccountsSession()
    {
        var registry = Registry();
        var first = AzureAccountService.StartInfo(registry, "azure-one", ["account", "list"]);
        var secondResource = AzureAccountService.StartInfo(registry, "azure-one", ["account", "get-access-token"]);
        var other = AzureAccountService.StartInfo(registry, "azure-two", ["account", "list"]);
        Assert.Equal(first.Environment["AZURE_CONFIG_DIR"], secondResource.Environment["AZURE_CONFIG_DIR"]);
        Assert.NotEqual(first.Environment["AZURE_CONFIG_DIR"], other.Environment["AZURE_CONFIG_DIR"]);
        Assert.StartsWith(_root, first.Environment["AZURE_CONFIG_DIR"]);
        Assert.Null(registry.AzureDirectory("azure-local"));
        Assert.Throws<ArgumentException>(() => registry.AzureDirectory("github-one"));
    }

    [Theory]
    [InlineData("../escape")]
    [InlineData("copilot")]
    [InlineData("azure-local")]
    public void UntrustedMetadataCannotEscapeStorageOrReplaceLegacyIdentity(string id)
        => Assert.Throws<ArgumentException>(() => new AccountRegistry([new(id, "Bad", "github")], _root));

    private static ModelSource Copilot(string id, string account) => new()
        { Id = id, Name = id, Kind = "copilot", AccountId = account, Endpoint = "", Deployments = [] };

    private static void Seed(AccountRegistry registry, string account, string token) =>
        TokenStore.CreateAccountCredentialStore(registry.DirectoryFor(account)).Save(new CredentialFileRecord {
            Version = CredentialFileRecord.GitHubCliOAuthVersion, AccessToken = token, CredentialId = account, Generation = 1 });

    private sealed class Factory : IHttpClientFactory
    {
        public HttpClient CreateClient(string name) => new(new Handler());
    }
    private sealed class Handler : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken ct)
        {
            var name = request.Headers.Authorization?.Parameter == "fixture-one" ? "one" : "two";
            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK) {
                Content = new StringContent("{\"data\":[{\"id\":\"shared-model\",\"name\":\"" + name + "\"}]}", System.Text.Encoding.UTF8, "application/json") });
        }
    }
    public void Dispose() { if (Directory.Exists(_root)) Directory.Delete(_root, true); }
}
