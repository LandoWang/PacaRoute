"""Local TLS fault fixture. Trust is scoped to a QA-only entry point, never the OS.

The entry point runs the same pacaroute.Run implementation. Manifests identify
this separate binary honestly; packaged-binary verification remains a separate run.
"""
import http.server
import json
import os
from pathlib import Path
import ssl
import subprocess
import threading


class StreamFixture:
    def __init__(self, root: Path, run: Path, mode: str):
        self.calls = 0
        self.mode = mode
        self.directory = run / "_fault-fixture"
        self.directory.mkdir(mode=0o700)
        config = self.directory / "openssl.cnf"
        config.write_text("""[req]
distinguished_name=dn
x509_extensions=extensions
prompt=no
[dn]
CN=PacaRoute isolated stream fixture
[extensions]
subjectAltName=IP:127.0.0.1
basicConstraints=critical,CA:TRUE
keyUsage=critical,digitalSignature,keyEncipherment,keyCertSign
extendedKeyUsage=serverAuth
""")
        cert, key = self.directory / "cert.pem", self.directory / "key.pem"
        subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
                        "-config", str(config), "-keyout", str(key), "-out", str(cert)],
                       capture_output=True, check=True)
        key.chmod(0o600)
        roots = self.directory / "roots.pem"
        roots.write_bytes(Path("/etc/ssl/cert.pem").read_bytes() + b"\n" + cert.read_bytes())
        shim = self.directory / "main.go"
        shim.write_text('''package main
import ("crypto/x509"; "os"; "github.com/LandoWang/pacaroute/internal/pacaroute")
func main() {
  pem, err := os.ReadFile(os.Getenv("PACAROUTE_QA_CA_FILE")); if err != nil { panic(err) }
  roots := x509.NewCertPool(); if !roots.AppendCertsFromPEM(pem) { panic("invalid QA roots") }
  x509.SetFallbackRoots(roots)
  os.Exit(pacaroute.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
''')
        overlay = self.directory / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(root / "cmd/pacaroute/main.go"): str(shim)}}))
        self.binary = self.directory / "pacaroute-qa"
        subprocess.run([os.environ.get("PACAROUTE_GO", "go"), "build", "-tags", "desktop", "-trimpath",
                        "-overlay", str(overlay), "-o", str(self.binary), "./cmd/pacaroute"], cwd=root, check=True)
        self.env = {"GODEBUG": "x509usefallbackroots=1", "PACAROUTE_QA_CA_FILE": str(roots)}
        fixture = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def do_POST(self):
                self.rfile.read(int(self.headers.get("Content-Length", 0)))
                fixture.calls += 1
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Connection", "close")
                self.end_headers()
                frames = [
                    {"type": "response.created", "response": {"id": "resp_qa_interrupted", "object": "response", "status": "in_progress", "model": "gpt-6-astra", "output": []}},
                    {"type": "response.output_item.added", "output_index": 0, "item": {"id": "msg_qa", "type": "message", "role": "assistant", "status": "in_progress", "content": []}},
                    {"type": "response.content_part.added", "output_index": 0, "item_id": "msg_qa", "content_index": 0, "part": {"type": "output_text", "text": "", "annotations": []}},
                    {"type": "response.output_text.delta", "output_index": 0, "item_id": "msg_qa", "content_index": 0, "delta": "QA stream interrupted. "},
                ]
                if fixture.mode == "error":
                    # Redacted Azure wire shape: Codex ignores this nested error
                    # and requires the following response.failed terminal.
                    frames.extend([
                        {"type": "error", "error": {"code": "rate_limit_reached", "message": "QA upstream overloaded"}},
                        {"type": "response.failed", "response": {"id": "resp_qa_interrupted", "object": "response", "status": "failed", "error": {"code": "rate_limit_exceeded", "message": "QA upstream overloaded"}}},
                    ])
                for index, frame in enumerate(frames):
                    frame["sequence_number"] = index
                    self.wfile.write(("event: " + frame["type"] + "\ndata: " + json.dumps(frame) + "\n\n").encode())
                    self.wfile.flush()
                self.close_connection = True

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        assert self.server.server_port != 8765
        tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.load_cert_chain(cert, key)
        self.server.socket = tls.wrap_socket(self.server.socket, server_side=True)
        self.endpoint = f"https://127.0.0.1:{self.server.server_port}"
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def close(self):
        self.server.shutdown()
        self.server.server_close()
