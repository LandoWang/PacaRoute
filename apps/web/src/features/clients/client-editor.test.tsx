// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { type Client, type Config } from "../../api/types";
import { RevisionConflictError } from "../../app/config-save";
import { StateContext, type AppState } from "../../app/state";
import { ClientEditor } from "./client-editor";

const client: Client = {
  id: "alice",
  name: "Alice",
  disabled: false,
  enabled_agents: ["codex"],
  routes: [],
};
const config = (revision: string, name: string) =>
  ({ revision, clients: [{ ...client, name }] }) as unknown as Config;
afterEach(cleanup);

describe("client editor after a revision conflict", () => {
  it("does not overwrite the newer configuration when saved again", async () => {
    const save = vi
      .fn<AppState["save"]>()
      .mockRejectedValueOnce(new RevisionConflictError())
      .mockResolvedValue(undefined);
    const view = (c: Config) => (
      <StateContext.Provider value={{ config: c, save } as unknown as AppState}>
        <ClientEditor original={client} onClose={() => {}} onKey={() => {}} />
      </StateContext.Provider>
    );
    const { rerender } = render(view(config("1", "Alice")));
    fireEvent.change(screen.getByLabelText("名称"), {
      target: { value: "Alice from tab A" },
    });
    const form = document.querySelector("form")!;
    await act(async () => fireEvent.submit(form));
    expect(screen.getByRole("alert").textContent).toContain("其他位置更改");
    // App publishes the newer snapshot after the conflict.
    rerender(view(config("2", "Alice from tab B")));
    await act(async () => fireEvent.submit(form));
    expect(save).toHaveBeenCalledTimes(2);
    expect(save.mock.calls.map(([next]) => next.revision)).toEqual(["1", "1"]);
  });
});
