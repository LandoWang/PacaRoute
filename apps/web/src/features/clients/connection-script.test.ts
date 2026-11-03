import { execFileSync, spawnSync } from "node:child_process";
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
const hasPwsh =
  spawnSync("pwsh", ["-NoProfile", "-NonInteractive", "-Command", "exit 0"])
    .status === 0;
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
  it("doubles every PowerShell single-quote-like character", () => {
    const text = connectionScript(
      client,
      "codex",
      "https://host/a\u2018b\u2019c\u201ad\u201be",
      "coding",
      "powershell",
      "k\u2018\u2019\u201a\u201b'",
    );
    expect(text).toContain(
      "'https://host/a\u2018\u2018b\u2019\u2019c\u201a\u201ad\u201b\u201be'",
    );
    expect(text).toContain(
      "$pacaKey = 'k\u2018\u2018\u2019\u2019\u201a\u201a\u201b\u201b'''",
    );
  });
  it.skipIf(!hasPwsh)(
    "passes PowerShell quote-like characters and metacharacters literally",
    () => {
      const dir = mkdtempSync(join(tmpdir(), "paca-ps-script-"));
      try {
        const capture = join(dir, "capture");
        writeFileSync(
          join(dir, "pacaroute-cli"),
          `#!/bin/bash\nprintf '%s\\n' "$@" > "$CAPTURE"\ncat >> "$CAPTURE"\n`,
          { mode: 0o700 },
        );
        const url =
          "https://host/p\u2019;New-Item url-marker;echo \u2018\u201a\u201b$(New-Item url-sub)`x";
        const key =
          "synthetic\u2019;New-Item key-marker;\u2019\u2018\u201a\u201b$(New-Item key-sub)`$HOME";
        const script = connectionScript(
          client,
          "codex",
          url,
          "coding",
          "powershell",
          key,
        );
        const file = join(dir, "connect.ps1");
        writeFileSync(file, script);
        execFileSync("pwsh", ["-NoProfile", "-NonInteractive", "-File", file], {
          input: "",
          cwd: dir,
          env: {
            ...process.env,
            PATH: dir + ":" + process.env.PATH,
            CAPTURE: capture,
          },
        });
        const captured = readFileSync(capture, "utf8");
        expect(captured).toContain("--url\n" + url + "\n");
        expect(captured).toContain("--apply\n" + key);
        for (const marker of ["url-marker", "url-sub", "key-marker", "key-sub"])
          expect(() => readFileSync(join(dir, marker))).toThrow();
      } finally {
        rmSync(dir, { recursive: true, force: true });
      }
    },
  );
  it("keeps router onboarding separate from native Agent scripts", () => {
    expect(() => connectionScript({ ...client, enabled_agents: ["pacaroute"] }, "pacaroute", "https://host", "coding", "shell", "synthetic-key")).toThrow("接入二维码");
  });

});
