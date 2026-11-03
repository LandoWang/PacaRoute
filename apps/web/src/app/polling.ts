// Refresh interval for page data (usage, activity, errors, quotas).
export const POLL_INTERVAL_MS = 15_000;
// Refresh interval for service status and routing health.
export const STATUS_POLL_INTERVAL_MS = 5_000;
// Runs `task` now (unless `immediate` is false) and then every `interval` ms.
// Runs never overlap, hidden pages are skipped, a page that becomes visible
// again refreshes at once and stopping aborts the in-flight request.
export function startPolling(
  task: (signal: AbortSignal) => Promise<unknown>,
  interval: number,
  immediate = true,
): () => void {
  const controller = new AbortController();
  let pending = false;
  const run = () => {
    if (pending || controller.signal.aborted) return;
    pending = true;
    void task(controller.signal)
      .catch(() => {})
      .finally(() => {
        pending = false;
      });
  };
  const visible = () => {
    if (!document.hidden) run();
  };
  if (immediate) run();
  const timer = interval ? setInterval(visible, interval) : undefined;
  if (interval) document.addEventListener("visibilitychange", visible);
  return () => {
    controller.abort();
    clearInterval(timer);
    document.removeEventListener("visibilitychange", visible);
  };
}
