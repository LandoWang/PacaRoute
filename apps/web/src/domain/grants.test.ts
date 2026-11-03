import { describe, expect, it } from "vitest";
import { type Client } from "../api/types";
import { enabledAgents, isProtocolAgent } from "./routing";

const client = { id: "c", name: "C", disabled: false, routes: [] } as Client;
describe("Client grants", () => {
  it("treats omitted or null enabled_agents as the legacy default and [] as none", () => {
    expect(enabledAgents(client)).toEqual(["codex", "claude-code"]);
    expect(enabledAgents({ ...client, enabled_agents: null })).toEqual([
      "codex",
      "claude-code",
    ]);
    expect(enabledAgents({ ...client, enabled_agents: [] })).toEqual([]);
  });
  it("separates native protocols from the PacaRoute grant", () => {
    expect(isProtocolAgent("codex")).toBe(true);
    expect(isProtocolAgent("claude-code")).toBe(true);
    expect(isProtocolAgent("pacaroute")).toBe(false);
  });
});
