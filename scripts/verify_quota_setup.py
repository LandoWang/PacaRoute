"""Synthetic prices and generous quotas for isolated real-client settlement QA."""
import json
import urllib.request


def configure_billing(home, port, model):
    key = (home / 'admin.key').read_text().strip()
    endpoint = f'http://127.0.0.1:{port}/api/config'
    headers = {'Authorization': 'Bearer ' + key, 'Content-Type': 'application/json'}
    with urllib.request.urlopen(urllib.request.Request(endpoint, headers=headers)) as response:
        config = json.load(response)
    for source in config['sources']:
        source['prices'] = {model: {'input_usd': '1', 'cached_usd': '1', 'cache_write_usd': '1', 'output_usd': '1'}}
    for client in config['clients']:
        client['quota'] = {'window_seconds': 3600, 'requests': 1000, 'input_tokens': 1000000000, 'cost_usd': '100'}
    with urllib.request.urlopen(urllib.request.Request(endpoint, data=json.dumps(config).encode(), method='PUT', headers=headers)) as response:
        assert response.status == 200
