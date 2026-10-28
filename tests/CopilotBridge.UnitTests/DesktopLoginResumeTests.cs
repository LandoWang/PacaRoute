using CopilotBridge.Cli.Auth;
using CopilotBridge.Cli.Copilot;
using CopilotBridge.Cli.Desktop;
using CopilotBridge.Cli.Models.GitHub;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace CopilotBridge.UnitTests;

public sealed class DesktopLoginResumeTests : IDisposable
{
    private readonly string _root = Path.Combine(Path.GetTempPath(), "desktop-login-" + Guid.NewGuid().ToString("N"));

    [Fact]
    public async Task Reopening_and_resuming_saved_login_needs_no_network_or_device_authorization()
    {
        var store = SavedCredential();
        var before = File.ReadAllBytes(store.FilePath);
        using var handler = new DenyNetwork();
        using var http = new HttpClient(handler);
        for (var session = 0; session < 2; session++)
        {
            using var auth = Open(store, http);
            Assert.True(auth.GetCredentialStatus() is not null);
            var reply = await DesktopCommand.LoginAsync(auth, reauthorize: false, CancellationToken.None);
            Assert.True(reply.LoggedIn);
        }
        Assert.Empty(handler.Requests);
        Assert.Equal(before, File.ReadAllBytes(store.FilePath));
    }

    [Fact]
    public async Task Explicit_reauthorization_can_start_a_new_flow_without_destroying_the_saved_login()
    {
        var store = SavedCredential();
        var before = File.ReadAllBytes(store.FilePath);
        using var handler = new DenyNetwork();
        using var http = new HttpClient(handler);
        using var auth = Open(store, http);
        await Assert.ThrowsAsync<HttpRequestException>(() =>
            DesktopCommand.LoginAsync(auth, reauthorize: true, CancellationToken.None));
        Assert.Single(handler.Requests);
        Assert.Equal("/login/device/code", handler.Requests[0].AbsolutePath);
        Assert.Equal(before, File.ReadAllBytes(store.FilePath));
    }

    private CredentialStore SavedCredential()
    {
        var store = new CredentialStore(_root, new TestProtector());
        store.Save(new CredentialFileRecord { Version = 1, AccessToken = "dummy-saved-credential",
            CredentialId = "desktop-fixture", Generation = 1 });
        return store;
    }

    private static AuthService Open(CredentialStore store, HttpClient http)
    {
        var factory = new SingleClientHttpClientFactory(http);
        var credentials = new CredentialService(factory, store, TimeProvider.System, NullLogger<CredentialService>.Instance);
        return new AuthService(factory, credentials, TimeProvider.System, NullLoggerFactory.Instance,
            enableBackgroundRefresh: false, ownsCredentialService: true);
    }

    private sealed class DenyNetwork : HttpMessageHandler
    {
        public List<Uri> Requests { get; } = [];
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken ct)
        {
            Requests.Add(request.RequestUri!);
            throw new HttpRequestException("Network intentionally unavailable in this contract check.");
        }
    }

    private sealed class TestProtector : ITokenProtector
    {
        public byte[] Protect(byte[] plaintext) => plaintext.Select(b => (byte)(b ^ 0x5A)).ToArray();
        public byte[] Unprotect(byte[] blob) => Protect(blob);
    }

    public void Dispose() { if (Directory.Exists(_root)) Directory.Delete(_root, recursive: true); }
}
