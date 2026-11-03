// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { type Config, type Status, type Usage } from "../../api/types";
import { StateContext, type AppState } from "../../app/state";
import { UsagePage } from "./usage-page";

const totals = {
  requests: 3,
  input_tokens: 1200,
  output_tokens: 300,
  cached_tokens: 100,
  failures: 0,
  missing_usage: 0,
};
const usage: Usage = {
  total: totals,
  sources: { s1: totals },
  source_models: { s1: { "gpt-x": totals } },
  timeline: {},
};
const config = {
  revision: "1",
  accounts: [],
  sources: [{ id: "s1", name: "Source One", deployments: [] }],
  pools: [],
  clients: [],
  proxies: { profiles: [], bindings: {}, account_proxy_id: "" },
  source_state: { paused: [] },
} as unknown as Config;
function view(status: Status) {
  const state = {
    config,
    status,
    navigate: () => {},
  } as unknown as AppState;
  return (
    <StateContext.Provider value={state}>
      <UsagePage />
    </StateContext.Provider>
  );
}
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("usage page rendering", () => {
  it("does not rebuild the hourly chart or table when only service status changes", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json(usage)),
    );
    const { container, rerender } = render(
      view({ uptime_seconds: 1 } as Status),
    );
    await screen.findByText("Source One");
    expect(container.querySelectorAll(".usage-chart rect")).toHaveLength(48);
    expect(screen.getByText("gpt-x")).toBeTruthy();
    const labels = vi.spyOn(Date.prototype, "toLocaleString");
    rerender(view({ uptime_seconds: 6 } as Status));
    expect(labels).not.toHaveBeenCalled();
    expect(container.querySelectorAll(".usage-chart rect")).toHaveLength(48);
  });
});
