using System.Collections.Concurrent;
using CopilotBridge.Cli.Copilot;

namespace CopilotBridge.Cli.Sources;

/// <summary>Separate immutable pools for each source and upstream surface.</summary>
internal sealed class SourceHttpClientFactory(ProxyRegistry proxies, string proxyId) : IHttpClientFactory, IDisposable
{
    private readonly ConcurrentDictionary<string, Lazy<HttpClientHandler>> _handlers = new();
    public HttpClient CreateClient(string name)
    {
        var handler = _handlers.GetOrAdd(name, _ => new Lazy<HttpClientHandler>(() => proxies.CreateHandler(proxyId))).Value;
        var client = new HttpClient(handler, disposeHandler: false);
        client.Timeout = name == UpstreamHttpClientNames.GitHubAuth ? TimeSpan.FromSeconds(30)
            : name == UpstreamHttpClientNames.Metadata ? TimeSpan.FromMinutes(2) : Timeout.InfiniteTimeSpan;
        client.DefaultRequestHeaders.UserAgent.ParseAdd("copilot-bridge/0.1");
        return client;
    }
    public void Dispose() { foreach (var h in _handlers.Values) if (h.IsValueCreated) h.Value.Dispose(); }
}
