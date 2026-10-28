using System.Diagnostics;
using System.Net;
using System.Text.Json;
using CopilotBridge.Cli.Desktop;
using CopilotBridge.Cli.Models;

namespace CopilotBridge.Cli.Sources;

internal sealed record ProxyProfile(string Id, string Name, string Mode, string? Address);
internal sealed record ProxyConfiguration
{
    public ProxyProfile[] Profiles { get; init; } = [];
    public Dictionary<string, string> Bindings { get; init; } = new();
    public string AccountProxyId { get; init; } = "system";
}

/// <summary>Immutable transport choices, resolved before sending credentials.</summary>
internal sealed class ProxyRegistry
{
    private readonly ProxyConfiguration _config;
    private readonly bool _legacy;
    public bool IsLegacy => _legacy;
    public ProxyRegistry(ProxyConfiguration config, bool legacy = false)
    {
        _config = config; _legacy = legacy;
        var ids = new HashSet<string>(StringComparer.Ordinal) { "system", "direct" };
        foreach (var profile in config.Profiles)
        {
            if (!System.Text.RegularExpressions.Regex.IsMatch(profile.Id, @"\A[a-z][a-z0-9-]{0,62}\z")
                || !ids.Add(profile.Id) || string.IsNullOrWhiteSpace(profile.Name) || profile.Name.Length > 100)
                throw new ArgumentException("Invalid or duplicate proxy profile.");
            ValidateMode(profile);
        }
        foreach (var id in config.Bindings.Values.Append(config.AccountProxyId)) _ = Find(id);
    }
    public static ProxyRegistry Load()
    {
        var path = Environment.GetEnvironmentVariable("COPILOT_BRIDGE_PROXIES_FILE");
        return string.IsNullOrEmpty(path) ? new(new(), legacy: true) :
            new(JsonSerializer.Deserialize(File.ReadAllText(path), JsonContext.Default.ProxyConfiguration)
                ?? throw new ArgumentException("Invalid proxy configuration."));
    }
    public string ForSource(string? id) => _config.Bindings.GetValueOrDefault(id ?? "copilot", "system");
    public string ForAccount => _config.AccountProxyId;
    public ProxyProfile Find(string id) => id switch
    {
        "system" => new(id, "System", "system", null),
        "direct" => new(id, "Direct", "direct", null),
        _ => _config.Profiles.FirstOrDefault(p => p.Id == id) ?? throw new ArgumentException("Unknown proxy profile: " + id),
    };
    private static void ValidateMode(ProxyProfile p)
    {
        if (p.Mode == "system") return;
        _ = DesktopNetwork.CreateProxy(p.Mode, p.Address);
    }
    public HttpClientHandler CreateHandler(string id)
    {
        var profile = Find(id);
        var handler = new HttpClientHandler { AllowAutoRedirect = false };
        if (profile.Mode == "direct") handler.UseProxy = false;
        else if (profile.Mode == "custom") handler.Proxy = ExplicitProxy(profile.Address!);
        // DefaultProxy is read-only here: environment/system behavior remains intact.
        return handler;
    }
    private static IWebProxy ExplicitProxy(string address) => new WebProxy(new Uri(address)) { BypassProxyOnLocal = false };
    public void Apply(ProcessStartInfo start, string id)
    {
        if (_legacy) return;
        var p = Find(id);
        foreach (var key in new[] { "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy", "NO_PROXY", "no_proxy" })
        {
            start.Environment.Remove(key);
            if (Environment.GetEnvironmentVariable(key) is { } inherited) start.Environment[key] = inherited;
        }
        if (p.Mode == "system") return;
        foreach (var key in new[] { "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy" })
        {
            start.Environment.Remove(key);
            if (p.Mode == "custom") start.Environment[key] = p.Address;
        }
        start.Environment["NO_PROXY"] = start.Environment["no_proxy"] = p.Mode == "direct" ? "*" : "localhost,127.0.0.1,::1";
    }
}
