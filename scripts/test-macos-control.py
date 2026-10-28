#!/usr/bin/env python3
"""Exercise the real Debug bridge with local HTTP fixtures, never real credentials/configs."""
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
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

REPO = Path(__file__).resolve().parents[1]
DOTNET = os.environ.get("BRIDGE_DOTNET", str(Path.home() / ".dotnet/dotnet"))
DEBUG = REPO / "src/CopilotBridge.Cli/bin/Debug/net10.0"


class Upstream(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        self.reply({"data": [
            {"id": "claude-opus-5.5", "vendor": "Anthropic", "supported_endpoints": ["/v1/messages"]},
            {"id": "gpt-6-astra", "vendor": "OpenAI", "supported_endpoints": ["/responses"]},
        ]})

    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", 0)))
        if self.path.endswith("/count_tokens"):
            self.reply({"input_tokens": 999})
        elif self.path.endswith("/responses"):
            data = (REPO / "tests/CopilotBridge.UnitTests/Fixtures/responses-sse-text.txt").read_bytes()
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        else:
            self.reply({"id": "msg_fixture", "type": "message", "role": "assistant", "model": "claude-opus-5.5",
                        "content": [{"type": "text", "text": "fixture response"}], "stop_reason": "end_turn",
                        "usage": {"input_tokens": 100, "output_tokens": 20, "cache_read_input_tokens": 80}})

    def reply(self, body):
        data = json.dumps(body).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


with tempfile.TemporaryDirectory(prefix="bridge-desktop-contract-") as temp:
    root = Path(temp)
    runtime = root / "runtime"
    shutil.copytree(DEBUG, runtime, ignore=shutil.ignore_patterns("*.dat", "*.dat.*", "log", "desktop-activity", "request-traces"))
    settings = json.loads((runtime / "appsettings.json").read_text())
    settings["AutoUpdate"]["EnableAutoUpdate"] = False
    settings["Tracing"]["Enabled"] = False
    (runtime / "appsettings.json").write_text(json.dumps(settings))
    client_config = root / "claude-settings.json"
    client_config.write_text('{"env":{"KEEP":"yes"},"permissions":{"allow":["Read"]}}')
    server = ThreadingHTTPServer(("127.0.0.1", 0), Upstream)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    env = dict(os.environ,
               COPILOT_BRIDGE_DESKTOP="1", COPILOT_BRIDGE_PROXY_MODE="direct",
               COPILOT_BRIDGE_TEST_UPSTREAM_BASE_URL=f"http://127.0.0.1:{server.server_port}",
               COPILOT_BRIDGE_TEST_CLAUDE_SETTINGS_PATH=str(client_config),
               COPILOT_BRIDGE_TEST_CODEX_CONFIG_PATH=str(root / "absent-codex.toml"))
    base = [DOTNET, str(runtime / "copilot-bridge.dll")]

    def command(*args):
        result = subprocess.run(base + list(args), env=env, cwd=runtime, text=True, capture_output=True, timeout=30)
        lines = [json.loads(line) for line in result.stdout.splitlines() if line.startswith("{")]
        assert lines, (result.returncode, result.stderr[-1000:])
        return result.returncode, lines[-1]

    rc, models = command("desktop", "models")
    assert rc == 0 and len(models["models"]) == 2, models
    before = client_config.read_bytes()
    rc, preview = command("desktop", "configure", "--client", "claude-code", "--model", "claude-opus-5.5", "--port", str(port))
    assert rc == 0 and client_config.read_bytes() == before
    rc, applied = command("desktop", "configure", "--client", "claude-code", "--model", "claude-opus-5.5", "--port", str(port),
                          "--apply", "--fingerprint", preview["fingerprint"])
    assert rc == 0 and Path(applied["backup_path"]).read_bytes() == before
    config = json.loads(client_config.read_text())
    assert config["env"]["KEEP"] == "yes" and config["env"]["ANTHROPIC_MODEL"] == "claude-opus-5.5"
    rc, stale = command("desktop", "configure", "--client", "claude-code", "--apply", "--fingerprint", preview["fingerprint"])
    assert rc != 0 and not stale["ok"]

    with (root / "stderr.txt").open("w") as errors:
        child = subprocess.Popen(base + ["serve", "--port", str(port)], env=env, cwd=runtime,
                                 stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=errors, text=True)
        try:
            deadline = time.monotonic() + 30
            ready = False
            while time.monotonic() < deadline and child.poll() is None:
                if select.select([child.stdout], [], [], 0.2)[0]:
                    line = child.stdout.readline()
                    if '"event":"ready"' in line:
                        ready = True
                        break
            assert ready, (root / "stderr.txt").read_text()[-2500:]
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

            def post(path, body):
                request = urllib.request.Request(f"http://127.0.0.1:{port}{path}", data=json.dumps(body).encode(),
                                                 headers={"Content-Type": "application/json"})
                try:
                    with opener.open(request, timeout=15) as response:
                        return response.status, response.read()
                except urllib.error.HTTPError as error:
                    return error.code, error.read()

            message = {"model": "claude-opus-5.5", "messages": [{"role": "user", "content": "fixture prompt must not be logged"}], "max_tokens": 64, "stream": False}
            status, body = post("/cc/v1/messages", message)
            assert status == 200, (status, body[:1000])
            status, body = post("/cc/v1/messages/count_tokens", {"model": message["model"], "messages": message["messages"]})
            assert status == 200, (status, body[:1000])
            status, body = post("/codex/responses", {"model": "gpt-6-astra", "input": [{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "fixture input"}]}], "stream": True})
            assert status == 200, (status, body[:1000])
            post("/cc/v1/messages", dict(message, model="model-that-does-not-exist"))
            time.sleep(0.3)
            events = [json.loads(line) for file in (runtime / "desktop-activity").glob("*.jsonl") for line in file.read_text().splitlines()]
            requests = [item for item in events if item["kind"] == "request"]
            assert any(item.get("input_tokens") == 100 and item.get("output_tokens") == 20 for item in requests), requests
            assert any(item.get("client") == "Codex" for item in requests), requests
            assert any(item["kind"] == "estimate" for item in events), events
            assert any(item.get("status", 200) >= 400 for item in requests), requests
            assert "fixture prompt" not in json.dumps(events) and "fixture response" not in json.dumps(events)
            print("PASS: real CLI JSON, live model discovery, preview/apply/stale protection, private backup, readiness, Claude/Codex HTTP, usage, errors, and no prompt/body telemetry")
        finally:
            if child.poll() is None:
                child.send_signal(signal.SIGINT)
                try: child.wait(timeout=10)
                except subprocess.TimeoutExpired: child.kill(); child.wait()
            server.shutdown()
