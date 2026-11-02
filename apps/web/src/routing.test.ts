import { describe, it, expect } from "vitest";
import { routeTarget, routeAgent, share, type Config, type Route } from "./api";
const config: Config = {
  revision: "1",
  accounts: [],
  sources: [
    {
      id: "azure",
      name: "Azure",
      kind: "azure-openai",
      auth_mode: "azure-cli",
      account_id: "azure-local",
      endpoint: "https://example.openai.azure.com",
      deployments: ["deployment-one"],
    },
  ],
  pools: [
    {
      id: "claude-pool",
      name: "Claude",
      agent: "claude-code",
      model: "balanced",
      failover: true,
      members: [],
    },
  ],
  clients: [],
  proxies: { profiles: [], bindings: {}, account_proxy_id: "system" },
  source_state: { paused: [] },
};
const route: Route = {
  id: "a",
  agent: "claude-code",
  match_model: "*",
  source_id: "copilot",
  model: "old",
};
describe("visual route editing contracts", () => {
  it("selecting a pool uses its model alias instead of a previous concrete model", () => {
    expect(routeTarget(route, "pool:claude-pool", config).model).toBe(
      "balanced",
    );
    expect(route.model).toBe("old");
  });
  it("changing Agent cannot retain an incompatible pool", () => {
    const updated = routeAgent(
      { ...route, source_id: "pool:claude-pool", model: "balanced" },
      "codex",
      config,
    );
    expect(updated).toMatchObject({
      agent: "codex",
      source_id: "copilot",
      model: "",
    });
  });
  it("selecting an Azure source chooses its deployment, not an unrelated model", () =>
    expect(routeTarget(route, "azure", config).model).toBe("deployment-one"));
  it("zero weight gets no new sessions and proportions are relative", () => {
    const members = [
      { id: "a", source_id: "a", model: "m", weight: 7 },
      { id: "b", source_id: "b", model: "m", weight: 3 },
      { id: "c", source_id: "c", model: "m", weight: 0 },
    ];
    expect(members.map((m) => share(m.weight, members))).toEqual([70, 30, 0]);
    expect(share(0, [members[2]])).toBe(0);
  });
});

import { compatibleAccount, resourceKind, defaultEndpoint } from "./api";
describe("account and source provider separation", () => {
  it("maps four account providers to their resource products", () =>
    expect(
      ["github", "openai", "anthropic", "azure"].map(resourceKind),
    ).toEqual(["copilot", "openai", "anthropic", "azure-openai"]));
  it("prevents assigning an unrelated provider account", () => {
    expect(compatibleAccount("openai", "anthropic")).toBe(false);
    expect(compatibleAccount("copilot", "github")).toBe(true);
    expect(compatibleAccount("azure-openai", "azure")).toBe(true);
    expect(compatibleAccount("openai", "api-key")).toBe(true);
  });
  it("uses the official default API endpoints", () => {
    expect(defaultEndpoint("openai")).toBe("https://api.openai.com/v1");
    expect(defaultEndpoint("anthropic")).toBe("https://api.anthropic.com");
  });
});
