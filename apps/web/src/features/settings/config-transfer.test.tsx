import { renderToStaticMarkup } from "react-dom/server";
import { describe, it, expect } from "vitest";
import { type Config, type ImportPlan } from "../../api/types";
import { accounts, sources } from "../../domain/config";
import { ImportSummary } from "./config-transfer";
describe("configuration transfer preview", () => {
  const plan: ImportPlan = {
    mode: "replace",
    revision: "1",
    namespace: "import",
    fingerprint: "hash",
    added: { accounts: 2, sources: 3, pools: 1, clients: 2, proxies: 1 },
    removed: { accounts: 4, sources: 5, pools: 2, clients: 3, proxies: 1 },
    changes: [
      {
        kind: "accounts",
        from_id: "a",
        to_id: "new-a",
        from_name: "Work",
        to_name: "Work (2)",
      },
    ],
    issues: [
      { code: "client_key_generated", name: "Imported 客户端" },
      { code: "account_credentials_required", name: "Work (2)" },
    ],
  };
  it("makes replacement, renaming and required authorization visible before apply", () => {
    const html = renderToStaticMarkup(<ImportSummary plan={plan} />);
    expect(html).toContain("替换现有 5 项");
    expect(html).toContain("Work (2)");
    expect(html).toContain("需要重新登录");
    expect(html).toContain("生成新客户端密钥");
    expect(
      renderToStaticMarkup(<ImportSummary plan={{ ...plan, mode: "add" }} />),
    ).not.toContain("覆盖会移除");
  });
  it("does not invent accounts or sources in an empty workspace", () => {
    const config = { accounts: [], sources: [] } as unknown as Config;
    expect(accounts(config)).toEqual([]);
    expect(sources(config)).toEqual([]);
  });
});
