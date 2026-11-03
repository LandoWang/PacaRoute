// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { type Config } from "../../api/types";
import { PoolEditor } from "../pools/pool-editor";
import { ProxyEditor } from "./proxy-editor";

const config = {
  revision: "1",
  accounts: [],
  sources: [],
  pools: [
    {
      id: "pool-a",
      name: "Existing pool",
      agent: "codex",
      model: "balanced",
      failover: true,
      members: [{ id: "m1", source_id: "s1", model: "x", weight: 1 }],
    },
  ],
  clients: [],
  proxies: {
    profiles: [{ id: "proxy-a", name: "Existing proxy", mode: "direct" }],
    bindings: {},
    account_proxy_id: "",
  },
  source_state: { paused: [] },
} as Config;

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function submitWithID(id: string) {
  fireEvent.change(screen.getByLabelText("内部 ID"), {
    target: { value: id },
  });
  fireEvent.submit(document.querySelector("form")!);
  await screen.findByRole("alert");
}

describe("adding pools and proxies", () => {
  it("does not replace an existing proxy that shares the new proxy's ID", async () => {
    const save = vi.fn(async () => {});
    render(<ProxyEditor config={config} save={save} close={() => {}} />);
    await submitWithID("proxy-a");
    expect(save).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toContain("内部 ID");
  });

  it("does not replace an existing pool that shares the new pool's ID", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({ models: [] })),
    );
    const save = vi.fn(async () => {});
    render(<PoolEditor config={config} save={save} close={() => {}} />);
    await submitWithID("pool-a");
    expect(save).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toContain("内部 ID");
  });
});
