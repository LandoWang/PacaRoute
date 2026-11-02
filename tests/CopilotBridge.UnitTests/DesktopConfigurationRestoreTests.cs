using System.Text;
using System.Text.Json;
using CopilotBridge.Cli.Desktop;
using CopilotBridge.Cli.Hosting.ClientConfig;
using CopilotBridge.Cli.Models;
using Xunit;

namespace CopilotBridge.UnitTests;

/// <summary>Contract: recovery is exact, reversible, private, client-scoped and preview-bound.</summary>
public sealed class DesktopConfigurationRestoreTests : IDisposable
{
    private readonly string _root = Path.Combine(Path.GetTempPath(), "bridge-restore-" + Guid.NewGuid().ToString("N"));
    public DesktopConfigurationRestoreTests() => Directory.CreateDirectory(_root);
    public void Dispose() => Directory.Delete(_root, true);

    [Theory]
    [InlineData("codex", "config.toml", "# comment\r\nmodel_provider = \"azure_astra\"\r\n[model_providers.azure_astra]\r\napi_key = \"private-original-key\"\r\n", "model_provider = \"copilot-bridge\"\n")]
    [InlineData("claude-code", "settings.json", "{\r\n  \"env\": {\"ANTHROPIC_AUTH_TOKEN\": \"private-original-key\"},\r\n  \"permissions\": {\"allow\": [\"Read\"]}\r\n}\r\n", "{\"env\":{\"ANTHROPIC_BASE_URL\":\"http://localhost:8766/cc\"}}")]
    public void Restore_PreservesExactOriginalBytesAndCanUndoRestoration(string client, string name, string original, string connected)
    {
        var target = Path.Combine(_root, name);
        byte[] bytes = [0xef, 0xbb, 0xbf, .. Encoding.UTF8.GetBytes(original)];
        File.WriteAllBytes(target, bytes);
        var backup = DesktopClientConfiguration.Apply(new ConfigPlan(client, ConfigScope.Global,
            target, connected, original, []), DesktopClientConfiguration.Fingerprint(original))!;
        var listing = DesktopConfigurationRestore.List(client, target);
        Assert.Single(listing);
        Assert.False(listing[0].BridgeConfigured);
        var preview = DesktopConfigurationRestore.Plan(client, target, Path.GetFileName(backup));
        Assert.Equal(connected, File.ReadAllText(target));
        Assert.DoesNotContain("private-original-key", JsonSerializer.Serialize(
            new DesktopReply { Backups = listing, Changes = preview.Summary }, JsonContext.Default.DesktopReply));
        var undo = DesktopConfigurationRestore.Apply(preview, preview.Fingerprint)!;
        Assert.Equal(bytes, File.ReadAllBytes(target));
        Assert.Equal(connected, File.ReadAllText(undo));
        var reverse = DesktopConfigurationRestore.Plan(client, target, Path.GetFileName(undo));
        DesktopConfigurationRestore.Apply(reverse, reverse.Fingerprint);
        Assert.Equal(connected, File.ReadAllText(target));
        if (!OperatingSystem.IsWindows())
        {
            Assert.Equal(UnixFileMode.UserRead | UnixFileMode.UserWrite, File.GetUnixFileMode(undo));
            Assert.Equal(UnixFileMode.UserRead | UnixFileMode.UserWrite, File.GetUnixFileMode(target));
        }
    }

    [Fact]
    public void InitiallyMissingConfiguration_IsRemovedAndCanBeRecreatedByUndo()
    {
        var path = Path.Combine(_root, "settings.json");
        const string content = "{\"env\":{\"ANTHROPIC_BASE_URL\":\"http://localhost:8765/cc\"}}";
        var snapshot = DesktopClientConfiguration.Apply(new ConfigPlan("claude-code", ConfigScope.Global,
            path, content, null, []), DesktopClientConfiguration.Fingerprint(null))!;
        Assert.True(DesktopConfigurationRestore.List("claude-code", path).Single().MissingFile);
        var plan = DesktopConfigurationRestore.Plan("claude-code", path, Path.GetFileName(snapshot));
        var undo = DesktopConfigurationRestore.Apply(plan, plan.Fingerprint)!;
        Assert.False(File.Exists(path));
        Assert.True(File.Exists(snapshot));
        var reverse = DesktopConfigurationRestore.Plan("claude-code", path, Path.GetFileName(undo));
        DesktopConfigurationRestore.Apply(reverse, reverse.Fingerprint);
        Assert.Equal(content, File.ReadAllText(path));
    }

    [Theory]
    [InlineData(true)]
    [InlineData(false)]
    public void ChangingCurrentFileOrSelectedBackupAfterPreview_AbortsWithoutWriting(bool changeCurrent)
    {
        var path = Path.Combine(_root, "config.toml");
        File.WriteAllText(path, "model = \"current\"\n");
        var backup = DesktopConfigurationRestore.Snapshot(path, Encoding.UTF8.GetBytes("model = \"original\"\n"));
        var plan = DesktopConfigurationRestore.Plan("codex", path, Path.GetFileName(backup));
        File.WriteAllText(changeCurrent ? path : backup, "model = \"external-edit\"\n");
        var before = File.ReadAllBytes(path);
        var files = Directory.GetFiles(_root, "*.bak").Length;
        Assert.Throws<InvalidOperationException>(() => DesktopConfigurationRestore.Apply(plan, plan.Fingerprint));
        Assert.Equal(before, File.ReadAllBytes(path));
        Assert.Equal(files, Directory.GetFiles(_root, "*.bak").Length);
    }

    [Theory]
    [InlineData("../../config.toml")]
    [InlineData("settings.json.bridge-20261028-080000-0123456789abcdef0123456789abcdef.bak")]
    [InlineData("config.toml.bridge-20269999-080000-0123456789abcdef0123456789abcdef.bak")]
    public void BackupOutsideClientNamespace_IsRejected(string id)
    {
        var path = Path.Combine(_root, "config.toml");
        File.WriteAllText(path, "# unchanged");
        Assert.Throws<InvalidOperationException>(() => DesktopConfigurationRestore.Plan("codex", path, id));
        Assert.Equal("# unchanged", File.ReadAllText(path));
    }

    [Fact]
    public void InvalidBackup_IsVisibleButCannotBeRestored_AndBrokenCurrentFileCanBeRepaired()
    {
        var path = Path.Combine(_root, "settings.json");
        File.WriteAllText(path, "broken current file");
        var bad = DesktopConfigurationRestore.Snapshot(path, Encoding.UTF8.GetBytes("secret invalid JSON"));
        Assert.False(DesktopConfigurationRestore.List("claude-code", path).Single().Valid);
        var ex = Assert.Throws<InvalidOperationException>(() => DesktopConfigurationRestore.Plan("claude-code", path, Path.GetFileName(bad)));
        Assert.DoesNotContain("secret", ex.Message);
        var good = DesktopConfigurationRestore.Snapshot(path, Encoding.UTF8.GetBytes("{}"));
        var plan = DesktopConfigurationRestore.Plan("claude-code", path, Path.GetFileName(good));
        var undo = DesktopConfigurationRestore.Apply(plan, plan.Fingerprint)!;
        Assert.Equal("{}", File.ReadAllText(path));
        Assert.Equal("broken current file", File.ReadAllText(undo));
    }

    [Fact]
    public void SymlinkBackupOrTarget_IsRejected()
    {
        if (OperatingSystem.IsWindows()) return;
        var path = Path.Combine(_root, "config.toml");
        File.WriteAllText(path, "model = \"safe\"");
        var backup = DesktopConfigurationRestore.Snapshot(path, Encoding.UTF8.GetBytes("# original"));
        File.Delete(backup);
        File.CreateSymbolicLink(backup, path);
        Assert.Empty(DesktopConfigurationRestore.List("codex", path));
        Assert.Throws<InvalidOperationException>(() => DesktopConfigurationRestore.Plan("codex", path, Path.GetFileName(backup)));
        File.Delete(backup);
        File.WriteAllText(backup, "# original");
        File.Delete(path);
        File.CreateSymbolicLink(path, backup);
        Assert.Throws<InvalidOperationException>(() => DesktopConfigurationRestore.Plan("codex", path, Path.GetFileName(backup)));
        Assert.Equal("# original", File.ReadAllText(backup));
    }

    [Fact]
    public void EmptyFileAndMissingFileRemainDistinct_AndIdenticalRestoreDoesNotCreateBackup()
    {
        var path = Path.Combine(_root, "config.toml");
        File.WriteAllBytes(path, []);
        var backup = DesktopConfigurationRestore.Snapshot(path, []);
        var plan = DesktopConfigurationRestore.Plan("codex", path, Path.GetFileName(backup));
        Assert.Null(DesktopConfigurationRestore.Apply(plan, plan.Fingerprint));
        Assert.Single(DesktopConfigurationRestore.List("codex", path));
        File.Delete(path);
        Assert.Throws<InvalidOperationException>(() => DesktopConfigurationRestore.Apply(plan, plan.Fingerprint));
    }
}
