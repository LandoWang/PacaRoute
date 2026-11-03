import { describe, expect, it } from "vitest";
import { APIError, type api } from "../api/transport";
import { type Config } from "../api/types";
import { RevisionConflictError, saveConfig } from "./config-save";

const draft = {
  revision: "1",
  clients: [{ id: "edited" }],
} as unknown as Config;
const latest = {
  revision: "2",
  clients: [{ id: "other" }],
} as unknown as Config;

function server(put: () => Promise<unknown>, current: Config) {
  const calls: string[] = [];
  const request = (async (path: string, method = "GET") => {
    calls.push(method + " " + path);
    return method === "PUT" ? put() : current;
  }) as typeof api;
  return { calls, request };
}

describe("configuration save revision handling", () => {
  it("reports a stale revision as a conflict, shows the newer snapshot and never retries the draft", async () => {
    const { calls, request } = server(
      () => Promise.reject(new APIError(409, "configuration changed")),
      latest,
    );
    const shown: Config[] = [];
    await expect(
      saveConfig(draft, (c) => shown.push(c), request),
    ).rejects.toBeInstanceOf(RevisionConflictError);
    expect(shown).toEqual([latest]);
    expect(calls.filter((c) => c.startsWith("PUT"))).toHaveLength(1);
  });

  it("keeps validation failures on the current revision as the server's error", async () => {
    const failure = new APIError(
      409,
      "account is still referenced by a source",
    );
    const { request } = server(() => Promise.reject(failure), draft);
    const shown: Config[] = [];
    await expect(saveConfig(draft, (c) => shown.push(c), request)).rejects.toBe(
      failure,
    );
    expect(shown).toEqual([]);
  });

  it("returns the saved snapshot", async () => {
    const { request } = server(() => Promise.resolve(latest), draft);
    await expect(saveConfig(draft, () => {}, request)).resolves.toBe(latest);
  });
});
