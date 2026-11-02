#!/usr/bin/env python3
"""Exercise the offline desktop recovery CLI using disposable client configuration."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

REPO = Path(__file__).resolve().parents[1]
DOTNET = os.environ.get("BRIDGE_DOTNET", str(Path.home() / ".dotnet/dotnet"))
ENGINE = Path(sys.argv[1]) if len(sys.argv) > 1 else REPO / "src/CopilotBridge.Cli/bin/Debug/net10.0/copilot-bridge.dll"
BASE = [DOTNET, str(ENGINE)] if ENGINE.suffix == ".dll" else [str(ENGINE)]

with tempfile.TemporaryDirectory(prefix="bridge-restore-cli-") as temp:
    root = Path(temp)
    codex = root / "codex" / "config.toml"
    codex.parent.mkdir()
    claude = root / "claude-settings.json"
    sources = root / "sources.json"
    sources.write_text("[]")
    env = dict(os.environ, CODEX_HOME=str(codex.parent),
               COPILOT_BRIDGE_TEST_CLAUDE_SETTINGS_PATH=str(claude),
               COPILOT_BRIDGE_SOURCES_FILE=str(sources))

    def command(*args, ok=True):
        result = subprocess.run(BASE + ["desktop", *args], env=env, capture_output=True, text=True, timeout=30)
        replies = [json.loads(line) for line in result.stdout.splitlines() if line.startswith("{")]
        assert replies, (result.returncode, result.stderr[-500:])
        reply = replies[-1]
        assert reply["ok"] == ok and (result.returncode == 0) == ok, reply
        return reply

    cases = [("codex", codex, b'# personal\r\nmodel_provider = "azure_astra"\r\nmodel = "old-model"\r\n')]
    # Release builds intentionally do not honor the Claude fixture override.
    if ENGINE.suffix == ".dll":
        cases.append(("claude-code", claude, b'{"env":{"CUSTOM":"keep"},"permissions":{"allow":["Read"]}}\r\n'))

    for client, path, original in cases:
        path.write_bytes(original)
        preview = command("configure", "--client", client, "--model", "gpt-6-astra")
        applied = command("configure", "--client", client, "--model", "gpt-6-astra",
                          "--apply", "--fingerprint", preview["fingerprint"])
        connected = path.read_bytes()
        assert connected != original
        backup = Path(applied["backup_path"])
        assert backup.read_bytes() == original
        # Recovery must work even when configured sources can no longer be loaded.
        sources.write_text("invalid-source-metadata")
        listing = command("config-backups", "--client", client)
        assert listing["path"] == str(path)
        assert any(b["id"] == backup.name and not b["bridge_configured"] for b in listing["backups"])
        restore = command("restore-config", "--client", client, "--backup", backup.name)
        assert path.read_bytes() == connected
        command("restore-config", "--client", client, "--backup", backup.name, "--apply", ok=False)
        assert path.read_bytes() == connected
        restored = command("restore-config", "--client", client, "--backup", backup.name,
                           "--apply", "--fingerprint", restore["fingerprint"])
        assert path.read_bytes() == original
        assert Path(restored["backup_path"]).read_bytes() == connected
        stale = command("restore-config", "--client", client, "--backup", backup.name)
        path.write_bytes(original + b" ")
        command("restore-config", "--client", client, "--backup", backup.name,
                "--apply", "--fingerprint", stale["fingerprint"], ok=False)
        assert path.read_bytes() == original + b" "
        command("restore-config", "--client", client, "--backup", "../" + backup.name, ok=False)
        sources.write_text("[]")

        path.unlink()
        preview = command("configure", "--client", client)
        applied = command("configure", "--client", client, "--apply", "--fingerprint", preview["fingerprint"])
        marker = Path(applied["backup_path"])
        assert marker.suffix == ".absent"
        restore = command("restore-config", "--client", client, "--backup", marker.name)
        command("restore-config", "--client", client, "--backup", marker.name,
                "--apply", "--fingerprint", restore["fingerprint"])
        assert not path.exists()
        print("PASS", client, "apply / preview / restore / private undo snapshot / stale guard / initially missing file")
