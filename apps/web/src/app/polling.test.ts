// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { startPolling } from "./polling";

let hidden = false;
beforeEach(() => {
  vi.useFakeTimers();
  hidden = false;
  vi.spyOn(document, "hidden", "get").mockImplementation(() => hidden);
});
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});
function deferredTask() {
  const runs: { signal: AbortSignal; done: () => void }[] = [];
  const task = vi.fn(
    (signal: AbortSignal) =>
      new Promise<void>((done) => runs.push({ signal, done })),
  );
  return { runs, task };
}
const settle = () => vi.advanceTimersByTimeAsync(0);

describe("polling", () => {
  it("never overlaps a slow request", async () => {
    const { runs, task } = deferredTask();
    const stop = startPolling(task, 1000);
    await vi.advanceTimersByTimeAsync(3500);
    expect(task).toHaveBeenCalledTimes(1);
    runs[0].done();
    await settle();
    await vi.advanceTimersByTimeAsync(1000);
    expect(task).toHaveBeenCalledTimes(2);
    stop();
  });

  it("skips hidden pages and refreshes as soon as the page is visible", async () => {
    const { runs, task } = deferredTask();
    const stop = startPolling(task, 1000, false);
    hidden = true;
    await vi.advanceTimersByTimeAsync(5000);
    expect(task).not.toHaveBeenCalled();
    hidden = false;
    document.dispatchEvent(new Event("visibilitychange"));
    expect(task).toHaveBeenCalledTimes(1);
    runs[0].done();
    stop();
  });

  it("aborts the in-flight request and stops scheduling when stopped", async () => {
    const { runs, task } = deferredTask();
    const stop = startPolling(task, 1000);
    stop();
    expect(runs[0].signal.aborted).toBe(true);
    runs[0].done();
    await vi.advanceTimersByTimeAsync(5000);
    document.dispatchEvent(new Event("visibilitychange"));
    expect(task).toHaveBeenCalledTimes(1);
  });
});
