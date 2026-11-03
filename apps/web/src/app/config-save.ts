import { api, APIError } from "../api/transport";
import { type Config } from "../api/types";
export class RevisionConflictError extends Error {
  constructor() {
    super(
      "配置已在其他位置更改，本次修改未保存。页面已载入最新配置；你的编辑仍保留在此窗口，请核对后关闭并重新编辑。",
    );
  }
}
// Saves a full configuration draft built from `next.revision`. The server
// answers 409 both for a stale revision and for invalid references, so the
// current snapshot decides: a newer revision is published through `onLatest`
// for display and reported as a conflict, never retried over the newer state.
export async function saveConfig(
  next: Config,
  onLatest: (latest: Config) => void,
  request: typeof api = api,
): Promise<Config> {
  try {
    return await request<Config>("config", "PUT", next);
  } catch (e) {
    if (!(e instanceof APIError) || e.status !== 409) throw e;
    const latest = await request<Config>("config").catch(() => undefined);
    if (!latest || latest.revision === next.revision) throw e;
    onLatest(latest);
    throw new RevisionConflictError();
  }
}
