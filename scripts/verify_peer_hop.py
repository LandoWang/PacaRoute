"""Isolated extra PacaRoute hop; upstream Auth credentials remain in place."""
import hashlib
import json
import pathlib
import signal
import socket
import subprocess
import time
import urllib.request
import urllib.error
import uuid


def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    assert port != 8765
    return port


class PeerHop:
    def __init__(self, binary, directory, upstream_port, upstream_key, agent, model, env):
        self.root = pathlib.Path(directory) / 'peer'
        self.root.mkdir()
        self.binary = str(binary)
        self.port, self.admin_port = free_port(), free_port()
        while self.admin_port == self.port:
            self.admin_port = free_port()
        self.key = 'synthetic-peer-client-key'
        self.source_id = 'qa-peer-' + uuid.uuid4().hex[:12]
        self.account_id = 'qa-remote-' + uuid.uuid4().hex[:12]
        origin = f'http://127.0.0.1:{upstream_port}'
        admin = (pathlib.Path(directory) / 'admin.key').read_text().strip()
        headers = {'Authorization': 'Bearer ' + admin, 'Content-Type': 'application/json'}
        with urllib.request.urlopen(urllib.request.Request(origin + '/api/config', headers=headers)) as response:
            config = json.load(response)
        client = next(c for c in config['clients'] if c.get('key_hash') == hashlib.sha256(upstream_key.encode()).hexdigest())
        client['enabled_agents'] = list(dict.fromkeys(client['enabled_agents'] + ['pacaroute']))
        client['routes'] += [dict(route, id='peer-' + route['id'], agent='pacaroute') for route in list(client['routes']) if route['agent'] == agent]
        request = urllib.request.Request(origin + '/api/config', data=json.dumps(config).encode(), method='PUT', headers=headers)
        with urllib.request.urlopen(request) as response:
            assert response.status == 200
        endpoint = origin + '/pacaroute'
        # Assert discovery sees only the granted Client model, using its key.
        request = urllib.request.Request(endpoint + '/v1/models', headers={'Authorization': 'Bearer ' + upstream_key})
        with urllib.request.urlopen(request, timeout=15) as response:
            models = json.load(response)['data']
        assert any(m['id'] == model and agent in m['supported_agents'] for m in models)
        values = {
            'accounts': [{'id': self.account_id, 'name': 'Remote QA', 'kind': 'pacaroute', 'endpoint': endpoint}],
            'sources': [{'id': self.source_id, 'name': 'Peer QA', 'kind': 'pacaroute', 'account_id': self.account_id, 'endpoint': endpoint, 'auth_mode': 'api-key', 'deployments': [model], 'agent_models': {agent: [model]}}],
            'clients': [{'id': 'qa-peer', 'name': 'QA Peer', 'enabled_agents': [agent], 'key_hash': hashlib.sha256(self.key.encode()).hexdigest(), 'routes': [{'id': 'main', 'agent': agent, 'match_model': model, 'model': model, 'source_id': self.source_id}]}],
        }
        for name, value in values.items():
            (self.root / (name + '.json')).write_text(json.dumps(value))
        log = (self.root / 'bridge.log').open('wb')
        self.process = subprocess.Popen([self.binary, 'serve', '--home', str(self.root), '--credential-root', str(self.root / 'credentials'), '--port', str(self.port), '--admin-port', str(self.admin_port), '--no-serve', '--trace-dir', str(self.root / 'traces')], env=dict(env, COPILOT_BRIDGE_SECRETS_STDIN='1'), stdin=subprocess.PIPE, stdout=log, stderr=subprocess.STDOUT)
        log.close()
        try:
            # Use Auth's existing stdin lease authority for this static QA Client
            # key. Do not write QA peer credentials into the user's Keychain.
            self.process.stdin.write((json.dumps({self.source_id: upstream_key}) + '\n').encode())
            self.process.stdin.flush()
            for _ in range(200):
                if self.process.poll() is not None:
                    raise RuntimeError('Peer QA startup failed')
                try:
                    with socket.create_connection(('127.0.0.1', self.admin_port), timeout=.3):
                        break
                except OSError:
                    time.sleep(.1)
            else:
                raise TimeoutError('Peer QA management')
            self.admin = (self.root / 'admin.key').read_text().strip()
            self.cycle()
        except BaseException:
            self.close()
            raise

    def api(self, path, data=None):
        body = json.dumps(data).encode() if data is not None else None
        request = urllib.request.Request(f'http://127.0.0.1:{self.admin_port}/api/' + path, data=body, method='PUT' if data is not None else 'GET', headers={'Authorization': 'Bearer ' + self.admin, 'Content-Type': 'application/json'})
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            detail = json.load(error).get("error", {}).get("message", "request failed")
            raise RuntimeError(f"Peer management {path}: {detail}") from None

    def cycle(self):
        assert not self.api('service', {'serving': False})['serving']
        assert any(a['id'] == self.account_id and a['kind'] == 'pacaroute' for a in self.api('config')['accounts'])
        assert self.api('service', {'serving': True})['serving']

    def close(self):
        if self.process.poll() is None:
            command = subprocess.check_output(['ps', '-p', str(self.process.pid), '-o', 'command=']).decode()
            assert self.binary in command and str(self.root) in command
            self.process.send_signal(signal.SIGINT)
            try:
                self.process.wait(timeout=35)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()
