"""Exercise Client creation, authenticated key recovery and the actual web script."""
import json, os, pathlib, subprocess, urllib.request

def configure(binary, root, port, client, agent, model, env):
    admin = (root / 'admin.key').read_text().strip()
    def api(path, method='GET', value=None):
        request = urllib.request.Request(f'http://127.0.0.1:{port}/api/{path}',
            data=json.dumps(value).encode() if value is not None else None, method=method,
            headers={'Authorization': 'Bearer ' + admin, 'Content-Type': 'application/json'})
        with urllib.request.urlopen(request) as response:
            assert response.headers['Cache-Control'] == 'no-store'
            return json.load(response)
    created = api('clients', 'POST', {k:v for k,v in client.items() if k != 'key_hash'})
    key = api(f"clients/{client['id']}/key")['api_key']
    assert key and key == created['api_key']
    renderer = subprocess.run(['node', 'scripts/verify-connection-script.mjs'],
        input=json.dumps({'client': client, 'agent': agent, 'url': f'http://127.0.0.1:{port}', 'model': model, 'key': key}).encode(),
        capture_output=True, env=env)
    assert renderer.returncode == 0, 'Connection script renderer failed'
    bindir = root / 'script-bin'; bindir.mkdir()
    (bindir / 'pacaroute').symlink_to(binary)
    result = subprocess.run(['bash'], input=renderer.stdout, capture_output=True,
        env=dict(env, PATH=str(bindir)+os.pathsep+env['PATH']))
    assert result.returncode == 0, 'Generated connection script failed'
    return result
