// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { type Config } from "../../api/types";
import { SourceEditor } from "./source-editor";

const config = {
  revision: "1",
  accounts: [{ id: "a", name: "OpenAI", kind: "openai" }],
  sources: [],
  pools: [],
  clients: [],
  proxies: { profiles: [], bindings: {}, account_proxy_id: "" },
  source_state: { paused: [] },
} as unknown as Config;

function pendingDiscovery() {
  const requests: { signal: AbortSignal; resolve: (v: unknown) => void }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(
      (_: string, init: RequestInit) =>
        new Promise((resolve) =>
          requests.push({
            signal: init.signal!,
            resolve: (v) => resolve(Response.json(v)),
          }),
        ),
    ),
  );
  return requests;
}
const discovered = {
  endpoint: "https://api.openai.com/v1",
  models: [{ id: "stale-model", name: "Stale model" }],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("source model discovery", () => {
  it("cancels discovery when the endpoint changes instead of reverting the edit", async () => {
    const requests = pendingDiscovery();
    render(<SourceEditor config={config} save={vi.fn()} close={() => {}} />);
    expect(requests).toHaveLength(1);
    const endpoint = screen.getByLabelText("资源 Endpoint") as HTMLInputElement;
    fireEvent.change(endpoint, {
      target: { value: "https://proxy.example/v1" },
    });
    expect(requests[0].signal.aborted).toBe(true);
    await act(async () => requests[0].resolve(discovered));
    expect(endpoint.value).toBe("https://proxy.example/v1");
    expect(screen.queryByText("Stale model")).toBeNull();
  });

  it("aborts an in-flight discovery when the editor closes", () => {
    const requests = pendingDiscovery();
    const { unmount } = render(
      <SourceEditor config={config} save={vi.fn()} close={() => {}} />,
    );
    unmount();
    expect(requests[0].signal.aborted).toBe(true);
  });

  it("applies a current discovery result", async () => {
    const requests = pendingDiscovery();
    render(<SourceEditor config={config} save={vi.fn()} close={() => {}} />);
    await act(async () => requests[0].resolve(discovered));
    expect(await screen.findByText("Stale model")).toBeTruthy();
  });
});
