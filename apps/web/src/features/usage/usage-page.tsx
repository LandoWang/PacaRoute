import { ArrowRight, Activity as ActivityIcon } from "lucide-react";
import { useState, Fragment, memo, useMemo } from "react";
import { type Config, type Usage, type Totals } from "../../api/types";
import { useApp, useLoad } from "../../app/state";
import { Loading } from "../../components/feedback";
import {
  Button,
  Badge,
  Panel,
  Field,
  Input,
  Select,
} from "../../components/ui";
import { sources, sourceName } from "../../domain/config";
import { format } from "../../domain/format";
import { Stats } from "./stats";
export function UsagePage() {
  const { config, status, navigate } = useApp();
  const [range, setRange] = useState("24"),
    [source, setSource] = useState(""),
    [client, setClient] = useState(""),
    [bounds, setBounds] = useState(() => ({ from: "", to: "" }));
  const to = new Date();
  to.setUTCHours(to.getUTCHours() + 1, 0, 0, 0);
  const from = new Date(
    +to - Number(range === "custom" ? "24" : range) * 3600000,
  );
  const fromISO =
      range === "custom" && bounds.from
        ? new Date(bounds.from).toISOString()
        : from.toISOString(),
    toISO =
      range === "custom" && bounds.to
        ? new Date(bounds.to).toISOString()
        : to.toISOString();
  const result = useLoad<Usage>(
    `usage?from=${encodeURIComponent(fromISO)}&to=${encodeURIComponent(toISO)}&source=${source}&client=${client}`,
    15000,
  );
  const hours = useMemo(
    () => usageHours(result.data, fromISO, toISO),
    [result.data, fromISO, toISO],
  );
  return (
    <>
      <div className="filters">
        <Field label="时间范围">
          <Select value={range} onChange={(e) => setRange(e.target.value)}>
            <option value="24">最近 24 小时</option>
            <option value="168">最近 7 天</option>
            <option value="720">最近 30 天</option>
            <option value="custom">自定义</option>
          </Select>
        </Field>
        {range === "custom" && (
          <>
            <Field label="开始时间">
              <Input
                type="datetime-local"
                value={bounds.from}
                onChange={(e) => setBounds({ ...bounds, from: e.target.value })}
              />
            </Field>
            <Field label="结束时间">
              <Input
                type="datetime-local"
                value={bounds.to}
                onChange={(e) => setBounds({ ...bounds, to: e.target.value })}
              />
            </Field>
          </>
        )}
        <Field label="来源">
          <Select value={source} onChange={(e) => setSource(e.target.value)}>
            <option value="">全部来源</option>
            {sources(config).map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="客户端">
          <Select value={client} onChange={(e) => setClient(e.target.value)}>
            <option value="">全部客户端</option>
            <option value="local">本机调用</option>
            {config.clients.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </Select>
        </Field>
      </div>
      <Loading loading={result.loading} error={result.error} />
      <Stats total={result.data?.total} />
      {result.data?.total.requests === 0 && (
        <div className="connection-strip">
          <ActivityIcon size={20} />
          <div>
            <strong>这个范围内还没有请求经过当前服务</strong>
            <p>
              只统计发送到 {status.gateway_url || location.origin}{" "}
              的请求。直连来源或其他端口的调用不会记在这里。
            </p>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => navigate("clients")}
            >
              到客户端获取接入脚本 <ArrowRight size={14} />
            </Button>
          </div>
        </div>
      )}
      {!!result.data?.total.missing_usage && (
        <p className="text-warning">
          {result.data.total.missing_usage} 次请求缺少完整 token
          用量；未知用量不会按 0 估算。
        </p>
      )}
      <UsageChart hours={hours} />
      <UsageBreakdown usage={result.data} config={config} />
    </>
  );
}
// Hourly buckets for the requested range; the chart and table below are
// memoized so status polling does not re-render thousands of bars and rows.
function usageHours(usage: Usage | undefined, fromISO: string, toISO: string) {
  const hours: [string, Totals][] = [];
  if (usage) {
    const start = new Date(fromISO);
    start.setUTCMinutes(0, 0, 0);
    for (
      let t = +start;
      t < +new Date(toISO) && hours.length < 8784;
      t += 3600000
    ) {
      const key = new Date(t).toISOString().replace(".000Z", "Z");
      hours.push([
        key,
        usage.timeline[key] || {
          requests: 0,
          input_tokens: 0,
          output_tokens: 0,
          cached_tokens: 0,
          failures: 0,
          missing_usage: 0,
        },
      ]);
    }
  }
  return hours;
}
const UsageChart = memo(function UsageChart({
  hours,
}: {
  hours: [string, Totals][];
}) {
  const max = Math.max(
    1,
    ...hours.map(([, v]) => v.input_tokens + v.output_tokens),
  );
  return (
    <Panel title="Token 时间分布" description="按小时聚合 · 横轴以本地时间显示">
      <div className="usage-chart">
        <svg
          role="img"
          aria-label="每小时输入与输出 token"
          viewBox="0 0 1000 200"
          preserveAspectRatio="none"
        >
          <line x1="0" y1="190" x2="1000" y2="190" stroke="#334155" />
          {hours.map(([time, v], i) => (
            <g key={time}>
              <title>
                {new Date(time).toLocaleString()} · 输入 {v.input_tokens} / 输出{" "}
                {v.output_tokens}
              </title>
              <rect
                x={(i * 1000) / hours.length}
                y={190 - (v.input_tokens / max) * 175}
                width={Math.max(0.5, 1000 / hours.length - 2)}
                height={(v.input_tokens / max) * 175}
                rx="2"
                fill="#67e8c5"
              />
              <rect
                x={(i * 1000) / hours.length}
                y={190 - ((v.input_tokens + v.output_tokens) / max) * 175}
                width={Math.max(0.5, 1000 / hours.length - 2)}
                height={(v.output_tokens / max) * 175}
                rx="2"
                fill="#b59aff"
              />
            </g>
          ))}
        </svg>
        {hours.every(([, v]) => v.requests === 0) && (
          <span className="chart-empty">这个时间范围还没有请求</span>
        )}
      </div>
      <div className="chart-caption">
        <span>{hours[0] && new Date(hours[0][0]).toLocaleString()}</span>
        <div className="actions">
          <Badge tone="mint">输入</Badge>
          <Badge tone="violet">输出</Badge>
        </div>
        <span>
          {hours.at(-1) && new Date(hours.at(-1)![0]).toLocaleString()}
        </span>
      </div>
    </Panel>
  );
});
const UsageBreakdown = memo(function UsageBreakdown({
  usage,
  config,
}: {
  usage?: Usage;
  config: Config;
}) {
  return (
    <Panel
      title="按来源 / 模型统计"
      description="每个来源先显示合计，再按实际调用模型展开；缓存为输入 token 中的缓存读取。"
    >
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>来源</th>
              <th>模型</th>
              <th>请求</th>
              <th>输入 token</th>
              <th>缓存 token</th>
              <th>输出 token</th>
              <th>失败</th>
            </tr>
          </thead>
          <tbody>
            {Object.entries(usage?.sources || {})
              .sort(([a], [b]) =>
                sourceName(config, a).localeCompare(sourceName(config, b)),
              )
              .map(([id, v]) => (
                <Fragment key={id}>
                  <tr className="usage-source-total">
                    <th scope="row">{sourceName(config, id)}</th>
                    <td>合计</td>
                    <td>{format(v.requests)}</td>
                    <td>{format(v.input_tokens)}</td>
                    <td>{format(v.cached_tokens)}</td>
                    <td>{format(v.output_tokens)}</td>
                    <td>{v.failures}</td>
                  </tr>
                  {Object.entries(usage?.source_models?.[id] || {})
                    .sort(([, a], [, b]) => b.requests - a.requests)
                    .map(([model, m]) => (
                      <tr key={model} className="usage-model-row">
                        <td>
                          <span className="muted">↳</span>
                        </td>
                        <th scope="row">{model || "未记录模型"}</th>
                        <td>{format(m.requests)}</td>
                        <td>{format(m.input_tokens)}</td>
                        <td>{format(m.cached_tokens)}</td>
                        <td>{format(m.output_tokens)}</td>
                        <td>{m.failures}</td>
                      </tr>
                    ))}
                </Fragment>
              ))}
            {usage?.total.requests === 0 && (
              <tr>
                <td colSpan={7} className="muted">
                  此范围暂无用量
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </Panel>
  );
});
