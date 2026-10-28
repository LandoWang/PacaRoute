using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Text.RegularExpressions;
using CopilotBridge.Cli.Hosting.ClientConfig;

namespace CopilotBridge.Cli.Desktop;

internal static partial class DesktopClientConfiguration
{
    internal static IClientConfigurator Client(string id) => id switch
    {
        "codex" => new CodexConfigurator(),
        "claude-code" => new ClaudeCodeConfigurator(),
        _ => throw new ArgumentException("Choose codex or claude-code."),
    };

    internal static ConfigPlan Plan(string client, int port, string? model, string source = "copilot")
    {
        if (port is < 1024 or > 65535) throw new ArgumentException("Port must be between 1024 and 65535.");
        var plan = Client(client).Plan(new BridgeConnection(port, source), ConfigScope.Global);
        if (string.IsNullOrEmpty(model)) return plan;
        return plan with
        {
            NewContent = WithModel(client, plan.NewContent, model),
            Summary = [.. plan.Summary, $"Default model = {model}"],
        };
    }

    internal static string WithModel(string client, string content, string model)
    {
        if (!ModelId().IsMatch(model)) throw new ArgumentException("Invalid model ID.");
        if (client == "codex")
        {
            var doc = Tomlyn.Parsing.SyntaxParser.Parse(content);
            if (doc.HasErrors) throw new InvalidOperationException("Existing Codex config is invalid TOML.");
            var oldValue = doc.KeyValues.FirstOrDefault(k => k.Key?.ToString().Trim() == "model")?.Value;
            if (oldValue is Tomlyn.Syntax.StringValueSyntax oldString)
            {
                // Change only the literal token; attached whitespace/comments stay intact.
                oldString.Token!.Text = $"\"{model}\"";
            }
            else if (oldValue is null) CodexConfigurator.UpsertTopLevelString(doc, "model", model);
            else throw new InvalidOperationException("The Codex model value must be a string.");
            var result = doc.ToString();
            if (Tomlyn.Parsing.SyntaxParser.Parse(result).HasErrors)
                throw new InvalidOperationException("Refusing to write invalid TOML.");
            return result;
        }
        if (client != "claude-code") throw new ArgumentException("Unsupported client.");
        var root = JsonNode.Parse(content) as JsonObject
            ?? throw new InvalidOperationException("Claude settings must be a JSON object.");
        var env = root["env"] as JsonObject ?? new JsonObject();
        if (root["env"] is null) root["env"] = env;
        else if (root["env"] is not JsonObject) throw new InvalidOperationException("Claude env must be an object.");
        env["ANTHROPIC_MODEL"] = model;
        return root.ToJsonString(new JsonSerializerOptions { WriteIndented = true }) + "\n";
    }

    internal static string? ReadModel(string client, string path)
    {
        if (!File.Exists(path)) return null;
        var content = File.ReadAllText(path);
        if (client == "codex")
        {
            var doc = Tomlyn.Parsing.SyntaxParser.Parse(content);
            if (doc.HasErrors) throw new InvalidOperationException("Invalid TOML configuration.");
            return CodexConfigurator.FindTopLevelString(doc, "model");
        }
        return JsonNode.Parse(content)?["env"]?["ANTHROPIC_MODEL"]?.GetValue<string>();
    }

    internal static string Fingerprint(string? content) => Convert.ToHexString(
        SHA256.HashData(Encoding.UTF8.GetBytes(content is null ? "missing" : "content:" + content)));

    internal static string? Apply(ConfigPlan plan, string expected)
    {
        var parent = Path.GetDirectoryName(plan.TargetPath)!;
        Directory.CreateDirectory(parent);
        using var guard = new FileStream(plan.TargetPath + ".copilot-bridge.lock",
            FileMode.OpenOrCreate, FileAccess.ReadWrite, FileShare.None);
        var current = File.Exists(plan.TargetPath) ? File.ReadAllText(plan.TargetPath) : null;
        if (Fingerprint(current) != expected || current != plan.OriginalContent)
            throw new InvalidOperationException("Configuration changed since preview. Refresh and preview again.");
        if (plan.IsNoOp) return null;
        string? backup = null;
        if (current is not null)
        {
            backup = plan.TargetPath + $".bridge-{DateTime.UtcNow:yyyyMMdd-HHmmss}-{Guid.NewGuid():N}.bak";
            WritePrivate(backup, File.ReadAllBytes(plan.TargetPath));
        }
        var temporary = plan.TargetPath + $".{Guid.NewGuid():N}.tmp";
        try
        {
            WritePrivate(temporary, plan.NewContent);
            File.Move(temporary, plan.TargetPath, overwrite: true);
        }
        finally { if (File.Exists(temporary)) File.Delete(temporary); }
        return backup;
    }

    private static void WritePrivate(string path, string content)
        => WritePrivate(path, Encoding.UTF8.GetBytes(content));

    private static void WritePrivate(string path, byte[] content)
    {
        var options = new FileStreamOptions { Mode = FileMode.CreateNew, Access = FileAccess.Write };
        if (!OperatingSystem.IsWindows()) options.UnixCreateMode = UnixFileMode.UserRead | UnixFileMode.UserWrite;
        using var stream = new FileStream(path, options);
        stream.Write(content);
        stream.Flush(flushToDisk: true);
    }

    [GeneratedRegex(@"\A[A-Za-z0-9][A-Za-z0-9._:/-]{0,159}\z")]
    private static partial Regex ModelId();
}
