"""Exercise portable import before real clients; never copy upstream credentials."""
import json
import urllib.request


def imported_pool(home, port, client_id):
    admin = (home / "admin.key").read_text().strip()

    def api(path, data=None, method=None):
        body = None if data is None else json.dumps(data).encode()
        req = urllib.request.Request(
            f"http://127.0.0.1:{port}/api/{path}", data=body,
            method=method or ("GET" if body is None else "POST"),
            headers={"Authorization": "Bearer " + admin, "Content-Type": "application/json"})
        with urllib.request.urlopen(req, timeout=30) as response:
            return json.load(response)

    original = api("config")
    bundle = api("config/export")
    assert all(not c.get("key_hash") for c in bundle["config"]["clients"])
    request = {"bundle": bundle, "mode": "replace", "revision": original["revision"]}
    plan = api("config/import", request)
    api("config/import", dict(request, apply=True, namespace=plan["namespace"], fingerprint=plan["fingerprint"]))
    current = api("config")
    mapping = {(c["kind"], c["from_id"]): c["to_id"] for c in plan["changes"]}
    # Portable imports intentionally have no upstream credentials. For live QA,
    # reconnect their sources to the pre-existing Auth authority, like choosing
    # an already authorized account after import. Credential files/refresh tokens
    # remain in place, and Auth retains their single original refresh lock.
    for source in original["sources"]:
        imported = next(s for s in current["sources"] if s["id"] == mapping[("sources", source["id"])])
        account = next(a for a in original["accounts"] if a["id"] == source["account_id"])
        if not any(a["id"] == account["id"] for a in current["accounts"]):
            current["accounts"].append(account)
        imported["account_id"] = account["id"]
    api("config", current, "PUT")
    current = api("config")
    client = next(c for c in current["clients"] if c["id"] == mapping[("clients", client_id)])
    key = api("clients/" + client["id"] + "/key")["api_key"]
    assert key and key != "qa-client-synthetic-key"
    (home / "import-plan.json").write_text(json.dumps(plan, indent=2))
    return client, current["pools"], key
