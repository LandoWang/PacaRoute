import { execFileSync } from "node:child_process";
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Client } from "../../api/types";
import { connectionScript } from "./connection-script";
const client: Client = {
  id: "alice",
  name: "Alice",
  disabled: false,
  enabled_agents: ["codex"],
  routes: [],
};
describe("客户端 connection scripts", () => {
  it("rejects disabled Agents, unsafe URLs and remote local Clients", () => {
    expect(() =>
      connectionScript(
        client,
        "claude-code",
        "https://host",
        "coding",
        "shell",
      ),
    ).toThrow();
    for (const url of [
      "file:///x",
      "https://key@host",
      "https://host/?key=secret",
    ]) {
      expect(() =>
        connectionScript(client, "codex", url, "coding", "shell"),
      ).toThrow();
    }
    expect(() =>
      connectionScript(
        { ...client, local: true },
        "codex",
        "https://host",
        "coding",
        "shell",
      ),
    ).toThrow();
  });
  it.skipIf(process.platform === "win32")(
    "passes shell metacharacters literally and embeds the key without prompting or passing it in argv",
    () => {
      const dir = mkdtempSync(join(tmpdir(), "paca-script-"));
      try {
        const capture = join(dir, "capture");
        writeFileSync(
          join(dir, "pacaroute"),
          `#!/bin/bash\nprintf '%s\\n' "$@" > "$CAPTURE"\ncat >> "$CAPTURE"\n`,
          { mode: 0o700 },
        );
        const url = "https://host/path'$(touch never-created)";
        const key = "synthetic'$(touch key-executed)`touch key-executed`$HOME";
        const script = connectionScript(
          client,
          "codex",
          url,
          "coding",
          "shell",
          key,
        );
        expect(script).not.toContain("read -r");
        execFileSync("bash", ["-c", script], {
          input: "",
          cwd: dir,
          env: {
            ...process.env,
            PATH: dir + ":" + process.env.PATH,
            CAPTURE: capture,
          },
        });
        expect(readFileSync(capture, "utf8")).toContain(url + "\n");
        expect(readFileSync(capture, "utf8")).toContain("--apply\n" + key);
        expect(() => readFileSync(join(dir, "never-created"))).toThrow();
        expect(() => readFileSync(join(dir, "key-executed"))).toThrow();
      } finally {
        rmSync(dir, { recursive: true, force: true });
      }
    },
  );
  it("requires a remote key but keeps the local default keyless", () => {
    expect(() =>
      connectionScript(client, "codex", "https://host", "coding", "shell"),
    ).toThrow("Key");
    for (const platform of ["shell", "powershell"] as const) {
      const script = connectionScript(
        { ...client, id: "default", local: true },
        "codex",
        "http://localhost:8766",
        "coding",
        platform,
        "ignored-key",
      );
      expect(script).toContain("--local --apply");
      expect(script).not.toContain("ignored-key");
    }
  });
  it("embeds a literal PowerShell key and quotes arguments", () => {
    const text = connectionScript(
      client,
      "codex",
      "https://host/one'two",
      "coding",
      "powershell",
      "synthetic'$(danger)`value",
    );
    expect(text).not.toContain("Read-Host");
    expect(text).toContain("$pacaKey = 'synthetic''$(danger)`value'");
    expect(text).toContain("$pacaKey | & $pacaBinary");
    expect(text).toContain("'https://host/one''two'");
    expect(text).toContain("Remove-Variable pacaKey");
  });
  it("keeps router onboarding separate from native Agent scripts", () => {
    expect(() => connectionScript({ ...client, enabled_agents: ["pacaroute"] }, "pacaroute", "https://host", "coding", "shell", "synthetic-key")).toThrow("接入二维码");
  });

});
