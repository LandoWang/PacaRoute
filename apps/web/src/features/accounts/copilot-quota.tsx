import { Badge, ErrorNote } from "../../components/ui";
type Quota = {
  copilot_plan?: string;
  quota_reset_date?: string;
  quota_snapshots?: Record<
    string,
    {
      unlimited?: boolean;
      percent_remaining?: number;
      remaining?: number;
      quota_remaining?: number;
      entitlement?: number;
      token_based_billing?: boolean;
    }
  >;
};
export type CopilotInfo = {
  quota?: Quota;
  quota_error?: string;
  login?: string;
};
const number = (v?: number) =>
  typeof v === "number" && Number.isFinite(v) ? v.toLocaleString("zh-CN") : "—";
export function CopilotQuotaDetails({ info }: { info: CopilotInfo }) {
  const quota = info.quota;
  const entries = Object.entries(quota?.quota_snapshots || {});
  return (
    <>
      {info.quota_error && <ErrorNote>{info.quota_error}</ErrorNote>}
      <div className="actions">
        {quota?.copilot_plan && (
          <Badge tone="violet">{quota.copilot_plan}</Badge>
        )}
        {info.login && <small>{info.login}</small>}
        {quota?.quota_reset_date && (
          <small>下次重置：{quota.quota_reset_date}</small>
        )}
      </div>
      {entries.length ? (
        <div className="quota-grid">
          {entries.map(([id, value]) => (
            <div className="quota-item" key={id}>
              <small>
                {(
                  {
                    premium_interactions: "Premium 额度",
                    chat: "Chat",
                    completions: "代码补全",
                  } as Record<string, string>
                )[id] || id}
              </small>
              <strong>
                {value.unlimited
                  ? "不限量"
                  : typeof value.percent_remaining === "number"
                    ? `${number(value.percent_remaining)}%`
                    : "—"}
              </strong>
              <small>
                {value.unlimited
                  ? "GitHub 返回的额度状态"
                  : `剩余 ${number(value.quota_remaining ?? value.remaining)} / ${number(value.entitlement)} ${value.token_based_billing ? "credits" : "请求"}`}
              </small>
            </div>
          ))}
        </div>
      ) : (
        !info.quota_error && (
          <p className="muted">GitHub 未返回可展示的额度数据。</p>
        )
      )}
      <p className="muted">
        Copilot 账号额度由 GitHub 返回，可能有延迟；与本机请求、token
        统计分别计算。
      </p>
    </>
  );
}
