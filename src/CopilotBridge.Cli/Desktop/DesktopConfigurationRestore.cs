using System.Globalization;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
using CopilotBridge.Cli.Hosting.ClientConfig;

namespace CopilotBridge.Cli.Desktop;

internal sealed record DesktopRestorePlan(string Client, string TargetPath, string BackupId,
    byte[]? Current, byte[]? Restored, string Fingerprint, IReadOnlyList<string> Summary);

/// <summary>Offline, exact-byte undo for the desktop's per-client configuration snapshots.</summary>
internal static partial class DesktopConfigurationRestore
{
    private const string MissingMarker = "copilot-bridge:missing-config:v1\n";

    internal static string Target(string client) => client switch
    {
        "codex" => CodexConfigurator.ResolvePath(),
        "claude-code" => ClaudeCodeConfigurator.ResolvePath(ConfigScope.Global),
        _ => throw new ArgumentException("Choose codex or claude-code."),
    };

    internal static string Snapshot(string target, byte[]? bytes)
    {
        var path = target + $".bridge-{DateTime.UtcNow:yyyyMMdd-HHmmss}-{Guid.NewGuid():N}."
            + (bytes is null ? "absent" : "bak");
        DesktopClientConfiguration.WritePrivate(path, bytes ?? Encoding.UTF8.GetBytes(MissingMarker));
        return path;
    }

    internal static List<DesktopConfigBackup> List(string client, string target)
    {
        var parent = Path.GetDirectoryName(target)!;
        if (!Directory.Exists(parent)) return [];
        var result = new List<DesktopConfigBackup>();
        foreach (var path in Directory.EnumerateFiles(parent, Path.GetFileName(target) + ".bridge-*"))
        {
            var id = Path.GetFileName(path);
            if (!TryIdentity(target, id, out var stamp) || IsLink(path)) continue;
            var missing = id.EndsWith(".absent", StringComparison.Ordinal);
            try
            {
                var bytes = ReadSnapshot(target, id);
                var connected = bytes is not null && Validate(client, bytes);
                result.Add(new(id, stamp, missing, connected, true));
            }
            catch (Exception ex) when (ex is IOException or UnauthorizedAccessException or InvalidOperationException)
            {
                result.Add(new(id, stamp, missing, false, false));
            }
        }
        return result.OrderByDescending(b => b.CreatedAt, StringComparer.Ordinal)
            .ThenByDescending(b => b.Id, StringComparer.Ordinal).ToList();
    }

    internal static DesktopRestorePlan Plan(string client, string target, string id)
    {
        var restored = ReadSnapshot(target, id);
        var connected = restored is not null && Validate(client, restored);
        var current = ReadCurrent(target);
        TryIdentity(target, id, out var stamp);
        var fingerprint = Hash(Encoding.UTF8.GetBytes(target + "\0" + id + "\0" + Hash(current) + "\0" + Hash(restored)));
        return new(client, target, id, current, restored, fingerprint,
        [
            $"备份时间：{stamp}",
            restored is null ? "恢复到原先没有配置文件的状态：移除此配置文件。"
                : connected ? "此备份仍连接 Copilot Bridge。" : "此备份未选择 Copilot Bridge 连接。",
            "将还原整个配置文件，包括来源、模型和其他设置；备份之后的修改也会被撤销。",
            "当前配置会先独立备份，可以再次还原。已打开的会话不会立即切换；请新建会话。",
        ]);
    }

    internal static string? Apply(DesktopRestorePlan plan, string expected)
    {
        using var guard = new FileStream(plan.TargetPath + ".copilot-bridge.lock",
            FileMode.OpenOrCreate, FileAccess.ReadWrite, FileShare.None);
        var fresh = Plan(plan.Client, plan.TargetPath, plan.BackupId);
        if (fresh.Fingerprint != expected || fresh.Fingerprint != plan.Fingerprint)
            throw new InvalidOperationException("配置或备份在预览后发生了变化，请重新预览后再还原。");
        if (Equal(fresh.Current, fresh.Restored)) return null;
        var undo = Snapshot(plan.TargetPath, fresh.Current);
        if (fresh.Restored is null)
        {
            File.Delete(plan.TargetPath);
            return undo;
        }
        var temporary = plan.TargetPath + $".{Guid.NewGuid():N}.tmp";
        try
        {
            DesktopClientConfiguration.WritePrivate(temporary, fresh.Restored);
            File.Move(temporary, plan.TargetPath, overwrite: true);
        }
        finally { if (File.Exists(temporary)) File.Delete(temporary); }
        return undo;
    }

    private static byte[]? ReadSnapshot(string target, string id)
    {
        if (!TryIdentity(target, id, out _))
            throw new InvalidOperationException("请选择此客户端的有效备份。");
        var path = Path.Combine(Path.GetDirectoryName(target)!, id);
        if (IsLink(path)) throw new InvalidOperationException("不能还原符号链接备份。");
        if (!File.Exists(path)) throw new InvalidOperationException("备份不存在，请刷新列表。");
        var bytes = File.ReadAllBytes(path);
        if (!id.EndsWith(".absent", StringComparison.Ordinal)) return bytes;
        if (!bytes.AsSpan().SequenceEqual(Encoding.UTF8.GetBytes(MissingMarker)))
            throw new InvalidOperationException("备份标记无效。");
        return null;
    }

    private static byte[]? ReadCurrent(string path)
    {
        if (IsLink(path) || Directory.Exists(path))
            throw new InvalidOperationException("配置路径是链接或目录，不能自动还原。");
        return File.Exists(path) ? File.ReadAllBytes(path) : null;
    }

    // Validate without exposing parse diagnostics, which may contain credentials.
    private static bool Validate(string client, byte[] bytes)
    {
        try
        {
            var text = new UTF8Encoding(false, true).GetString(bytes).TrimStart('\uFEFF');
            if (client == "codex")
            {
                var doc = Tomlyn.Parsing.SyntaxParser.Parse(text);
                if (doc.HasErrors) throw new InvalidOperationException("备份不是有效的 Codex 配置。");
                return CodexConfigurator.FindTopLevelString(doc, "model_provider") == "copilot-bridge";
            }
            if (client != "claude-code") throw new InvalidOperationException("Unsupported client.");
            using var json = JsonDocument.Parse(text);
            if (json.RootElement.ValueKind != JsonValueKind.Object)
                throw new InvalidOperationException("备份不是有效的 Claude Code 配置。");
            return json.RootElement.TryGetProperty("env", out var env) && env.ValueKind == JsonValueKind.Object
                && env.TryGetProperty("ANTHROPIC_BASE_URL", out var url) && url.ValueKind == JsonValueKind.String
                && Uri.TryCreate(url.GetString(), UriKind.Absolute, out var uri)
                && uri.IsLoopback && uri.AbsolutePath.TrimEnd('/').EndsWith("/cc", StringComparison.Ordinal);
        }
        catch (Exception ex) when (ex is JsonException or DecoderFallbackException)
        { throw new InvalidOperationException("备份格式无效，不能还原。"); }
    }

    private static bool TryIdentity(string target, string id, out string stamp)
    {
        stamp = "";
        var prefix = Path.GetFileName(target) + ".bridge-";
        if (id != Path.GetFileName(id) || !id.StartsWith(prefix, StringComparison.Ordinal)) return false;
        var match = Suffix().Match(id[prefix.Length..]);
        if (!match.Success || !DateTime.TryParseExact(match.Groups[1].Value, "yyyyMMdd-HHmmss",
                CultureInfo.InvariantCulture, DateTimeStyles.AssumeUniversal | DateTimeStyles.AdjustToUniversal, out var date)) return false;
        stamp = date.ToString("yyyy-MM-dd'T'HH:mm:ss'Z'", CultureInfo.InvariantCulture);
        return true;
    }

    private static bool IsLink(string path) => new FileInfo(path).LinkTarget is not null;
    private static string Hash(byte[]? bytes) => bytes is null ? "missing" : Convert.ToHexString(SHA256.HashData(bytes));
    private static bool Equal(byte[]? a, byte[]? b) => a is null ? b is null : b is not null && a.AsSpan().SequenceEqual(b);

    [GeneratedRegex(@"\A(\d{8}-\d{6})-[a-f0-9]{32}\.(bak|absent)\z")]
    private static partial Regex Suffix();
}
