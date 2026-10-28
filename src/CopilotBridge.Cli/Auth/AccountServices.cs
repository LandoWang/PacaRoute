using System.Collections.Concurrent;
using CopilotBridge.Cli.Catalogs.Codex;
using CopilotBridge.Cli.Copilot;
using CopilotBridge.Cli.Hosting.Options;
using CopilotBridge.Cli.Sources;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;

namespace CopilotBridge.Cli.Auth;

/// <summary>Account-bound services never depend on the ambient request after selection.</summary>
internal sealed class AccountServices(AccountRegistry accounts, AuthService defaultAuth, IServiceProvider services) : IDisposable
{
    private readonly ConcurrentDictionary<string, Lazy<Entry>> _entries = new();
    internal sealed record Entry(AuthService Auth, ICopilotClient Client, CodexCatalogOverlayService Overlay);

    public Entry For(string id) => _entries.GetOrAdd(id, key => new Lazy<Entry>(() =>
    {
        var factory = services.GetRequiredService<IHttpClientFactory>();
        var auth = key == "copilot" ? defaultAuth : accounts.CreateGitHub(key, factory);
        var client = new CopilotClient(factory, auth, services.GetRequiredService<CopilotHeaderFactory>(),
            services.GetRequiredService<IOptions<UpstreamRetryOptions>>(), services.GetRequiredService<IOptions<UpstreamTimeoutOptions>>(),
            services.GetRequiredService<ILogger<CopilotClient>>());
        return new Entry(auth, client, new CodexCatalogOverlayService(client,
            services.GetRequiredService<ILogger<CodexCatalogOverlayService>>(), services.GetRequiredService<IOptions<CodexModelCatalogOptions>>()));
    })).Value;

    public static string Selected(IHttpContextAccessor accessor) =>
        (accessor.HttpContext?.Items[typeof(ModelSource)] as ModelSource)?.AccountId ?? "copilot";

    public static void Register(IServiceCollection services, SourceRegistry sources)
    {
        var accounts = AccountRegistry.Load();
        foreach (var source in sources.Sources)
        {
            var account = accounts.Find(source.AccountId ?? "azure-local");
            if (account.Kind != (source.Kind == "copilot" ? "github" : "azure"))
                throw new ArgumentException("Source/account provider mismatch.");
        }
        services.AddSingleton(accounts);
        services.AddHttpContextAccessor();
        services.AddSingleton<AccountServices>();
        services.RemoveAll<IAuthService>();
        services.RemoveAll<ICopilotClient>();
        services.RemoveAll<CodexCatalogOverlayService>();
        services.AddSingleton<IAuthService, RequestAccountAuth>();
        services.AddScoped<ICopilotClient>(sp => sp.GetRequiredService<AccountServices>()
            .For(Selected(sp.GetRequiredService<IHttpContextAccessor>())).Client);
        services.AddScoped<CodexCatalogOverlayService>(sp => sp.GetRequiredService<AccountServices>()
            .For(Selected(sp.GetRequiredService<IHttpContextAccessor>())).Overlay);
    }

    public void Dispose()
    {
        foreach (var pair in _entries)
            if (pair.Key != "copilot" && pair.Value.IsValueCreated) pair.Value.Value.Auth.Dispose();
    }
}

internal sealed class RequestAccountAuth(AccountServices services, IHttpContextAccessor accessor) : IAuthService
{
    private AuthService Current => services.For(AccountServices.Selected(accessor)).Auth;
    public bool IsAuthenticated => Current.IsAuthenticated;
    public string TokenLocation => Current.TokenLocation;
    public string? CopilotApiBaseUrl => Current.CopilotApiBaseUrl;
    public DateTimeOffset? CopilotTokenExpiry => Current.CopilotTokenExpiry;
    public ValueTask<string> EnsureGitHubTokenAsync(CancellationToken ct = default) => Current.EnsureGitHubTokenAsync(ct);
    public ValueTask<CopilotAuthLease> GetCopilotTokenAsync(CopilotLeaseRejection? rejection = null, CancellationToken ct = default)
        => Current.GetCopilotTokenAsync(rejection, ct);
    public void SignOut() => Current.SignOut();
}
