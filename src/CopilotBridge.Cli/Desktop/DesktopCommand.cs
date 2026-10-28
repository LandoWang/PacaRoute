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
        var action = new Argument<string>("action") { Description = "status, account, models, login, logout, configure" };
        var client = new Option<string?>("--client");
        var model = new Option<string?>("--model");
        var port = new Option<int>("--port") { DefaultValueFactory = _ => 8765 };
        var apply = new Option<bool>("--apply");
        var fingerprint = new Option<string?>("--fingerprint");
        var command = new Command("desktop", "JSON-lines control interface for the native macOS app");
        command.Arguments.Add(action);
        command.Options.Add(client);
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
                    args.GetValue(fingerprint), ct);
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
        int port, bool apply, string? fingerprint, CancellationToken ct)
    {
        if (port is < 1024 or > 65535) throw new ArgumentException("Port must be between 1024 and 65535.");
        if (action == "configure")
        {
            var plan = DesktopClientConfiguration.Plan(client ?? "", port, model);
            var backup = apply ? DesktopClientConfiguration.Apply(plan,
                fingerprint ?? throw new ArgumentException("Preview the configuration before applying it.")) : null;
            return new DesktopReply
            {
                Path = plan.TargetPath, Fingerprint = DesktopClientConfiguration.Fingerprint(plan.OriginalContent),
                Changes = plan.Summary, BackupPath = backup,
            };
        }

        using var http = new HttpClient { Timeout = TimeSpan.FromSeconds(30) };
        http.DefaultRequestHeaders.UserAgent.ParseAdd("copilot-bridge/0.1");
        var factory = new SingleClientHttpClientFactory(http);
        using var auth = new AuthService(factory, challenge => Write(new DesktopReply
        {
            Event = "device_code", UserCode = challenge.UserCode,
            VerificationUri = challenge.VerificationUri, ExpiresIn = challenge.ExpiresIn.TotalSeconds,
        }), AuthCommand.LoadLoginProvider());

        if (action == "login")
        {
            await auth.LoginAsync(ct);
            return new DesktopReply { LoggedIn = true };
        }
        if (action == "logout")
        {
            auth.SignOut();
            return new DesktopReply { LoggedIn = false };
        }
        if (action == "status")
        {
            var clients = new List<DesktopClient>();
            foreach (var id in new[] { "codex", "claude-code" })
            {
                try
                {
                    var state = DesktopClientConfiguration.Client(id).Read(new BridgeConnection(port), ConfigScope.Global);
                    clients.Add(new DesktopClient(id, state.TargetPath, state.ConfiguredForBridge, state.Drifted,
                        DesktopClientConfiguration.ReadModel(id, state.TargetPath), null));
                }
                catch (Exception ex) { clients.Add(new DesktopClient(id, "", false, false, null, DesktopTelemetry.Sanitize(ex.Message))); }
            }
            return new DesktopReply { LoggedIn = auth.GetCredentialStatus() is not null, Clients = clients };
        }
        if (!auth.IsAuthenticated) throw new InvalidOperationException("Sign in to GitHub first.");
        if (action == "account")
        {
            var user = await auth.GetGitHubUserAsync(ct);
            try { return new DesktopReply { Login = user.Login, Quota = await auth.GetAccountUsageAsync(ct) }; }
            catch (Exception ex) { return new DesktopReply { Login = user.Login, QuotaError = DesktopTelemetry.Sanitize(ex.Message) }; }
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
                        m.SupportedEndpoints?.Contains("/v1/messages") == true && claude.Get(m.Id) is not null,
                        m.SupportedEndpoints?.Contains("/responses") == true && codex.KnownIds.Contains(m.Id)))
                    .Where(m => m.Claude || m.Codex).OrderBy(m => m.Id).ToList(),
            };
        }
        throw new ArgumentException("Unknown desktop action.");
    }

    private static void Write(DesktopReply reply)
    {
        Console.WriteLine(JsonSerializer.Serialize(reply, JsonContext.Default.DesktopReply));
        Console.Out.Flush();
    }
}
