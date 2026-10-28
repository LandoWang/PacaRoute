using System.Text.Json;
using System.Text.RegularExpressions;
using CopilotBridge.Cli.Models;
using Microsoft.Extensions.Logging.Abstractions;

namespace CopilotBridge.Cli.Auth;

internal sealed record AccountProfile(string Id, string Name, string Kind);

internal sealed partial class AccountRegistry
{
    public static readonly AccountProfile Default = new("copilot", "GitHub Copilot", "github");
    public static readonly AccountProfile LocalAzure = new("azure-local", "Local Azure CLI", "azure");
    public IReadOnlyList<AccountProfile> Accounts { get; }
    public string Root { get; }

    public AccountRegistry(IEnumerable<AccountProfile> accounts, string root)
    {
        Accounts = new[] { Default, LocalAzure }.Concat(accounts).ToArray();
        Root = root;
        if (Accounts.Any(a => !ValidId().IsMatch(a.Id) || string.IsNullOrWhiteSpace(a.Name)
                || a.Name.Length > 100 || a.Kind is not ("github" or "azure"))
            || Accounts.Select(a => a.Id).Distinct(StringComparer.Ordinal).Count() != Accounts.Count)
            throw new ArgumentException("Invalid or duplicate account metadata.");
    }

    public static AccountRegistry Load()
    {
        var path = Environment.GetEnvironmentVariable("COPILOT_BRIDGE_ACCOUNTS_FILE");
        var root = string.IsNullOrEmpty(path) ? AppContext.BaseDirectory : Path.GetDirectoryName(Path.GetFullPath(path))!;
        return new(string.IsNullOrEmpty(path) || !File.Exists(path) ? [] :
            JsonSerializer.Deserialize(File.ReadAllText(path), JsonContext.Default.AccountProfileArray) ?? [], root);
    }

    public AccountProfile Find(string id) => Accounts.FirstOrDefault(a => a.Id == id)
        ?? throw new ArgumentException("Unknown account.");

    public string DirectoryFor(string id)
    {
        Find(id);
        var path = Path.Combine(Root, "accounts", id);
        if (OperatingSystem.IsWindows()) Directory.CreateDirectory(path);
        else Directory.CreateDirectory(path, UnixFileMode.UserRead | UnixFileMode.UserWrite | UnixFileMode.UserExecute);
        return path;
    }

    public string? AzureDirectory(string id)
    {
        if (Find(id).Kind != "azure") throw new ArgumentException("Expected an Azure account.");
        return id == "azure-local" ? null : DirectoryFor(id);
    }

    public AuthService CreateGitHub(string id, IHttpClientFactory factory, Action<DeviceCodeChallenge>? challenge = null)
    {
        if (Find(id).Kind != "github") throw new ArgumentException("Expected a GitHub account.");
        if (id == "copilot") return new AuthService(factory, challenge, AuthCommand.LoadLoginProvider());
        var credentials = new CredentialService(factory, TokenStore.CreateAccountCredentialStore(DirectoryFor(id)),
            TimeProvider.System, NullLogger<CredentialService>.Instance, challenge, AuthCommand.LoadLoginProvider());
        return new AuthService(factory, credentials, TimeProvider.System, NullLoggerFactory.Instance,
            enableBackgroundRefresh: true, ownsCredentialService: true);
    }

    [GeneratedRegex(@"\A[a-z][a-z0-9-]{0,62}\z")]
    private static partial Regex ValidId();
}
