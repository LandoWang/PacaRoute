using System.Net.Http.Headers;
using CopilotBridge.Cli.Auth;

namespace CopilotBridge.Cli.Sources;

internal sealed class AzureResponsesClient(AzureAuthService auth, ProxyRegistry proxies) : IDisposable
{
    private readonly System.Collections.Concurrent.ConcurrentDictionary<string, Lazy<HttpClient>> _clients = new();

    public async Task<HttpResponseMessage> PostAsync(ModelSource source, byte[] body, CancellationToken ct)
    {
        var lease = await auth.GetLeaseAsync(source, ct);
        var destination = source.ResponsesUri;
#if DEBUG
        // Local fixture seam is absent from the shipped AOT binary.
        var fixture = Environment.GetEnvironmentVariable("COPILOT_BRIDGE_TEST_AZURE_UPSTREAM");
        if (Uri.TryCreate(fixture, UriKind.Absolute, out var fixtureUri) && fixtureUri.IsLoopback)
            destination = new Uri(fixtureUri, source.Id + "/responses");
#endif
        using var request = new HttpRequestMessage(HttpMethod.Post, destination);
        request.Headers.Add(lease.Header, lease.Value);
        request.Content = new ByteArrayContent(body);
        request.Content.Headers.ContentType = new MediaTypeHeaderValue("application/json");
        using var timeout = CancellationTokenSource.CreateLinkedTokenSource(ct);
        timeout.CancelAfter(TimeSpan.FromSeconds(120));
        var http = _clients.GetOrAdd(source.Id, _ => new Lazy<HttpClient>(() =>
            new HttpClient(proxies.CreateHandler(proxies.ForSource(source.Id))) { Timeout = Timeout.InfiniteTimeSpan })).Value;
        return await http.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, timeout.Token);
    }

    public void Dispose() { foreach (var c in _clients.Values) if (c.IsValueCreated) c.Value.Dispose(); }
}
