using System.Diagnostics;
using System.Text.Json;
using CopilotBridge.Cli.Models;
using CopilotBridge.Cli.Sources;

namespace CopilotBridge.Cli.Auth;

internal sealed record AzureAuthLease(string Header, string Value);

/// <summary>Azure credentials are owned here; callers receive only an immutable lease.</summary>
internal sealed class AzureAuthService : IDisposable
{
    private readonly Dictionary<string, string> _keys;
    private readonly Dictionary<string, (AzureAuthLease Lease, DateTimeOffset Expires)> _cache = new();
    private readonly SemaphoreSlim _gate = new(1, 1);

    public AzureAuthService(Dictionary<string, string> keys) => _keys = keys;

    public static async Task<AzureAuthService> FromDesktopAsync(CancellationToken ct)
    {
        if (Environment.GetEnvironmentVariable("COPILOT_BRIDGE_SECRETS_STDIN") != "1")
            return new AzureAuthService([]);
        using var timeout = CancellationTokenSource.CreateLinkedTokenSource(ct);
        timeout.CancelAfter(TimeSpan.FromSeconds(10));
        var line = await Console.In.ReadLineAsync(timeout.Token);
        if (line is null || line.Length > 65536) throw new InvalidOperationException("Missing desktop credential envelope.");
        return new AzureAuthService(JsonSerializer.Deserialize(line, JsonContext.Default.DictionaryStringString) ?? []);
    }

    public async Task<AzureAuthLease> GetLeaseAsync(ModelSource source, CancellationToken ct)
    {
        if (source.AuthMode == "api-key")
            return _keys.TryGetValue(source.Id, out var key) && !string.IsNullOrWhiteSpace(key)
                && !key.Any(char.IsControl)
                ? new AzureAuthLease("api-key", key)
                : throw new InvalidOperationException("Save this source's API key in the app, then restart Bridge.");
        await _gate.WaitAsync(ct);
        try
        {
            var subscription = source.SubscriptionId ?? "";
            if (_cache.TryGetValue(subscription, out var cached) && cached.Expires > DateTimeOffset.UtcNow.AddMinutes(5))
                return cached.Lease;
            var start = new ProcessStartInfo
            {
                FileName = OperatingSystem.IsMacOS() && File.Exists("/opt/homebrew/bin/az") ? "/opt/homebrew/bin/az" : "az",
                UseShellExecute = false, RedirectStandardOutput = true, RedirectStandardError = true,
                RedirectStandardInput = true, CreateNoWindow = true,
            };
            foreach (var arg in new[] { "account", "get-access-token", "--resource", "https://cognitiveservices.azure.com", "-o", "json" })
                start.ArgumentList.Add(arg);
            if (subscription.Length > 0) { start.ArgumentList.Add("--subscription"); start.ArgumentList.Add(subscription); }
            using var process = Process.Start(start) ?? throw new InvalidOperationException("Azure CLI could not start.");
            process.StandardInput.Close();
            using var timeout = CancellationTokenSource.CreateLinkedTokenSource(ct);
            timeout.CancelAfter(TimeSpan.FromSeconds(30));
            var stdout = process.StandardOutput.ReadToEndAsync(timeout.Token);
            var stderr = process.StandardError.ReadToEndAsync(timeout.Token);
            try { await process.WaitForExitAsync(timeout.Token); }
            catch { if (!process.HasExited) process.Kill(entireProcessTree: true); throw; }
            _ = await stderr;
            if (process.ExitCode != 0) throw new InvalidOperationException("Azure CLI authentication failed. Run az login for this subscription, then retry.");
            using var doc = JsonDocument.Parse(await stdout);
            var token = doc.RootElement.GetProperty("accessToken").GetString()
                ?? throw new InvalidOperationException("Azure CLI returned no token.");
            var expires = DateTimeOffset.UtcNow.AddMinutes(10);
            if (doc.RootElement.TryGetProperty("expires_on", out var expiry)
                && long.TryParse(expiry.ToString(), out var seconds)) expires = DateTimeOffset.FromUnixTimeSeconds(seconds);
            var lease = new AzureAuthLease("Authorization", "Bearer " + token);
            _cache[subscription] = (lease, expires);
            return lease;
        }
        finally { _gate.Release(); }
    }

    public void Dispose() { _keys.Clear(); _cache.Clear(); _gate.Dispose(); }
}
