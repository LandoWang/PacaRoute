using System.CommandLine;
using System.Text.Json;
using CopilotBridge.Cli.Auth;
using CopilotBridge.Cli.Copilot;
using CopilotBridge.Cli.Hosting.ClientConfig;
using CopilotBridge.Cli.Hosting.Options;
using CopilotBridge.Cli.Models;
using CopilotBridge.Cli.Pipeline.Routing;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace CopilotBridge.Cli.Desktop;

internal static class DesktopCommand
{
    public static Command Build()
    {
        var action = new Argument<string>("action") { Description = "status, account, models, login, reauthorize, logout, configure" };
        var client = new Option<string?>("--client");
        var source = new Option<string>("--source") { DefaultValueFactory = _ => "copilot" };
        var account = new Option<string?>("--account");
        var subscription = new Option<string?>("--subscription");
        var resource = new Option<string?>("--resource");
        var model = new Option<string?>("--model");
        var port = new Option<int>("--port") { DefaultValueFactory = _ => 8765 };
        var apply = new Option<bool>("--apply");
        var fingerprint = new Option<string?>("--fingerprint");
        var command = new Command("desktop", "JSON-lines control interface for the native macOS app");
        command.Arguments.Add(action);
        command.Options.Add(client);
        command.Options.Add(source);
        command.Options.Add(account);
        command.Options.Add(subscription);
        command.Options.Add(resource);
        command.Options.Add(model);
        command.Options.Add(port);
        command.Options.Add(apply);
        command.Options.Add(fingerprint);
        command.SetAction(async (args, ct) =>
        {
            try
            {
                var reply = await RunAsync(args.GetValue(action)!, args.GetValue(client),
                    args.GetValue(model), args.GetValue(port), args.GetValue(apply),
                    args.GetValue(fingerprint), args.GetValue(source)!, args.GetValue(account), args.GetValue(subscription), args.GetValue(resource), ct);
                Write(reply);
                return reply.Ok ? 0 : 1;
            }
            catch (Exception ex)
            {
                Write(new DesktopReply { Ok = false, Error = DesktopTelemetry.Sanitize(ex.Message) });
                return 1;
            }
        });
        return command;
    }

    private static async Task<DesktopReply> RunAsync(string action, string? client, string? model,
        int port, bool apply, string? fingerprint, string sourceId, string? accountId, string? subscription, string? resource, CancellationToken ct)
    {
        if (port is < 1024 or > 65535) throw new ArgumentException("Port must be between 1024 and 65535.");
        var sources = Sources.SourceRegistry.Load();
        var pools = new Sources.SourcePoolRegistry(sources, Sources.SourcePoolRegistry.ConfigurationPath);
        var pool = sourceId.StartsWith("pool:", StringComparison.Ordinal)
            ? pools.Find(sourceId[5..]) ?? throw new ArgumentException("Unknown source pool.") : null;
        var source = sourceId == "copilot" || pool is not null ? null : sources.Find(sourceId)
            ?? throw new ArgumentException("Unknown source.");
        if (pool is not null && action == "models")
            return new DesktopReply { Models = [new DesktopModel(pool.Model, pool.Name, "Source pool", pool.Agent == "claude-code", pool.Agent == "codex")] };
        if (source?.Kind == "azure-openai" && action == "models")
            return new DesktopReply { Models = source.Deployments.Select(d => new DesktopModel(d, d, "Azure OpenAI", true, true)).ToList() };
        if (action == "configure")
        {
            if (pool is not null && (client != pool.Agent || model != pool.Model))
                throw new ArgumentException("Select this source pool's configured Agent and model.");
            if (source?.Kind == "azure-openai" && (string.IsNullOrEmpty(model) || !source.Deployments.Contains(model)))
                throw new ArgumentException("Select a deployment from the chosen source.");
            var plan = DesktopClientConfiguration.Plan(client ?? "", port, model, sourceId);
            var backup = apply ? DesktopClientConfiguration.Apply(plan,
                fingerprint ?? throw new ArgumentException("Preview the configuration before applying it.")) : null;
            return new DesktopReply
            {
                Path = plan.TargetPath, Fingerprint = DesktopClientConfiguration.Fingerprint(plan.OriginalContent),
                Changes = plan.Summary, BackupPath = backup,
            };
        }

        var accounts = AccountRegistry.Load();
        accountId ??= source?.AccountId ?? "copilot";
        if (accounts.Find(accountId).Kind == "azure")
            return await AzureAccountService.ExecuteAsync(accounts, accountId, action, subscription, resource, Write, ct);
        var proxies = Sources.ProxyRegistry.Load();
        var proxyId = source is not null || action == "models" ? proxies.ForSource(sourceId) : proxies.ForAccount;
        using var http = new HttpClient(proxies.CreateHandler(proxyId)) { Timeout = TimeSpan.FromSeconds(30) };
        http.DefaultRequestHeaders.UserAgent.ParseAdd("copilot-bridge/0.1");
        var factory = new SingleClientHttpClientFactory(http);
        using var auth = accounts.CreateGitHub(accountId, factory, challenge => Write(new DesktopReply
        {
            Event = "device_code", UserCode = challenge.UserCode,
            VerificationUri = challenge.VerificationUri, ExpiresIn = challenge.ExpiresIn.TotalSeconds,
        }));

        if (action is "login" or "reauthorize")
            return await LoginAsync(auth, reauthorize: action == "reauthorize", ct);
        if (action == "logout")
        {
            auth.SignOut();
            return new DesktopReply { LoggedIn = false };
        }
        if (action == "account-status") return new DesktopReply { LoggedIn = auth.GetCredentialStatus() is not null };
        if (action == "status")
        {
            var clients = new List<DesktopClient>();
            foreach (var id in new[] { "codex", "claude-code" })
            {
                try
                {
                    var state = DesktopClientConfiguration.Client(id).Read(new BridgeConnection(port), ConfigScope.Global);
                    var currentSource = "copilot";
                    if (state.CurrentBaseUrl is { } url && Uri.TryCreate(url, UriKind.Absolute, out var current))
                    {
                        var segments = current.AbsolutePath.Split('/');
                        if (segments.Length >= 4 && segments[1] == "sources") currentSource = segments[2];
                        if (segments.Length >= 4 && segments[1] == "pools") currentSource = "pool:" + segments[2];
                    }
                    if (currentSource != "copilot")
                        state = DesktopClientConfiguration.Client(id).Read(new BridgeConnection(port, currentSource), ConfigScope.Global);
                    clients.Add(new DesktopClient(id, state.TargetPath, state.ConfiguredForBridge,
                        state.Drifted || (currentSource.StartsWith("pool:", StringComparison.Ordinal)
                            ? pools.Find(currentSource[5..]) is null : currentSource != "copilot" && sources.Find(currentSource) is null),
                        DesktopClientConfiguration.ReadModel(id, state.TargetPath), null, currentSource));
                }
                catch (Exception ex) { clients.Add(new DesktopClient(id, "", false, false, null, DesktopTelemetry.Sanitize(ex.Message))); }
            }
            return new DesktopReply { LoggedIn = auth.GetCredentialStatus() is not null, Clients = clients };
        }
        if (!auth.IsAuthenticated) throw new InvalidOperationException("Sign in to GitHub first.");
        if (action == "account")
        {
            var user = await auth.GetGitHubUserAsync(ct);
            try { return new DesktopReply { LoggedIn = true, Login = user.Login, Quota = await auth.GetAccountUsageAsync(ct) }; }
            catch (Exception ex) { return new DesktopReply { LoggedIn = true, Login = user.Login, QuotaError = DesktopTelemetry.Sanitize(ex.Message) }; }
        }
        if (action == "models")
        {
            var copilot = new CopilotClient(factory, auth, new CopilotHeaderFactory(),
                Options.Create(new UpstreamRetryOptions()), Options.Create(new UpstreamTimeoutOptions()),
                NullLogger<CopilotClient>.Instance);
            var response = await copilot.GetModelsAsync(ct);
            var claude = new ModelProfileCatalog();
            var codex = new CodexModelProfileCatalog();
            return new DesktopReply
            {
                Models = response.Data.Where(m => m.Policy?.State != "disabled")
                    .Select(m => new DesktopModel(m.Id, m.Name ?? m.Id, m.Vendor,
                        (m.SupportedEndpoints?.Contains("/v1/messages") == true && claude.Get(m.Id) is not null)
                            || (m.SupportedEndpoints?.Contains("/responses") == true && codex.KnownIds.Contains(m.Id)),
                        m.SupportedEndpoints?.Contains("/responses") == true && codex.KnownIds.Contains(m.Id)))
                    .Where(m => m.Claude || m.Codex).OrderBy(m => m.Id).ToList(),
            };
        }
        throw new ArgumentException("Unknown desktop action.");
    }

    internal static async Task<DesktopReply> LoginAsync(AuthService auth, bool reauthorize, CancellationToken ct)
    {
        if (reauthorize) await auth.LoginAsync(ct);
        else await auth.EnsureGitHubTokenAsync(ct);
        return new DesktopReply { LoggedIn = true };
    }

    private static void Write(DesktopReply reply)
    {
        Console.WriteLine(JsonSerializer.Serialize(reply, JsonContext.Default.DesktopReply));
        Console.Out.Flush();
    }
}
