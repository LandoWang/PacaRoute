#!/usr/bin/env python3
"""Transport contracts: concurrent sources choose immutable proxies; no fallback.
Uses a disposable Debug engine, fake credentials and local upstream/proxy servers.
"""
import concurrent.futures
import json
import os
from pathlib import Path
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
DOTNET = str(Path.home() / '.dotnet/dotnet')
observed = []
class Server(BaseHTTPRequestHandler):
    # Model a persistent HTTP/1.1 proxy; the default HTTP/1.0 fixture can close
    # a pooled proxy connection while the next POST is being sent.
    protocol_version = "HTTP/1.1"
    def log_message(self, *_): pass
    def do_POST(self):
        data = self.rfile.read(int(self.headers['Content-Length']))
        observed.append((self.server.label, self.path, self.headers.get('api-key'), data))
        body = b'{"id":"fixture","object":"response","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1}}'
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers(); self.wfile.write(body)

def server(label):
    s = ThreadingHTTPServer(('127.0.0.1', 0), Server); s.label = label
    threading.Thread(target=s.serve_forever, daemon=True).start()
    return s

servers = [server(x) for x in ['upstream', 'proxy-a', 'proxy-b']]
try:
    with tempfile.TemporaryDirectory(prefix='agentlink-proxy-test-') as tmp:
        root = Path(tmp); runtime = root / 'runtime'
        shutil.copytree(REPO / 'src/CopilotBridge.Cli/bin/Debug/net10.0', runtime,
                        ignore=shutil.ignore_patterns('*.dat', '*.dat.*', 'log', 'desktop-activity', 'request-traces'))
        settings = json.loads((runtime / 'appsettings.json').read_text())
        settings['AutoUpdate']['EnableAutoUpdate'] = False; settings['Tracing']['Enabled'] = False
        (runtime / 'appsettings.json').write_text(json.dumps(settings))
        ids = ['azure-a', 'azure-b', 'azure-direct', 'azure-failed']
        (root / 'sources.json').write_text(json.dumps([{'id': x, 'name': x, 'endpoint': 'https://example.openai.azure.com',
            'auth_mode': 'api-key', 'deployments': ['shared-model']} for x in ids]))
        (root / 'accounts.json').write_text('[]')
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0)); closed = sock.getsockname()[1]
        config = {'profiles': [{'id': 'proxy-a', 'name': 'A', 'mode': 'custom', 'address': f'http://127.0.0.1:{servers[1].server_port}'},
                    {'id': 'proxy-b', 'name': 'B', 'mode': 'custom', 'address': f'http://127.0.0.1:{servers[2].server_port}'},
                    {'id': 'failed', 'name': 'Failed', 'mode': 'custom', 'address': f'http://127.0.0.1:{closed}'}],
                  'bindings': dict(zip(ids, ['proxy-a', 'proxy-b', 'direct', 'failed'])), 'account_proxy_id': 'system'}
        (root / 'proxies.json').write_text(json.dumps(config))
        env = dict(os.environ, COPILOT_BRIDGE_DESKTOP='1', COPILOT_BRIDGE_SECRETS_STDIN='1',
            COPILOT_BRIDGE_SOURCES_FILE=str(root / 'sources.json'), COPILOT_BRIDGE_ACCOUNTS_FILE=str(root / 'accounts.json'),
            COPILOT_BRIDGE_PROXIES_FILE=str(root / 'proxies.json'),
            COPILOT_BRIDGE_TEST_AZURE_UPSTREAM=f'http://127.0.0.1:{servers[0].server_port}/')
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0)); port = sock.getsockname()[1]
        with (root / 'out').open('w') as out, (root / 'err').open('w') as err:
            process = subprocess.Popen([DOTNET, str(runtime / 'copilot-bridge.dll'), 'serve', '--port', str(port)],
                env=env, cwd=runtime, stdin=subprocess.PIPE, stdout=out, stderr=err)
            process.stdin.write((json.dumps({x: x + '-fixture-key' for x in ids}) + '\n').encode()); process.stdin.close()
            try:
                for _ in range(150):
                    if '"event":"ready"' in (root / 'out').read_text(): break
                    if process.poll() is not None: raise AssertionError((root / 'err').read_text())
                    time.sleep(.1)
                def request(source):
                    req = urllib.request.Request(f'http://127.0.0.1:{port}/sources/{source}/codex/responses',
                        data=b'{"model":"shared-model","input":"fixture"}', headers={'Content-Type': 'application/json'})
                    try:
                        with urllib.request.urlopen(req, timeout=15) as result: return result.status
                    except urllib.error.HTTPError as error: return error.code
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                    statuses = list(pool.map(request, ids[:3] * 8))
                if statuses != [200] * 24:
                    failure = REPO / 'artifacts/proxy-fixture-failure'
                    failure.mkdir(parents=True, exist_ok=True)
                    shutil.copy2(root / 'out', failure / 'out.txt'); shutil.copy2(root / 'err', failure / 'err.txt')
                    for log in (runtime / 'log').glob('*.log'): shutil.copy2(log, failure / log.name)
                assert statuses == [200] * 24, statuses
                assert request('azure-failed') >= 400, 'Broken proxy silently fell back'
                for label, path, key, data in observed:
                    source = next(x for x in ids if x in path)
                    assert label == {'azure-a': 'proxy-a', 'azure-b': 'proxy-b', 'azure-direct': 'upstream'}[source], (label, path)
                    assert key == source + '-fixture-key'
                assert len(observed) == 24
                print('PASS: 24 concurrent same-model requests isolated across two proxies and direct; broken proxy failed closed; credentials source-bound')
            finally:
                if process.poll() is None: process.send_signal(signal.SIGINT)
                try: process.wait(timeout=10)
                except subprocess.TimeoutExpired: process.kill(); process.wait()
finally:
    for s in servers: s.shutdown(); s.server_close()
