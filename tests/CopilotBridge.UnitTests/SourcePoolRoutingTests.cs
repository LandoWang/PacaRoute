using System.Net;
using System.Text;
using System.Text.Json;
using CopilotBridge.Cli.Models;
using CopilotBridge.Cli.Sources;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.TestHost;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;
using Xunit;

namespace CopilotBridge.UnitTests;

public sealed class SourcePoolRoutingTests
{
    [Fact]
    public async Task TransientFailureRetriesAnotherMemberAndPinsLaterTurnsThere()
    {
        await using var fixture = await Fixture.Create((request, _) => Task.FromResult(
            Reply(request.RequestUri!.AbsolutePath.Contains("source-a") ? 429 : 200, "upstream-body")));
        using var first = await fixture.Send("session-one");
        Assert.Equal(HttpStatusCode.OK, first.StatusCode);
        Assert.Equal("upstream-body", await first.Content.ReadAsStringAsync());
        using var second = await fixture.Send("session-one");
        Assert.Equal(HttpStatusCode.OK, second.StatusCode);
        Assert.Equal(new[] { "/sources/source-a/codex/responses", "/sources/source-b/codex/responses", "/sources/source-b/codex/responses" },
            fixture.Captured.Select(item => item.Uri.AbsolutePath));
        foreach (var item in fixture.Captured)
        {
            Assert.Equal("127.0.0.1", item.Uri.Host); Assert.Equal(47001, item.Uri.Port);
            using var body = JsonDocument.Parse(item.Body);
            Assert.Equal(item.Uri.AbsolutePath.Contains("source-a") ? "deployment-a" : "deployment-b", body.RootElement.GetProperty("model").GetString());
            Assert.Equal("exec", body.RootElement.GetProperty("tools")[0].GetProperty("name").GetString());
            Assert.Equal("keep-me", body.RootElement.GetProperty("unknown_field").GetString());
        }
    }

    [Fact]
    public async Task LiveWeightChangeAffectsNewSessionsWithoutMovingAnExistingSession()
    {
        await using var fixture = await Fixture.Create((_, _) => Task.FromResult(Reply(200, "ok")));
        using var one = await fixture.Send("old");
        fixture.Save(fixture.Pool with { Members = fixture.Pool.Members.Select(m => m with { Weight = m.Id == "a" ? 0 : 10 }).ToArray() });
        using var two = await fixture.Send("old");
        using var three = await fixture.Send("new");
        Assert.Equal(new[] { true, true, false }, fixture.Captured.Select(item => item.Uri.AbsolutePath.Contains("source-a")));
    }

    [Fact]
    public async Task InvalidRequestsAreNotRetriedAndExhaustedPoolsStopAfterEachMemberOnce()
    {
        await using (var fixture = await Fixture.Create((_, _) => Task.FromResult(Reply(400, "invalid-request"))))
        {
            using var response = await fixture.Send("bad");
            Assert.Equal(HttpStatusCode.BadRequest, response.StatusCode);
            Assert.Equal("invalid-request", await response.Content.ReadAsStringAsync()); Assert.Single(fixture.Captured);
        }
        await using (var fixture = await Fixture.Create((_, _) => Task.FromResult(Reply(503, "unavailable"))))
        {
            using var response = await fixture.Send("all-down");
            Assert.Equal(HttpStatusCode.ServiceUnavailable, response.StatusCode); Assert.Equal(2, fixture.Captured.Count);
            using var next = await fixture.Send("new-while-cooling");
            Assert.Equal(HttpStatusCode.ServiceUnavailable, next.StatusCode); Assert.Equal(2, fixture.Captured.Count);
        }
    }

    [Fact]
    public async Task NoSessionIdentifierCannotAccidentallyShareOneBindingForAllClients()
    {
        await using var fixture = await Fixture.Create((_, _) => Task.FromResult(Reply(200, "ok")));
        using var response = await fixture.Send(null);
        Assert.Equal(HttpStatusCode.BadRequest, response.StatusCode); Assert.Empty(fixture.Captured);
    }

    [Fact]
    public async Task ServerOnlyHistoryIsNeverSentToAReplacementProvider()
    {
        await using var fixture = await Fixture.Create((_, _) => Task.FromResult(Reply(200, "ok")));
        using var first = await fixture.Send("same");
        fixture.Affinity.Failed(fixture.Pool, fixture.Pool.Members[0]);
        using var next = await fixture.Send("same", "\"previous_response_id\":\"response-at-source-a\",");
        Assert.Equal(HttpStatusCode.Conflict, next.StatusCode); Assert.Single(fixture.Captured);
    }

    [Fact]
    public async Task ClientCancellationStopsTheAttemptWithoutFailoverOrCoolingItsSource()
    {
        // Cancelling a conversation is not evidence of a failed provider. The
        // next request in that conversation must retain its original binding.
        var entered = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        var cancelled = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        var calls = 0;
        await using var fixture = await Fixture.Create(async (_, token) =>
        {
            if (Interlocked.Increment(ref calls) == 1)
            {
                entered.SetResult();
                try { await Task.Delay(Timeout.Infinite, token); }
                finally { cancelled.SetResult(); }
            }
            return Reply(200, "continued");
        });
        using var cancellation = new CancellationTokenSource();
        var pending = fixture.Send("cancelled-session", cancellation: cancellation.Token);
        await entered.Task.WaitAsync(TimeSpan.FromSeconds(5));
        cancellation.Cancel();
        await Assert.ThrowsAnyAsync<OperationCanceledException>(() => pending);
        await cancelled.Task.WaitAsync(TimeSpan.FromSeconds(5));
        using var next = await fixture.Send("cancelled-session");
        Assert.Equal(HttpStatusCode.OK, next.StatusCode);
        Assert.Equal(2, fixture.Captured.Count);
        Assert.All(fixture.Captured, item => Assert.Contains("source-a", item.Uri.AbsolutePath));
    }

    [Fact]
    public async Task StreamingBytesArePreservedAndPartialOutputCannotBeSplicedWithAnotherSource()
    {
        const string sse = "event: response.completed\ndata: {\"type\":\"response.completed\",\"text\":\"羊驼\"}\n\n";
        await using (var fixture = await Fixture.Create((_, _) => Task.FromResult(Reply(200, sse, "text/event-stream"))))
        {
            using var response = await fixture.Send("bytes");
            Assert.Equal(Encoding.UTF8.GetBytes(sse), await response.Content.ReadAsByteArrayAsync());
            Assert.Single(fixture.Captured);
        }
        await using (var fixture = await Fixture.Create((_, _) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        { Content = new StreamContent(new BrokenStream(Encoding.UTF8.GetBytes(sse))) })))
        {
            await Assert.ThrowsAnyAsync<Exception>(async () =>
            { using var response = await fixture.Send("broken"); await response.Content.ReadAsByteArrayAsync(); });
            Assert.Single(fixture.Captured); Assert.True(fixture.ResponseStarted);
        }
    }

    private static HttpResponseMessage Reply(int status, string body, string contentType = "application/json") =>
        new((HttpStatusCode)status) { Content = new StringContent(body, Encoding.UTF8, contentType) };

    private sealed class Fixture : IAsyncDisposable
    {
        private readonly string _root = Path.Combine(Path.GetTempPath(), "pool-route-" + Guid.NewGuid().ToString("N"));
        private WebApplication _app = null!;
        private HttpClient _client = null!;
        public PoolAffinityStore Affinity { get; private set; } = null!;
        public List<(Uri Uri, string Body)> Captured { get; } = [];
        public bool ResponseStarted { get; private set; }
        public SourcePool Pool { get; } = new() { Id = "balanced", Name = "Balanced", Agent = "codex", Model = "gpt-6-astra", Members = [
            new() { Id = "a", SourceId = "source-a", Model = "deployment-a", Weight = 1 },
            new() { Id = "b", SourceId = "source-b", Model = "deployment-b", Weight = 1 }] };
        private string PoolsFile => Path.Combine(_root, "pools.json");
        public void Save(SourcePool pool) => File.WriteAllText(PoolsFile, JsonSerializer.Serialize(new[] { pool }, JsonContext.Default.SourcePoolArray));
        public static async Task<Fixture> Create(Func<HttpRequestMessage, CancellationToken, Task<HttpResponseMessage>> handler)
        {
            var fixture = new Fixture(); Directory.CreateDirectory(fixture._root); fixture.Save(fixture.Pool);
            var sources = new SourceRegistry(fixture.Pool.Members.Select(member => new ModelSource { Id = member.SourceId,
                Name = member.Id, Endpoint = "https://example.openai.azure.com", Deployments = [member.Model] }));
            var builder = WebApplication.CreateSlimBuilder(); builder.WebHost.UseTestServer(); builder.Logging.ClearProviders();
            fixture.Affinity = new PoolAffinityStore(Path.Combine(fixture._root, "affinity.jsonl"), _ => 0);
            builder.Services.AddSingleton(fixture.Affinity);
            builder.Services.AddSingleton(new SourcePoolRegistry(sources, fixture.PoolsFile));
            builder.Services.AddSingleton(new PoolTransport(new Handler(async (request, token) =>
            {
                fixture.Captured.Add((request.RequestUri!, await request.Content!.ReadAsStringAsync(token)));
                return await handler(request, token);
            })));
            fixture._app = builder.Build();
            fixture._app.Use(async (http, next) =>
            {
                http.Connection.LocalPort = 47001;
                http.Response.OnStarting(() => { fixture.ResponseStarted = true; return Task.CompletedTask; });
                await next(http);
            });
            PoolRouting.Use(fixture._app);
            await fixture._app.StartAsync(); fixture._client = fixture._app.GetTestClient();
            return fixture;
        }
        public Task<HttpResponseMessage> Send(string? session, string extra = "", CancellationToken cancellation = default)
        {
            var request = new HttpRequestMessage(HttpMethod.Post, "/pools/balanced/codex/responses")
            { Content = new StringContent("{" + extra + "\"model\":\"gpt-6-astra\",\"input\":\"hello\",\"tools\":[{\"type\":\"custom\",\"name\":\"exec\"}],\"unknown_field\":\"keep-me\"}", Encoding.UTF8, "application/json") };
            request.Headers.Host = "attacker.invalid:5555";
            if (session is not null) request.Headers.Add("thread-id", session);
            return _client.SendAsync(request, cancellation);
        }
        public async ValueTask DisposeAsync()
        { _client.Dispose(); await _app.DisposeAsync(); Affinity.Dispose(); Directory.Delete(_root, true); }
    }
    private sealed class Handler(Func<HttpRequestMessage, CancellationToken, Task<HttpResponseMessage>> handle) : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken) => handle(request, cancellationToken);
    }
    private sealed class BrokenStream(byte[] prefix) : Stream
    {
        private bool _sent;
        public override bool CanRead => true; public override bool CanSeek => false; public override bool CanWrite => false;
        public override long Length => throw new NotSupportedException(); public override long Position { get => 0; set => throw new NotSupportedException(); }
        public override int Read(byte[] buffer, int offset, int count)
        { if (_sent) throw new IOException("fixture interruption"); _sent = true; prefix.CopyTo(buffer, offset); return prefix.Length; }
        public override ValueTask<int> ReadAsync(Memory<byte> buffer, CancellationToken cancellationToken = default)
        { if (_sent) throw new IOException("fixture interruption"); _sent = true; prefix.CopyTo(buffer); return ValueTask.FromResult(prefix.Length); }
        public override void Flush() { } public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
        public override void SetLength(long value) => throw new NotSupportedException(); public override void Write(byte[] buffer, int offset, int count) => throw new NotSupportedException();
    }
}
