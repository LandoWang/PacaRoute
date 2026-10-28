using System.Diagnostics;
using System.Text.Json;
using System.Text.RegularExpressions;
using CopilotBridge.Cli.Desktop;

namespace CopilotBridge.Cli.Auth;

internal static partial class AzureAccountService
{
    internal static ProcessStartInfo StartInfo(AccountRegistry accounts, string accountId, IEnumerable<string> arguments)
    {
        var start = new ProcessStartInfo
        {
            FileName = OperatingSystem.IsMacOS() && File.Exists("/opt/homebrew/bin/az") ? "/opt/homebrew/bin/az" : "az",
            UseShellExecute = false, RedirectStandardOutput = true, RedirectStandardError = true,
            RedirectStandardInput = true, CreateNoWindow = true,
        };
#if DEBUG
        if (Environment.GetEnvironmentVariable("COPILOT_BRIDGE_TEST_AZURE_CLI") is { Length: > 0 } testCli)
        {
            if (!Path.IsPathFullyQualified(testCli)) throw new ArgumentException("Fixture CLI path must be absolute.");
            start.FileName = testCli;
        }
#endif
        var directory = accounts.AzureDirectory(accountId);
        if (directory is not null) start.Environment["AZURE_CONFIG_DIR"] = directory;
        start.Environment["AZURE_CORE_LOGIN_EXPERIENCE_V2"] = "off";
        foreach (var argument in arguments) start.ArgumentList.Add(argument);
        return start;
    }

    internal static async Task<string> RunAsync(AccountRegistry accounts, string accountId, string[] arguments,
        CancellationToken ct, Action<DesktopReply>? challenge = null)
    {
        using var timeout = CancellationTokenSource.CreateLinkedTokenSource(ct);
        timeout.CancelAfter(TimeSpan.FromSeconds(challenge is null ? 90 : 900));
        using var process = Process.Start(StartInfo(accounts, accountId, arguments))
            ?? throw new InvalidOperationException("Install Azure CLI to sign in to Azure accounts.");
        process.StandardInput.Close();
        var stdout = process.StandardOutput.ReadToEndAsync(timeout.Token);
        var stderr = DrainAsync();
        try
        {
            await process.WaitForExitAsync(timeout.Token);
            await stderr;
            if (process.ExitCode != 0)
                throw new InvalidOperationException("Azure command failed. Check this account's login, subscription and resource permissions.");
            return await stdout;
        }
        finally
        {
            if (!process.HasExited) process.Kill(entireProcessTree: true);
            try { await Task.WhenAll(stdout, stderr); } catch { }
        }
        async Task DrainAsync()
        {
            while (await process.StandardError.ReadLineAsync(timeout.Token) is { } line)
            {
                if (challenge is null) continue;
                var match = DeviceCode().Match(line);
                if (match.Success) challenge(new DesktopReply { Event = "device_code", UserCode = match.Groups[1].Value,
                    VerificationUri = "https://microsoft.com/devicelogin", ExpiresIn = 900 });
            }
        }
    }

    public static async Task<DesktopReply> ExecuteAsync(AccountRegistry accounts, string id, string action,
        string? subscription, string? resource, Action<DesktopReply> challenge, CancellationToken ct)
    {
        if (action is "login" or "logout")
        {
            if (id == "azure-local") throw new InvalidOperationException("Manage the shared Azure CLI identity in your terminal, or add an isolated Azure account.");
            await RunAsync(accounts, id, action == "login"
                ? ["login", "--use-device-code", "--allow-no-subscriptions", "-o", "none"] : ["logout"], ct,
                action == "login" ? challenge : null);
            return new DesktopReply { LoggedIn = action == "login" };
        }
        if (action is "account" or "account-status" or "azure-subscriptions")
        {
            using var doc = JsonDocument.Parse(await RunAsync(accounts, id, ["account", "list", "-o", "json"], ct));
            var list = doc.RootElement.EnumerateArray().Where(a => a.GetProperty("state").GetString() == "Enabled")
                .Select(a => new DesktopSubscription(a.GetProperty("id").GetString()!, a.GetProperty("name").GetString()!,
                    a.GetProperty("user").GetProperty("name").GetString()!)).ToList();
            return new DesktopReply { LoggedIn = list.Count > 0, Login = list.FirstOrDefault()?.Login, Subscriptions = list };
        }
        if (!Guid.TryParse(subscription, out _)) throw new ArgumentException("Select an Azure subscription.");
        if (action == "azure-resources")
        {
            using var doc = JsonDocument.Parse(await RunAsync(accounts, id,
                ["cognitiveservices", "account", "list", "--subscription", subscription!, "-o", "json"], ct));
            var resources = doc.RootElement.EnumerateArray()
                .Where(a => a.GetProperty("kind").GetString() is "OpenAI" or "AIServices")
                .Select(a => new DesktopResource(a.GetProperty("id").GetString()!, a.GetProperty("name").GetString()!,
                    a.GetProperty("properties").TryGetProperty("endpoint", out var endpoint) ? endpoint.GetString() ?? "" : ""))
                .OrderBy(a => a.Name).ToList();
            return new DesktopReply { Resources = resources };
        }
        if (action == "azure-deployments")
        {
            var parts = (resource ?? "").Split('/');
            if (parts.Length != 9 || !parts[1].Equals("subscriptions", StringComparison.OrdinalIgnoreCase)
                || !parts[2].Equals(subscription, StringComparison.OrdinalIgnoreCase)
                || !parts[3].Equals("resourceGroups", StringComparison.OrdinalIgnoreCase)
                || !parts[5].Equals("providers", StringComparison.OrdinalIgnoreCase)
                || !parts[6].Equals("Microsoft.CognitiveServices", StringComparison.OrdinalIgnoreCase)
                || !parts[7].Equals("accounts", StringComparison.OrdinalIgnoreCase))
                throw new ArgumentException("Select a resource from this subscription.");
            using var doc = JsonDocument.Parse(await RunAsync(accounts, id,
                ["cognitiveservices", "account", "deployment", "list", "--subscription", subscription!,
                    "--resource-group", parts[4], "--name", parts[8], "-o", "json"], ct));
            return new DesktopReply { Models = doc.RootElement.EnumerateArray()
                .Where(a => a.GetProperty("properties").GetProperty("model").GetProperty("format").GetString() == "OpenAI")
                .Select(a => a.GetProperty("name").GetString()!)
                .Select(name => new DesktopModel(name, name, "Azure OpenAI", true, true)).ToList() };
        }
        throw new ArgumentException("Unsupported Azure account action.");
    }

    [GeneratedRegex(@"enter the code\s+([A-Z0-9]{6,20})", RegexOptions.IgnoreCase)]
    private static partial Regex DeviceCode();
}
