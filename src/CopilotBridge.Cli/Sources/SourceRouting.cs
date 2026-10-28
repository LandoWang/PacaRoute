using System.Diagnostics;
using System.Text;
using System.Text.Json;
using CopilotBridge.Cli.Auth;
using CopilotBridge.Cli.Endpoints;
using CopilotBridge.Cli.Desktop;
using CopilotBridge.Cli.Endpoints.ClaudeCode;
using CopilotBridge.Cli.Models;
using CopilotBridge.Cli.Models.Anthropic.Errors;
using CopilotBridge.Cli.Models.Anthropic.Request;
using CopilotBridge.Cli.Pipeline;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Http;
using Microsoft.Extensions.DependencyInjection;

namespace CopilotBridge.Cli.Sources;

internal static class SourceRouting
{
    public static void Use(WebApplication app)
    {
        app.Use(async (http, next) =>
        {
            var path = http.Request.Path.Value ?? "";
            if (!path.StartsWith("/sources/", StringComparison.Ordinal))
            {
                if (Environment.GetEnvironmentVariable("COPILOT_BRIDGE_DESKTOP") == "1"
                    && (path.StartsWith("/cc/") || path.StartsWith("/codex/"))
                    && !http.RequestServices.GetRequiredService<IAuthService>().IsAuthenticated)
                { await Error(http, 401, "Sign in to GitHub for the Copilot source."); return; }
                await next(http); return;
            }
            var parts = path.Split('/', 4);
            var source = parts.Length == 4
                ? http.RequestServices.GetRequiredService<SourceRegistry>().Find(parts[2]) : null;
            if (source is null) { await Error(http, 404, "Unknown model source."); return; }
            var route = "/" + parts[3];
            if (source.Kind == "copilot")
            {
                http.Items[typeof(ModelSource)] = source;
                if (!http.RequestServices.GetRequiredService<IAuthService>().IsAuthenticated)
                { await Error(http, 401, "Sign in to this GitHub account."); return; }
                if (!(route.StartsWith("/codex/", StringComparison.Ordinal) || route.StartsWith("/cc/", StringComparison.Ordinal)))
                { await Error(http, 404, "Unsupported source route."); return; }
                using var accountScope = Serilog.Context.LogContext.PushProperty("ModelSource", source.Id);
                http.Request.Path = route;
                try { await next(http); }
                finally { http.Request.Path = path; }
                return;
            }
            if (http.Request.Method == "GET" && route is "/codex/models" or "/cc/v1/models")
            {
                using var buffer = new MemoryStream();
                using (var w = new Utf8JsonWriter(buffer))
                {
                    w.WriteStartObject();
                    // Codex falls back to its bundled metadata for explicit deployment IDs.
                    w.WriteStartArray(route.StartsWith("/codex/") ? "models" : "data");
                    if (route.StartsWith("/cc/")) foreach (var deployment in source.Deployments)
                    {
                        w.WriteStartObject(); w.WriteString("id", deployment); w.WriteString("type", "model");
                        w.WriteString("display_name", deployment); w.WriteString("created_at", "2026-01-01T00:00:00Z"); w.WriteEndObject();
                    }
                    w.WriteEndArray(); w.WriteEndObject();
                }
                http.Response.ContentType = "application/json";
                await http.Response.Body.WriteAsync(buffer.ToArray(), http.RequestAborted); return;
            }
            if (http.Request.Method != "POST") { await Error(http, 404, "Unsupported source route."); return; }
            if (route == "/codex/responses") { await ForwardCodex(http, source); return; }
            if (route == "/cc/v1/messages/count_tokens") { await Estimate(http, source); return; }
            if (route != "/cc/v1/messages") { await Error(http, 404, "Unsupported source route."); return; }
            http.RequestServices.GetRequiredService<BridgeContext<MessagesRequest>>().Source = source;
            using var scope = Serilog.Context.LogContext.PushProperty("ModelSource", source.Id);
            http.Request.Path = route;
            try { await next(http); }
            finally { http.Request.Path = path; }
        });
    }

    private static async Task ForwardCodex(HttpContext http, ModelSource source)
    {
        var clock = Stopwatch.StartNew();
        var summary = new RequestSummary { Kind = "responses", Source = source.Id };
        try
        {
            using var input = await InboundBody.ReadPooledAsync(http.Request.Body, http.RequestAborted);
            var body = input.Memory.ToArray();
            using var doc = JsonDocument.Parse(body);
            var model = ReadModel(doc.RootElement, source);
            summary.RequestedModel = summary.ResolvedModel = model;
            using var response = await http.RequestServices.GetRequiredService<AzureResponsesClient>()
                .PostAsync(source, body, http.RequestAborted);
            http.Response.StatusCode = summary.StatusCode = (int)response.StatusCode;
            http.Response.ContentType = response.Content.Headers.ContentType?.ToString() ?? "application/json";
            foreach (var name in new[] { "retry-after", "x-request-id", "apim-request-id" })
                if (response.Headers.TryGetValues(name, out var values)) http.Response.Headers[name] = string.Join(',', values);
            using var stream = await response.Content.ReadAsStreamAsync(http.RequestAborted);
            var observer = new ResponsesUsageObserver(summary, http.Response.ContentType.StartsWith("text/event-stream"));
            var bytes = new byte[16384];
            while (true)
            {
                using var idle = CancellationTokenSource.CreateLinkedTokenSource(http.RequestAborted);
                idle.CancelAfter(TimeSpan.FromSeconds(120));
                var count = await stream.ReadAsync(bytes, idle.Token);
                if (count == 0) break;
                observer.Append(bytes.AsSpan(0, count));
                await http.Response.Body.WriteAsync(bytes.AsMemory(0, count), http.RequestAborted);
                await http.Response.Body.FlushAsync(http.RequestAborted);
                if (observer.Terminal) break;
            }
            observer.Complete();
        }
        catch (Exception ex) when (ex is JsonException or ArgumentException)
        {
            summary.StatusCode = 400;
            if (!http.Response.HasStarted) await Error(http, 400, "Invalid request or deployment not configured for this source.");
            else http.Abort();
        }
        catch (Exception)
        {
            summary.StatusCode = 502;
            if (!http.Response.HasStarted) await Error(http, 502, "Azure request failed. Check source authentication, network and deployment.");
            else http.Abort();
        }
        finally
        {
            summary.DurationMs = clock.ElapsedMilliseconds;
            DesktopTelemetry.Record(summary);
        }
    }

    internal static string ReadModel(JsonElement root, ModelSource source)
    {
        if (root.ValueKind != JsonValueKind.Object || !root.TryGetProperty("model", out var model)
            || model.ValueKind != JsonValueKind.String || !source.Deployments.Contains(model.GetString(), StringComparer.Ordinal))
            throw new ArgumentException("Deployment is not configured for this source.");
        return model.GetString()!;
    }

    private static async Task Estimate(HttpContext http, ModelSource source)
    {
        try
        {
            using var input = await InboundBody.ReadPooledAsync(http.Request.Body, http.RequestAborted);
            using var doc = JsonDocument.Parse(input.Memory);
            var model = ReadModel(doc.RootElement, source);
            // Deliberately conservative, locally computed admission estimate, never billing.
            var tokens = checked(input.Memory.Length / 3 + 1024);
            http.Response.Headers["X-Bridge-Token-Count"] = "local-estimate";
            http.Response.ContentType = "application/json";
            await http.Response.WriteAsync(JsonSerializer.Serialize(new CountTokensResponse { InputTokens = tokens },
                JsonContext.Default.CountTokensResponse), http.RequestAborted);
            DesktopTelemetry.Record(new RequestSummary { Kind = "count_tokens", Source = source.Id,
                RequestedModel = model, StatusCode = 200, Usage = new UsageSnapshot { InputTokens = tokens } });
        }
        catch (Exception ex) when (ex is JsonException or ArgumentException)
        { await Error(http, 400, "Invalid request or deployment not configured for this source."); }
    }

    private static Task Error(HttpContext http, int status, string message)
    {
        http.Response.StatusCode = status;
        http.Response.ContentType = "application/json";
        return http.Response.WriteAsync(JsonSerializer.Serialize(new ErrorResponse { Error = new ErrorBody {
                Type = status == 401 ? "authentication_error" : status >= 500 ? "api_error" : "invalid_request_error",
                Message = message } }, JsonContext.Default.ErrorResponse), http.RequestAborted);
    }
}

/// <summary>Observes bounded JSON lines without changing a single forwarded byte.</summary>
internal sealed class ResponsesUsageObserver(RequestSummary summary, bool streaming)
{
    private readonly MemoryStream _line = new();
    private bool _overflow;
    public bool Terminal { get; private set; }
    public void Append(ReadOnlySpan<byte> bytes)
    {
        foreach (var value in bytes)
        {
            if (streaming && value == 10) { Parse(); _line.SetLength(0); _overflow = false; }
            else if (_line.Length < 4 * 1024 * 1024) _line.WriteByte(value);
            else _overflow = true;
        }
    }
    public void Complete()
    {
        Parse();
        if (streaming && !Terminal && summary.StatusCode is >= 200 and < 300) summary.StatusCode = 502;
        _line.Dispose();
    }
    private void Parse()
    {
        if (_overflow || _line.Length == 0) return;
        var bytes = _line.GetBuffer().AsSpan(0, (int)_line.Length);
        if (streaming)
        {
            if (!bytes.StartsWith("data:"u8)) return;
            bytes = bytes[5..];
        }
        try
        {
            using var doc = JsonDocument.Parse(bytes.ToArray());
            var root = doc.RootElement;
            if (root.TryGetProperty("type", out var type))
            {
                Terminal |= type.GetString() is "response.completed" or "response.incomplete" or "response.failed";
                if (type.GetString() == "response.failed") summary.StatusCode = 502;
            }
            var response = root.TryGetProperty("response", out var envelope) ? envelope : root;
            if (response.TryGetProperty("status", out var status) && status.GetString() == "failed") summary.StatusCode = 502;
            if (!response.TryGetProperty("usage", out var usage) || usage.ValueKind != JsonValueKind.Object) return;
            if (usage.TryGetProperty("input_tokens", out var input) && input.TryGetInt32(out var i)) summary.Usage.InputTokens = i;
            if (usage.TryGetProperty("output_tokens", out var output) && output.TryGetInt32(out var o)) summary.Usage.OutputTokens = o;
            if (usage.TryGetProperty("input_tokens_details", out var details) && details.ValueKind == JsonValueKind.Object
                && details.TryGetProperty("cached_tokens", out var cache) && cache.TryGetInt32(out var c)) summary.Usage.CacheReadInputTokens = c;
        }
        catch (JsonException) { }
    }
}
