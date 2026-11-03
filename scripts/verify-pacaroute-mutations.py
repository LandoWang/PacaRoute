#!/usr/bin/env python3
"""Confirm critical regressions are caught, without changing the working tree."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
mutations = [
    ("client-quota-never-blocks", "quota.go",
     'state.Blocked != ""', 'false',
     "TestQuotaConcurrentAdmissionRestartAndIdempotentSettlement"),
    ("settlement-bills-public-alias", "server.go",
     'source.Prices[activity.Model]', 'source.Prices["coding"]',
     "TestGatewayClientQuotaUsesActualTargetAndConfirmedTotals"),
    ("peer-agent-inherits-native-grants", "peer.go",
     'route.Agent == "pacaroute" && c.validateTarget', 'c.validateTarget',
     "TestPacaRouteAgentGrantAndDisabledPaths"),
    ("legacy-local-sentinel-rejected", "server.go",
     ' || key == "copilot-bridge-provider"', '',
     "TestLegacyProviderSentinelLocalMigration"),
    ("peer-catalog-ignores-client-key", "peer.go",
     'client, ok := s.authenticate(r, c)', 'client, ok := c.Client("alice")',
     "TestPeerCatalogClientScopeAndCapability"),
    ("peer-session-loses-client-scope", "server.go",
     'affinityScope+"\\x00"+agent', '"shared"+"\\x00"+agent',
     "TestPeerNativeProtocolsAndSessionIsolation"),
    ("management-port-admits-inference", "desktop_control.go",
     'if managementPath(r.URL.Path) != management {', 'if false {',
     "TestIndependentManagementAndDrainingService"),

    ("import-reuses-account-authority", "config_transfer.go",
     'source.AccountID = accountMap[source.AccountID]', '',
     "TestConfigTransferExportAndAddIsolation"),
    ("import-reuses-source-identities", "config_transfer.go",
     'source.ID, source.Name = allocate("sources", old, source.Name, false)',
     '_, source.Name = allocate("sources", old, source.Name, false)',
     "TestConfigTransferExportAndAddIsolation"),
    ("pool-crosses-model-groups", "routing.go",
     'm.Weight == 0 || p.MemberModel(m) != model', 'm.Weight == 0',
     "TestMultiModelPoolThrottleWeightsAndStickyScope"),
    ("throttle-loses-model-scope", "routing.go",
     'key := t.Source.ID + ":" + t.Model', 'key := t.Source.ID',
     "TestMultiModelPoolThrottleWeightsAndStickyScope"),
    ("terminal-frame-waits-for-eof", "server.go",
     'if typ == "response.completed" || typ == "response.incomplete" || typ == "message_stop" {\n\t\t\treturn nil\n\t\t}',
     'if false {\n\t\t\treturn nil\n\t\t}',
     "TestTerminalFrameDoesNotWaitForTransportEOF"),
    ("stream-failure-keeps-sticky-source", "server.go",
     '\t\t\t\t\ts.Router.Fail(t, activity.Error)', '',
     "TestStreamFailureRecoversOnNextClientRetry"),
    ("azure-error-hides-terminal", "server.go",
     'pending = streamEventError(m, false)', 'return streamEventError(m, true)',
     "TestAzureErrorDoesNotHideFollowingFailureTerminal"),
    ("changing-stream-ids", "protocol.go",
     'key := "output:" + string(m["output_index"])',
     'key := rawString(m["item_id"])',
     "TestCrossProtocolChangingOpaqueIDsKeepOneTextBlock"),
    ("paused-source-admission", "routing.go",
     'if c.Paused(id) {', 'if false {',
     "TestClientAuthorizationRouteAndHotPause"),
    ("tool-image-loss", "protocol.go",
     'return jsonBytes(result), nil', 'return jsonBytes(textContent(b)), nil',
     "TestCrossProtocolToolImagesRoundTrip"),
]
results = []
with tempfile.TemporaryDirectory(prefix="pacaroute-mutations-") as temporary:
    target = Path(temporary)
    for name in ("go.mod", "go.sum"):
        shutil.copy2(root / name, target / name)
    shutil.copytree(root / "internal", target / "internal")
    for name, filename, before, after, test in mutations:
        path = target / "internal/pacaroute" / filename
        original = path.read_text()
        assert original.count(before) == 1, name
        path.write_text(original.replace(before, after))
        run = subprocess.run([os.environ.get("PACAROUTE_GO", "go"), "test",
                              "./internal/pacaroute", "-run", "^" + test + "$", "-count=1"],
                             cwd=target, capture_output=True, text=True, timeout=120)
        output = run.stdout + run.stderr
        killed = run.returncode != 0 and ("--- FAIL: " + test) in output
        results.append({"mutation": name, "test": test, "caught": killed})
        path.write_text(original)
report = root / "artifacts/pacaroute-verification/mutation-verdict.json"
report.parent.mkdir(parents=True, exist_ok=True)
report.write_text(json.dumps(results, indent=2) + "\n")
print(json.dumps(results, indent=2))
assert all(result["caught"] for result in results), "A regression survived or did not reach its assertion"
