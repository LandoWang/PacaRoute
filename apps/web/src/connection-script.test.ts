import { describe, expect, it } from "vitest";
import { execFileSync } from "node:child_process";
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { connectionScript } from "./connection-script";
import { type Client } from "./api";
const client: Client = {
  id: "alice",
  name: "Alice",
  disabled: false,
  enabled_agents: ["codex"],
  routes: [],
};
describe("Client connection scripts", () => {
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
    "passes shell metacharacters literally and reads the key only at execution",
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
        const script = connectionScript(
          client,
          "codex",
          url,
          "coding",
          "shell",
        );
        expect(script).not.toContain("example-secret-key");
        execFileSync("bash", ["-c", script], {
          input: "example-secret-key\n",
          cwd: dir,
          env: {
            ...process.env,
            PATH: dir + ":" + process.env.PATH,
            CAPTURE: capture,
          },
        });
        expect(readFileSync(capture, "utf8")).toContain(url + "\n");
        expect(readFileSync(capture, "utf8")).toContain(
          "--apply\nexample-secret-key",
        );
        expect(() => readFileSync(join(dir, "never-created"))).toThrow();
      } finally {
        rmSync(dir, { recursive: true, force: true });
      }
    },
  );
  it("uses PowerShell secure input and quoted arguments", () => {
    const text = connectionScript(
      client,
      "codex",
      "https://host/one'two",
      "coding",
      "powershell",
    );
    expect(text).toContain("Read-Host 'Client API key' -AsSecureString");
    expect(text).toContain("'https://host/one''two'");
    expect(text).toContain("ZeroFreeBSTR");
  });
});
