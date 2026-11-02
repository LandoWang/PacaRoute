#!/usr/bin/env python3
"""Inspect only explicit real-client manifests; never infer success from HTTP 200."""
import argparse
import hashlib
import json
from pathlib import Path
import sqlite3

parser = argparse.ArgumentParser()
parser.add_argument("manifests", nargs="+")
parser.add_argument("--output", required=True)
args = parser.parse_args()


def objects(value):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from objects(child)
    elif isinstance(value, list):
        for child in value:
            yield from objects(child)


results = []
for argument in args.manifests:
    path = Path(argument).resolve()
    manifest = json.loads(path.read_text())
    events = [json.loads(line) for line in Path(manifest["stdoutPath"]).read_text().splitlines() if line.strip()]
    calls, outputs = set(), set()
    for trace in (path.parent / "traces").rglob("upstream-res.sse"):
        for line in trace.read_text().splitlines():
            if not line.startswith("data:"):
                continue
            try:
                data = json.loads(line[5:].strip())
            except ValueError:
                continue
            for item in objects(data):
                if item.get("type") in ("function_call", "custom_tool_call"):
                    calls.add(item["call_id"])
                if item.get("type") == "tool_use":
                    calls.add(item["id"])
    for trace in (path.parent / "traces").rglob("client-req.json"):
        for item in objects(json.loads(trace.read_text())):
            if item.get("type") in ("function_call_output", "custom_tool_call_output"):
                outputs.add(item["call_id"])
            if item.get("type") == "tool_result":
                outputs.add(item["tool_use_id"])
    checks = {"wire_tool_roundtrips": len(calls & outputs)}
    passed = checks["wire_tool_roundtrips"] >= 3
    if "dispatchLogPath" in manifest:
        with sqlite3.connect("file:" + manifest["dispatchLogPath"] + "?mode=ro", uri=True) as database:
            rows = database.execute("SELECT level,target,feedback_log_body FROM logs WHERE ts BETWEEN ? AND ?",
                                    (manifest["start"], manifest["end"])).fetchall()
        fatals = [row for row in rows if
                  ("tools::router" in row[1] and row[0].upper() in ("ERROR", "FATAL")) or
                  any(term in (row[2] or "").lower() for term in
                      ("incompatible payload", "missing namespace", "polymorphism_"))]
        turns = [event["params"]["turn"]["status"] for event in events if event.get("method") == "turn/completed"]
        checks.update(dispatch_rows=len(rows), router_fatals=len(fatals), turn_statuses=turns)
        passed &= bool(rows) and not fatals and len(turns) == 2 and all(turn == "completed" for turn in turns)
    else:
        transcript_calls, transcript_outputs = set(), set()
        for event in events:
            for item in objects(event.get("message", {})):
                if item.get("type") == "tool_use":
                    transcript_calls.add(item["id"])
                if item.get("type") == "tool_result" and not item.get("is_error"):
                    transcript_outputs.add(item["tool_use_id"])
        final = next((event for event in reversed(events) if event.get("type") == "result"), {})
        complete = final.get("is_error") is False and "PACAROUTE_CLAUDE_OK" in final.get("result", "")
        upstream_models = sorted({json.loads(trace.read_text()).get("model", "")
                                  for trace in (path.parent / "traces").rglob("upstream-*-req.json")})
        expected_model = upstream_models == [manifest["upstream_model"]]
        checks.update(transcript_tool_roundtrips=len(transcript_calls & transcript_outputs),
                      complete_answer=complete, upstream_models=upstream_models,
                      expected_model=expected_model)
        passed &= len(transcript_calls & transcript_outputs) >= 3 and complete and expected_model
    current_hash = hashlib.sha256(Path(manifest["binaryPath"]).read_bytes()).hexdigest()
    checks["unchanged_binary"] = current_hash == manifest["binarySHA256"]
    passed &= checks["unchanged_binary"]
    results.append({"manifest": str(path), "binary_sha256": current_hash,
                    "verdict": "PASS" if passed else "FAIL", "checks": checks})
Path(args.output).write_text(json.dumps(results, indent=2) + "\n")
print(json.dumps(results, indent=2))
raise SystemExit(0 if all(result["verdict"] == "PASS" for result in results) else 1)
