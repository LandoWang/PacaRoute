using System.Text.Json;
using System.Text.RegularExpressions;
using CopilotBridge.Cli.Models;

namespace CopilotBridge.Cli.Sources;

internal sealed record ModelSource
{
    public required string Id { get; init; }
    public required string Name { get; init; }
    private string? _kind;
    public string Kind { get => _kind ?? "azure-openai"; init => _kind = value; }
    public required string Endpoint { get; init; }
    private string? _authMode;
    public string AuthMode { get => _authMode ?? "azure-cli"; init => _authMode = value; }
    public string? SubscriptionId { get; init; }
    public required string[] Deployments { get; init; }

    [System.Text.Json.Serialization.JsonIgnore]
    public Uri ResponsesUri => new(Endpoint.TrimEnd('/') + "/responses");
}

internal sealed partial class SourceRegistry
{
    public IReadOnlyList<ModelSource> Sources { get; }
    public SourceRegistry(IEnumerable<ModelSource> sources)
    {
        Sources = sources.Select(Validate).ToArray();
        if (Sources.Select(s => s.Id).Distinct(StringComparer.Ordinal).Count() != Sources.Count)
            throw new ArgumentException("Source IDs must be unique.");
    }

    public ModelSource? Find(string id) => Sources.FirstOrDefault(s => s.Id == id);

    public static SourceRegistry Load()
    {
        var path = Environment.GetEnvironmentVariable("COPILOT_BRIDGE_SOURCES_FILE");
        return new SourceRegistry(string.IsNullOrEmpty(path) ? [] :
            JsonSerializer.Deserialize(File.ReadAllText(path), JsonContext.Default.ModelSourceArray) ?? []);
    }

    internal static ModelSource Validate(ModelSource source)
    {
        if (!SourceId().IsMatch(source.Id) || source.Id == "copilot")
            throw new ArgumentException("Use a unique lowercase source ID (letters, digits and hyphens).");
        if (source.Kind != "azure-openai" || source.AuthMode is not ("azure-cli" or "api-key"))
            throw new ArgumentException("Unsupported source or authentication mode.");
        if (!Uri.TryCreate(source.Endpoint, UriKind.Absolute, out var uri)
            || uri.Scheme != "https" || uri.UserInfo.Length != 0 || uri.Query.Length != 0
            || uri.Fragment.Length != 0 || !uri.IsDefaultPort
            || uri.AbsolutePath.TrimEnd('/') is not ("" or "/openai/v1"))
            throw new ArgumentException("Enter an HTTPS resource endpoint or /openai/v1 URL without credentials or query parameters.");
        if (string.IsNullOrWhiteSpace(source.Name) || source.Name.Length > 100
            || source.Deployments is null || source.Deployments.Length == 0 || source.Deployments.Any(d => !DeploymentId().IsMatch(d))
            || source.Deployments.Distinct(StringComparer.Ordinal).Count() != source.Deployments.Length)
            throw new ArgumentException("Enter a source name and unique deployment names.");
        if (source.SubscriptionId is { Length: > 0 } && !Guid.TryParse(source.SubscriptionId, out _))
            throw new ArgumentException("Subscription ID must be a UUID.");
        return source with { Endpoint = uri.GetLeftPart(UriPartial.Authority) + "/openai/v1" };
    }

    [GeneratedRegex(@"\A[a-z][a-z0-9-]{0,62}\z")]
    private static partial Regex SourceId();
    [GeneratedRegex(@"\A[A-Za-z0-9][A-Za-z0-9._-]{0,159}\z")]
    private static partial Regex DeploymentId();
}
