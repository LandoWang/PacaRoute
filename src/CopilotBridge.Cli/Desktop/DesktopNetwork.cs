using System.Net;

namespace CopilotBridge.Cli.Desktop;

internal static class DesktopNetwork
{
    public static void ApplyEnvironment()
    {
        if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("COPILOT_BRIDGE_PROXIES_FILE"))) return;
        var mode = Environment.GetEnvironmentVariable("COPILOT_BRIDGE_PROXY_MODE");
        if (string.IsNullOrEmpty(mode) || mode == "system") return;
        HttpClient.DefaultProxy = CreateProxy(mode,
            Environment.GetEnvironmentVariable("COPILOT_BRIDGE_PROXY_URL"));
    }

    internal static IWebProxy CreateProxy(string mode, string? address)
    {
        if (mode == "direct") return new DirectProxy();
        if (mode != "custom" || !Uri.TryCreate(address, UriKind.Absolute, out var uri)
            || uri.Scheme is not ("http" or "https" or "socks5")
            || string.IsNullOrEmpty(uri.Host) || !string.IsNullOrEmpty(uri.UserInfo)
            || uri.AbsolutePath != "/" || !string.IsNullOrEmpty(uri.Query)
            || !string.IsNullOrEmpty(uri.Fragment))
            throw new ArgumentException("Use an HTTP, HTTPS, or SOCKS5 proxy URL without credentials, path, or query.");
        return new WebProxy(uri) { BypassProxyOnLocal = true };
    }

    private sealed class DirectProxy : IWebProxy
    {
        public ICredentials? Credentials { get; set; }
        public Uri GetProxy(Uri destination) => destination;
        public bool IsBypassed(Uri host) => true;
    }
}
