// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { type Config, type Status } from "../../api/types";
import { StateContext, type AppState } from "../../app/state";
import { Clients } from "./clients-page";

const status = { local_access: true, local_management: false } as Status;
function config(revision: string, aliceModel = "m1", bobName = "Bob"): Config {
  return {
    revision,
    accounts: [],
    sources: [
      {
        id: "s1",
        name: "Source",
        kind: "openai",
        account_id: "a",
        endpoint: "https://api.example",
        auth_mode: "api-key",
        deployments: ["m1", "m2"],
      },
    ],
    pools: [],
    proxies: { profiles: [], bindings: {}, account_proxy_id: "" },
    source_state: { paused: [] },
    clients: [
      {
        id: "alice",
        name: "Alice",
        disabled: false,
        enabled_agents: ["codex"],
        routes: [
          {
            id: "r1",
            agent: "codex",
            match_model: "*",
            source_id: "s1",
            model: aliceModel,
          },
        ],
      },
      {
        id: "bob",
        name: bobName,
        disabled: false,
        enabled_agents: ["codex"],
        routes: [],
      },
    ],
  };
}
function view(c: Config, save: AppState["save"] = async () => {}) {
  const state: AppState = {
    config: c,
    status,
    save,
    reload: async () => {},
    show: () => {},
    close: () => {},
    notice: () => {},
    navigate: () => {},
  };
  return (
    <StateContext.Provider value={state}>
      <Clients />
    </StateContext.Provider>
  );
}
const mode = () =>
  (screen.getByLabelText("模型选择方式") as HTMLSelectElement).value;

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) =>
      Response.json(
        url.includes("/quota")
          ? { window_seconds: 86400, requests: 0, input_tokens: 0 }
          : { models: [] },
      ),
    ),
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("client route card drafts", () => {
  it("keeps an unsaved selection when another part of the configuration is saved", () => {
    const { rerender } = render(view(config("1")));
    expect(mode()).toBe("fixed");
    fireEvent.change(screen.getByLabelText("模型选择方式"), {
      target: { value: "multiple" },
    });
    rerender(view(config("2", "m1", "Bob renamed")));
    expect(mode()).toBe("multiple");
    expect(screen.queryByText(/已在其他位置更改/)).toBeNull();
  });

  it("reports a conflict instead of discarding or overwriting an edited draft", () => {
    const { rerender } = render(view(config("1")));
    fireEvent.change(screen.getByLabelText("模型选择方式"), {
      target: { value: "multiple" },
    });
    rerender(view(config("2", "m2")));
    expect(mode()).toBe("multiple");
    expect(screen.getAllByText(/已在其他位置更改/).length).toBeGreaterThan(0);
    fireEvent.click(screen.getAllByText("保存路由")[0]);
    expect(screen.getByText(/请先载入最新路由/)).toBeTruthy();
    fireEvent.click(screen.getByText("放弃修改并载入最新路由"));
    expect(mode()).toBe("fixed");
    expect(screen.queryByText(/已在其他位置更改/)).toBeNull();
  });

  it("follows saved route changes while the draft is untouched", () => {
    const { rerender, container } = render(view(config("1")));
    rerender(view(config("2", "m2")));
    expect(screen.queryByText(/已在其他位置更改/)).toBeNull();
    expect(
      (container.querySelector("#alicecodex") as HTMLSelectElement).value,
    ).toBe("m2");
  });

  it("adopts its own saved routes without reporting a conflict", async () => {
    const saved: Config[] = [];
    const save = async (next: Config) => {
      saved.push({ ...next, revision: "2" });
    };
    const { rerender } = render(view(config("1"), save));
    fireEvent.change(screen.getByLabelText("模型选择方式"), {
      target: { value: "multiple" },
    });
    await act(async () => {
      fireEvent.click(screen.getAllByText("保存路由")[0]);
    });
    expect(saved).toHaveLength(1);
    rerender(view(saved[0], save));
    expect(screen.queryByText(/已在其他位置更改/)).toBeNull();
    expect(mode()).toBe("multiple");
  });
});
