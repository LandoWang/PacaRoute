import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { type ReactNode } from "react";
import {
  CooldownDetails,
  RoutingAlert,
  PoolAvailability,
  poolAllocation,
} from "./routing-health";
import { StateContext, type AppState } from "./state";
import { type Config, type RoutingHealth, share } from "./api";

const config: Config = {
  revision: "1",
  accounts: [],
  sources: [],
  clients: [],
  proxies: { profiles: [], bindings: {}, account_proxy_id: "system" },
  source_state: { paused: ["paused"] },
  pools: [
    {
      id: "astra",
      name: "Astra Pool",
      agent: "codex",
      model: "astra",
      failover: true,
      members: [
        { id: "bad", source_id: "azure", model: "astra", weight: 1 },
        { id: "good", source_id: "azure", model: "other", weight: 1 },
        { id: "paused", source_id: "paused", model: "astra", weight: 1 },
      ],
    },
  ],
};
const health: RoutingHealth = {
  observed_at: "2026-11-02T18:00:00Z",
  cooldowns: [
    {
      source_id: "azure",
      model: "astra",
      error: "upstream stream ended: EOF",
      failed_at: "2026-11-02T17:59:50Z",
      until: "2026-11-02T18:00:20Z",
    },
  ],
  pools: [{ id: "astra", available: 1, total: 3 }],
};
function render(children: ReactNode, routing = health) {
  return renderToStaticMarkup(
    <StateContext.Provider
      value={
        {
          config,
          status: { routing },
          navigate: () => {},
        } as unknown as AppState
      }
    >
      {children}
    </StateContext.Provider>,
  );
}
describe("routing health visibility", () => {
  it("renormalizes only the throttled model group and exposes partial model exhaustion", () => {
    const pool = {
      ...config.pools[0],
      multi_model: true,
      model: "astra",
      members: [
        { id: "a", source_id: "a", model: "astra", weight: 50 },
        { id: "b", source_id: "b", model: "astra", weight: 30 },
        { id: "c", source_id: "c", model: "astra", weight: 20 },
        { id: "sonnet", source_id: "c", model: "sonnet", weight: 1000 },
      ],
    };
    const state = {
      ...health,
      cooldowns: [{ ...health.cooldowns[0], source_id: "c", model: "astra" }],
    };
    const astra = poolAllocation(config, pool, state, "astra");
    expect(astra.map((m) => share(m.weight, astra))).toEqual([62.5, 37.5, 0]);
    expect(poolAllocation(config, pool, state, "sonnet")[0].weight).toBe(1000);
    const exhaustedModel = {
      ...state,
      pools: [
        {
          id: "astra",
          available: 1,
          total: 4,
          models: [
            { model: "astra", available: 0, total: 3 },
            { model: "sonnet", available: 1, total: 1 },
          ],
        },
      ],
    };
    expect(render(<RoutingAlert />, exhaustedModel)).toContain("critical");
    expect(render(<RoutingAlert />, exhaustedModel)).toContain(
      "Astra Pool · astra",
    );
  });
  it("shows cooldown reason, model and server-relative remaining time", () => {
    const html = render(<CooldownDetails source="azure" />);
    expect(html).toContain("20");
    expect(html).toContain("错误冷却");
    expect(html).toContain("astra");
    expect(html).toContain("upstream stream ended: EOF");
    expect(render(<CooldownDetails source="azure" model="other" />)).toBe("");
  });
  it("excludes only the failed model and paused source from displayed allocation", () => {
    expect(
      poolAllocation(config, config.pools[0], health).map((m) => m.weight),
    ).toEqual([0, 1, 0]);
    expect(
      poolAllocation(config, config.pools[0], { ...health, cooldowns: [] }).map(
        (m) => m.weight,
      ),
    ).toEqual([1, 1, 0]);
  });
  it("warns globally when degraded and alerts explicitly when a pool is exhausted", () => {
    expect(render(<RoutingAlert />)).toContain("来源异常，正在冷却");
    const exhausted = {
      ...health,
      pools: [{ id: "astra", available: 0, total: 3 }],
    };
    const html = render(
      <>
        <RoutingAlert />
        <PoolAvailability id="astra" />
      </>,
      exhausted,
    );
    expect(html).toContain("来源池无可用成员");
    expect(html).toContain("Astra Pool");
    expect(html).toContain("critical");
    expect(html).toContain("新会话可用 0 / 3");
    expect(render(<RoutingAlert />, { ...health, cooldowns: [] })).toBe("");
  });
});
