using System.Net;
using System.Text.Json;
using CopilotBridge.Cli.Desktop;
using CopilotBridge.Cli.Endpoints;
using CopilotBridge.Cli.Models;
using CopilotBridge.Cli.Models.Anthropic.Errors;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.DependencyInjection;

namespace CopilotBridge.Cli.Sources;

/// <summary>Reuses real source endpoints in the same listener, with proxies and redirects
/// disabled on this local hop. No new process, credential reader or protocol adapter.</summary>
internal sealed class PoolTransport : IDisposable
{
    private readonly HttpClient _client;
    internal static readonly HashSet<string> HopHeaders = new(StringComparer.OrdinalIgnoreCase)
        { "Host", "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade" };
    public PoolTransport() : this(new SocketsHttpHandler { UseProxy = false, AllowAutoRedirect = false,
        ConnectTimeout = TimeSpan.FromSeconds(5), AutomaticDecompression = DecompressionMethods.None }) { }
    internal PoolTransport(HttpMessageHandler handler) => _client = new HttpClient(handler) { Timeout = Timeout.InfiniteTimeSpan };

    public async Task<HttpResponseMessage> Send(HttpContext http, PoolMember member, string route, byte[] body)
    {
        var port = http.Connection.LocalPort;
        if (port is < 1 or > 65535) throw new InvalidOperationException("Pool routing requires a local Bridge listener.");
        var prefix = member.SourceId == "copilot" ? "" : "/sources/" + Uri.EscapeDataString(member.SourceId);
        // Never construct a destination from the client-supplied Host header.
        var target = new UriBuilder("http", "127.0.0.1", port, prefix + route) { Query = http.Request.QueryString.Value ?? "" }.Uri;
        using var request = new HttpRequestMessage(HttpMethod.Post, target) { Content = new ByteArrayContent(body) };
        request.Content.Headers.ContentType = new("application/json");
        foreach (var header in http.Request.Headers)
            if (!HopHeaders.Contains(header.Key) && header.Key is not ("Content-Length" or "Content-Type"))
                request.Headers.TryAddWithoutValidation(header.Key, header.Value.ToArray());
        return await _client.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, http.RequestAborted);
    }
    public void Dispose() => _client.Dispose();
}

internal static class PoolRouting
{
    public static void Use(WebApplication app) => app.Use(async (http, next) =>
    {
        if (!(http.Request.Path.Value ?? "").StartsWith("/pools/", StringComparison.Ordinal)) { await next(http); return; }
        try
        {
            var parts = http.Request.Path.Value!.Split('/', 4);
            var pool = parts.Length == 4 ? http.RequestServices.GetRequiredService<SourcePoolRegistry>().Find(parts[2]) : null;
            if (pool is null) { await Error(http, 404, "Unknown source pool."); return; }
            var route = "/" + parts[3];
            if (http.Request.Method == "GET" && route == (pool.Agent == "codex" ? "/codex/models" : "/cc/v1/models"))
            { await Models(http, pool); return; }
            var estimate = pool.Agent == "claude-code" && route == "/cc/v1/messages/count_tokens";
            if (http.Request.Method != "POST" || (!estimate && route != (pool.Agent == "codex" ? "/codex/responses" : "/cc/v1/messages")))
            { await Error(http, 404, "This source pool is configured for a different Agent route."); return; }
            using var input = await InboundBody.ReadPooledAsync(http.Request.Body, http.RequestAborted);
            using var document = JsonDocument.Parse(input.Memory);
            var body = document.RootElement;
            if (body.ValueKind != JsonValueKind.Object || !body.TryGetProperty("model", out var model)
                || model.ValueKind != JsonValueKind.String || model.GetString() != pool.Model)
            { await Error(http, 400, "Select the model configured for this source pool."); return; }
            var session = PoolSessionIdentity.Read(pool.Agent, http.Request.Headers, body);
            if (session is null && !estimate)
            { await Error(http, 400, "A stable Agent session identifier is required for source-pool routing."); return; }
            var transport = http.RequestServices.GetRequiredService<PoolTransport>();
            if (estimate)
            {
                // Admission estimates must not allocate a session or consume its weight.
                var member = pool.Members.First(member => member.Weight > 0);
                using var response = await transport.Send(http, member, route, RewriteModel(body, member.Model));
                await Forward(http, response); return;
            }
            await Route(http, pool, session!, route, body, transport,
                http.RequestServices.GetRequiredService<PoolAffinityStore>());
        }
        catch (PoolHistoryRequiredException)
        { await Error(http, 409, "Pool reassignment requires full conversation history; server response references cannot move between sources."); }
        catch (OperationCanceledException) when (http.RequestAborted.IsCancellationRequested) { }
        catch (Exception exception) when (exception is JsonException or ArgumentException)
        { if (!http.Response.HasStarted) await Error(http, 400, "Invalid source pool configuration or request."); else http.Abort(); }
        catch (Exception exception) when (exception is InvalidOperationException or IOException or UnauthorizedAccessException)
        { if (!http.Response.HasStarted) await Error(http, 503, "Source pool state is unavailable. Check pool members, cooldowns and local storage."); else http.Abort(); }
    });

    private static async Task Route(HttpContext http, SourcePool pool, string session, string route, JsonElement body,
        PoolTransport transport, PoolAffinityStore affinity)
    {
        var attempted = new HashSet<string>(StringComparer.Ordinal);
        var member = affinity.Select(pool, session, allowReassign: !HasRemoteReference(body));
        for (var attempt = 0; attempt < pool.Members.Length; attempt++)
        {
            attempted.Add(member.Id);
            http.Response.Clear();
            try
            {
                using var response = await transport.Send(http, member, route, RewriteModel(body, member.Model));
                if (Retryable((int)response.StatusCode))
                {
                    var delay = response.Headers.RetryAfter?.Delta
                        ?? (response.Headers.RetryAfter?.Date is { } date ? date - DateTimeOffset.UtcNow : (TimeSpan?)null);
                    affinity.Failed(pool, member, delay);
                    if (pool.Failover && attempt + 1 < pool.Members.Length)
                    {
                        if (HasRemoteReference(body)) { await Error(http, 409, "Pool failover requires full conversation history; server response references cannot move between sources."); return; }
                        if (Alternate(affinity, pool, session, attempted) is { } replacement)
                        { Switched(pool, member, replacement, (int)response.StatusCode); member = replacement; continue; }
                    }
                }
                await Forward(http, response);
                return;
            }
            catch (OperationCanceledException) when (http.RequestAborted.IsCancellationRequested) { return; }
            catch (Exception exception) when (exception is HttpRequestException or IOException or OperationCanceledException)
            {
                affinity.Failed(pool, member);
                if (http.Response.HasStarted) { http.Abort(); return; }
                if (pool.Failover && attempt + 1 < pool.Members.Length)
                {
                    if (HasRemoteReference(body)) { await Error(http, 409, "Pool failover requires full conversation history; server response references cannot move between sources."); return; }
                    if (Alternate(affinity, pool, session, attempted) is { } replacement)
                    { Switched(pool, member, replacement, 502); member = replacement; continue; }
                }
                await Error(http, 502, "Source pool transport failed; no alternate member is available."); return;
            }
        }
    }

    private static PoolMember? Alternate(PoolAffinityStore affinity, SourcePool pool, string session, HashSet<string> attempted)
    {
        try { return affinity.Select(pool, session, attempted); }
        catch (InvalidOperationException) { return null; }
    }
    private static bool Retryable(int status) => status is 401 or 403 or 408 or 429 || status >= 500;
    private static bool HasRemoteReference(JsonElement body) =>
        (body.TryGetProperty("previous_response_id", out var previous) && previous.ValueKind == JsonValueKind.String && !string.IsNullOrEmpty(previous.GetString()))
        || (body.TryGetProperty("input", out var input) && input.ValueKind == JsonValueKind.Array
            && input.EnumerateArray().Any(item => item.ValueKind == JsonValueKind.Object && item.TryGetProperty("type", out var type)
                && type.ValueKind == JsonValueKind.String && type.GetString() == "item_reference"));

    internal static byte[] RewriteModel(JsonElement body, string model)
    {
        using var buffer = new MemoryStream();
        using (var writer = new Utf8JsonWriter(buffer))
        {
            writer.WriteStartObject();
            foreach (var property in body.EnumerateObject())
                if (property.NameEquals("model")) writer.WriteString("model", model); else property.WriteTo(writer);
            writer.WriteEndObject();
        }
        return buffer.ToArray();
    }

    private static async Task Forward(HttpContext http, HttpResponseMessage response)
    {
        http.Response.StatusCode = (int)response.StatusCode;
        foreach (var header in response.Headers.Concat(response.Content.Headers))
            if (!PoolTransport.HopHeaders.Contains(header.Key)) http.Response.Headers[header.Key] = header.Value.ToArray();
        using var stream = await response.Content.ReadAsStreamAsync(http.RequestAborted);
        var buffer = new byte[16_384];
        while (true)
        {
            var count = await stream.ReadAsync(buffer, http.RequestAborted);
            if (count == 0) break;
            await http.Response.Body.WriteAsync(buffer.AsMemory(0, count), http.RequestAborted);
            await http.Response.Body.FlushAsync(http.RequestAborted);
        }
    }
    private static void Switched(SourcePool pool, PoolMember from, PoolMember to, int status)
    {
        if (Environment.GetEnvironmentVariable("COPILOT_BRIDGE_DESKTOP") != "1") return;
        DesktopTelemetry.Write(new DesktopActivity { Kind = "error", Source = to.SourceId, Model = to.Model,
            Client = pool.Agent == "codex" ? "Codex" : "Claude Code",
            Error = $"Source pool {pool.Id}: {from.SourceId}/{from.Model} -> {to.SourceId}/{to.Model} after HTTP {status}." });
    }

    private static async Task Models(HttpContext http, SourcePool pool)
    {
        http.Response.ContentType = "application/json";
        using var buffer = new MemoryStream();
        using (var writer = new Utf8JsonWriter(buffer))
        {
            writer.WriteStartObject(); writer.WriteStartArray(pool.Agent == "codex" ? "models" : "data");
            if (pool.Agent == "claude-code")
            {
                writer.WriteStartObject(); writer.WriteString("id", pool.Model); writer.WriteString("type", "model");
                writer.WriteString("display_name", pool.Name); writer.WriteString("created_at", "2026-01-01T00:00:00Z"); writer.WriteEndObject();
            }
            writer.WriteEndArray(); writer.WriteEndObject();
        }
        await http.Response.Body.WriteAsync(buffer.ToArray(), http.RequestAborted);
    }
    private static Task Error(HttpContext http, int status, string message)
    {
        http.Response.Clear(); http.Response.StatusCode = status; http.Response.ContentType = "application/json";
        return http.Response.WriteAsync(JsonSerializer.Serialize(new ErrorResponse { Error = new ErrorBody {
            Type = status >= 500 ? "api_error" : "invalid_request_error", Message = message } }, JsonContext.Default.ErrorResponse), http.RequestAborted);
    }
}
