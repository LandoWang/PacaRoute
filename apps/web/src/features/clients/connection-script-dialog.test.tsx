// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { type Client, type Config, type Status } from "../../api/types";
import { StateContext, type AppState } from "../../app/state";
import { ConnectionScript } from "./connection-script-dialog";

const client: Client = {
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
      model: "m1",
    },
  ],
};
const state = {
  config: {
    sources: [{ id: "s1", kind: "openai", deployments: ["m1"] }],
  } as unknown as Config,
  status: { gateway_url: "https://router.example" } as Status,
} as AppState;
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("connection script dialog", () => {
  it("shows a masked preview for PowerShell as well as Bash", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({ api_key: "synthetic-client-key" })),
    );
    render(
      <StateContext.Provider value={state}>
        <ConnectionScript client={client} agent="codex" onClose={() => {}} />
      </StateContext.Provider>,
    );
    for (const platform of ["shell", "powershell"]) {
      fireEvent.change(screen.getByLabelText("系统"), {
        target: { value: platform },
      });
      const preview = await screen.findByText(/<客户端密钥 已隐藏>/);
      expect(preview.textContent).not.toContain("synthetic-client-key");
      expect(screen.queryByRole("alert")).toBeNull();
    }
  });
});
