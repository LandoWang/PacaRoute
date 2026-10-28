#!/usr/bin/env python3
"""Offline source × harness contracts through a real Debug bridge process.

This verifies routing/wire contracts, NOT live Azure/client compatibility.
All credentials, endpoints, client files and telemetry are disposable fixtures.
"""
import json
import os
from pathlib import Path
import select
import shutil
import signal
import socket
import subprocess
import tempfile
import threading
import time
import urllib.request
import urllib.error
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

REPO = Path(__file__).resolve().parents[1]
DOTNET = os.environ.get("BRIDGE_DOTNET", str(Path.home() / ".dotnet/dotnet"))
observed = []


class Upstream(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        body = self.rfile.read(int(self.headers["Content-Length"]))
        observed.append((self.path, dict(self.headers), body))
        if self.path.startswith("/azure-other/"):
            data = b'{"error":{"code":"PermissionDenied","message":"fixture rejection"}}'
            self.send_response(403)
            self.send_header("Content-Type", "application/json")
        else:
            data = (REPO / "tests/CopilotBridge.UnitTests/Fixtures/responses-sse-text.txt").read_bytes()
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


with tempfile.TemporaryDirectory(prefix="bridge-source-contract-") as temp:
    root = Path(temp)
    runtime = root / "runtime"
    shutil.copytree(REPO / "src/CopilotBridge.Cli/bin/Debug/net10.0", runtime,
                    ignore=shutil.ignore_patterns("*.dat", "*.dat.*", "log", "desktop-activity", "request-traces"))
    settings = json.loads((runtime / "appsettings.json").read_text())
    settings["AutoUpdate"]["EnableAutoUpdate"] = False
    settings["Tracing"]["Enabled"] = False
    (runtime / "appsettings.json").write_text(json.dumps(settings))
    sources = [{"id": source, "name": source, "endpoint": "https://example.openai.azure.com",
                "auth_mode": "api-key", "deployments": ["company-deployment-4-1-2026-01-01"]}
               for source in ["azure-dev", "azure-other"]]
    source_file = root / "sources.json"
    source_file.write_text(json.dumps(sources))
    claude = root / "claude.json"
    claude.write_text('{"env":{"KEEP":"yes"}}')
    codex_home = root / "codex-home"
    codex_home.mkdir()
    codex = codex_home / "config.toml"
    codex.write_text('model = "old" # preserve\n')
    upstream = ThreadingHTTPServer(("127.0.0.1", 0), Upstream)
    threading.Thread(target=upstream.serve_forever, daemon=True).start()
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0)); port = sock.getsockname()[1]
    env = dict(os.environ, COPILOT_BRIDGE_DESKTOP="1", COPILOT_BRIDGE_PROXY_MODE="direct",
               COPILOT_BRIDGE_SOURCES_FILE=str(source_file),
               COPILOT_BRIDGE_TEST_AZURE_UPSTREAM=f"http://127.0.0.1:{upstream.server_port}/",
               COPILOT_BRIDGE_TEST_CLAUDE_SETTINGS_PATH=str(claude), CODEX_HOME=str(codex_home))
    base = [DOTNET, str(runtime / "copilot-bridge.dll")]

    def command(*args):
        result = subprocess.run(base + list(args), cwd=runtime, env=env, capture_output=True, text=True, timeout=30)
        assert result.returncode == 0, (result.stderr, result.stdout)
        return [json.loads(line) for line in result.stdout.splitlines() if line.startswith('{')][-1]

    model = sources[0]["deployments"][0]
    assert command("desktop", "models", "--source", "azure-dev")["models"][0]["id"] == model
    for client in ["codex", "claude-code"]:
        args = ["desktop", "configure", "--source", "azure-dev", "--client", client, "--model", model, "--port", str(port)]
        before = (codex if client == "codex" else claude).read_bytes()
        preview = command(*args)
        assert Path(preview["path"]) == (codex if client == "codex" else claude), "Refusing to apply outside the isolated fixture"
        assert (codex if client == "codex" else claude).read_bytes() == before
        result = command(*args, "--apply", "--fingerprint", preview["fingerprint"])
        assert Path(result["backup_path"]).read_bytes() == before
    states = command("desktop", "status", "--port", str(port))
    assert not states["logged_in"]
    assert all(c["source"] == "azure-dev" and not c["drifted"] for c in states["clients"]), states
    assert "/sources/azure-dev/codex" in codex.read_text() and "# preserve" in codex.read_text()
    assert json.loads(claude.read_text())["env"]["KEEP"] == "yes"
    assert "/sources/azure-dev/cc" in claude.read_text()
    service_env = dict(env, COPILOT_BRIDGE_SECRETS_STDIN="1")
    with (root / "stderr.txt").open("w") as errors:
        child = subprocess.Popen(base + ["serve", "--port", str(port)], cwd=runtime, env=service_env,
                                 stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=errors, text=True)
        child.stdin.write(json.dumps({"azure-dev": "fixture-key-a", "azure-other": "fixture-key-b"}) + "\n")
        child.stdin.close()
        try:
            deadline = time.monotonic() + 30
            ready = False
            while time.monotonic() < deadline and child.poll() is None:
                if select.select([child.stdout], [], [], 0.2)[0] and '"event":"ready"' in child.stdout.readline():
                    ready = True; break
            assert ready, (root / "stderr.txt").read_text()[-2500:]
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

            def post(path, body):
                req = urllib.request.Request(f"http://127.0.0.1:{port}" + path, data=body,
                    headers={"Content-Type": "application/json", "Authorization": "Bearer downstream-secret",
                             "Copilot-Integration-Id": "must-not-leak"})
                try:
                    with opener.open(req, timeout=20) as response:
                        return response.status, response.read(), dict(response.headers)
                except urllib.error.HTTPError as error:
                    return error.code, error.read(), dict(error.headers)

            native = json.dumps({"model": model, "input": "private-canary-prompt", "stream": True,
                "service_tier": "default", "store": True, "unknown_future_field": {"exact": 42}}).encode()
            status, body, _ = post("/sources/azure-dev/codex/responses", native)
            assert status == 200 and b"response.completed" in body
            assert observed[-1][2] == native
            assert observed[-1][1].get("api-key") == "fixture-key-a"
            assert not any(h.lower() in ["authorization", "copilot-integration-id", "editor-version"] for h in observed[-1][1])
            messages = json.dumps({"model": model, "messages": [{"role": "user", "content": "private-canary-prompt"}],
                                   "max_tokens": 100, "stream": True}).encode()
            status, body, _ = post("/sources/azure-dev/cc/v1/messages", messages)
            assert status == 200 and b"message_stop" in body, (status, body)
            assert json.loads(observed[-1][2])["model"] == model
            before_count = len(observed)
            status, body, headers = post("/sources/azure-dev/cc/v1/messages/count_tokens", messages)
            assert status == 200 and headers["X-Bridge-Token-Count"] == "local-estimate"
            assert len(observed) == before_count
            status, body, _ = post("/sources/azure-other/codex/responses", native)
            assert status == 403 and json.loads(body)["error"]["code"] == "PermissionDenied"
            assert observed[-1][1].get("api-key") == "fixture-key-b"
            assert post("/sources/missing/codex/responses", native)[0] == 404
            assert post("/sources/azure-dev/codex/responses", b'{"model":"other"}')[0] == 400
            assert post("/codex/responses", native)[0] == 401
        finally:
            child.send_signal(signal.SIGINT)
            try: child.wait(timeout=8)
            except subprocess.TimeoutExpired: child.kill(); child.wait()
            upstream.shutdown()
    activity = "".join(p.read_text() for p in (runtime / "desktop-activity").glob("*.jsonl"))
    events = [json.loads(line) for line in activity.splitlines()]
    assert any(e.get("source") == "azure-dev" and e.get("client") == "Claude Code" for e in events)
    assert any(e.get("source") == "azure-other" and e.get("status") == 403 for e in events)
    assert "private-canary-prompt" not in activity and "fixture-key" not in activity and "downstream-secret" not in activity
    print("PASS: Azure-only startup, source × harness configuration, native Responses fidelity, Claude translation, source isolation, local estimates and secret-free telemetry")
