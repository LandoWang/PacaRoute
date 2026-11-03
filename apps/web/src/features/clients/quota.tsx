import {
  type Client,
  type ClientQuota,
  type ClientQuotaState,
} from "../../api/types";
import { useLoad } from "../../app/state";
import { Field, Input, Select, ErrorNote } from "../../components/ui";
import { format } from "../../domain/format";
import { POLL_INTERVAL_MS } from "../../app/polling";
export function QuotaFields({
  value,
  onChange,
}: {
  value?: ClientQuota;
  onChange: (value: ClientQuota | undefined) => void;
}) {
  const set = (patch: Partial<ClientQuota>) =>
    onChange({ ...value!, ...patch });
  return (
    <details>
      <summary>客户端配额</summary>
      <label className="switch-row">
        <span>启用配额限制</span>
        <input
          type="checkbox"
          checked={!!value}
          onChange={(e) =>
            onChange(e.target.checked ? { window_seconds: 86400 } : undefined)
          }
        />
      </label>
      {value && (
        <>
          <Field label="滚动统计窗口">
            <Select
              value={value.window_seconds}
              onChange={(e) => set({ window_seconds: Number(e.target.value) })}
            >
              <option value={60}>最近 1 分钟</option>
              <option value={3600}>最近 1 小时</option>
              <option value={86400}>最近 24 小时</option>
              <option value={2592000}>最近 30 天</option>
              {![60, 3600, 86400, 2592000].includes(value.window_seconds) && (
                <option value={value.window_seconds}>
                  {value.window_seconds} 秒
                </option>
              )}
            </Select>
          </Field>
          <div className="form-grid">
            <Field
              label="请求数上限"
              hint="空白或 0 表示不限；已接受的请求立即计数"
            >
              <Input
                type="number"
                min="0"
                step="1"
                value={value.requests || ""}
                onChange={(e) => set({ requests: Number(e.target.value) || 0 })}
              />
            </Field>
            <Field
              label="输入 token 上限"
              hint="含缓存读取和写入，按上游确认用量累计"
            >
              <Input
                type="number"
                min="0"
                step="1"
                value={value.input_tokens || ""}
                onChange={(e) =>
                  set({ input_tokens: Number(e.target.value) || 0 })
                }
              />
            </Field>
            <Field label="费用上限（USD）" hint="空白或 0 表示不限">
              <Input
                type="text"
                inputMode="decimal"
                placeholder="例如 10.00"
                value={value.cost_usd || ""}
                onChange={(e) => set({ cost_usd: e.target.value })}
              />
            </Field>
          </div>
          <p className="muted">
            按已确认用量拦截后续请求。并发请求可能少量超额；缺少用量或费率会显示为待计价，不会当作零费用。到限返回
            429。
          </p>
        </>
      )}
    </details>
  );
}
export function QuotaMeter({ client }: { client: Client }) {
  const load = useLoad<ClientQuotaState>(
      `clients/${client.id}/quota`,
      POLL_INTERVAL_MS,
    ),
    v = load.data;
  if (load.error) return <ErrorNote>配额统计暂时不可用</ErrorNote>;
  if (!v) return null;
  const limit = (n?: number | string) =>
    n && Number(n) > 0
      ? ` / ${typeof n === "number" ? format(n) : "$" + n}`
      : "";
  return (
    <div className="quota-meter">
      <strong>
        {v.blocked ? "配额已达上限 · 新请求将返回 429" : "客户端用量与配额"}
      </strong>
      <div className="inventory">
        <span>
          请求 {format(v.requests)}
          {limit(v.limits?.requests)}
        </span>
        <span>
          输入 {format(v.input_tokens)}
          {limit(v.limits?.input_tokens)}
        </span>
        <span>
          已确认费用 ${v.cost_usd}
          {limit(v.limits?.cost_usd)}
        </span>
      </div>
      <small className="muted">
        最近{" "}
        {v.window_seconds >= 86400
          ? v.window_seconds / 86400 + " 天"
          : v.window_seconds >= 3600
            ? v.window_seconds / 3600 + " 小时"
            : v.window_seconds / 60 + " 分钟"}{" "}
        · {v.pending} 个待结算 · {v.unpriced}{" "}
        个待计价。按本机账本与配置费率统计，不代表平台账单。
      </small>
    </div>
  );
}
