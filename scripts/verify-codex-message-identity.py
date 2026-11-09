#!/usr/bin/env python3
"""Check real Codex message lifecycle identity using this run's notifications."""
import argparse
import json
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("manifest")
parser.add_argument("--output", required=True)
args = parser.parse_args()
manifest = json.loads(Path(args.manifest).read_text())
events = [json.loads(line) for line in Path(manifest["stdoutPath"]).read_text().splitlines() if line.strip()]
started, completed, deltas = {}, {}, set()
duplicates = 0
finals = []
for event in events:
    method = event.get("method")
    params = event.get("params", {})
    item = params.get("item", {})
    if method in ("item/started", "item/completed") and item.get("type") == "agentMessage":
        key = (params.get("turnId"), item.get("id"))
        target = started if method == "item/started" else completed
        duplicates += key in target
        target[key] = item
        if method == "item/completed" and item.get("phase") == "final_answer":
            finals.append(item.get("text", ""))
    if method == "item/agentMessage/delta":
        deltas.add((params.get("turnId"), params.get("itemId")))

result = {
    "manifest": str(Path(args.manifest).resolve()),
    "started_messages": len(started),
    "completed_messages": len(completed),
    "completions_without_start": len(completed.keys() - started.keys()),
    "starts_without_completion": len(started.keys() - completed.keys()),
    "deltas_without_start": len(deltas - started.keys()),
    "duplicate_message_events": duplicates,
    "final_messages": len(finals),
    "first_canary_count": sum(text.count("PACAROUTE_APPSERVER_OK") for text in finals),
    "continuation_canary_count": sum(text.count("PACAROUTE_APPSERVER_RESUME_OK") for text in finals),
}
passed = (bool(started) and bool(deltas) and started.keys() == completed.keys()
          and deltas <= started.keys() and duplicates == 0 and len(finals) == 2
          and result["first_canary_count"] == result["continuation_canary_count"] == 1)
result["verdict"] = "PASS" if passed else "FAIL"
Path(args.output).write_text(json.dumps(result, indent=2) + "\n")
print(json.dumps(result, indent=2))
raise SystemExit(0 if passed else 1)
