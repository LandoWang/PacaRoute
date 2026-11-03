import { type Config, type Pool, type RoutingHealth } from "../../api/types";
import { useApp } from "../../app/state";
import { Badge, Button } from "../../components/ui";
import { memberModel } from "../../domain/routing";
export function poolAllocation(
  config: Config,
  pool: Pool,
  health?: RoutingHealth,
  model?: string,
) {
  return pool.members
    .filter((m) => !model || memberModel(pool, m) === model)
    .map((member) => ({
      ...member,
      weight:
        config.source_state.paused.includes(member.source_id) ||
        health?.cooldowns.some(
          (c) => c.source_id === member.source_id && c.model === member.model,
        )
          ? 0
          : member.weight,
    }));
}

export function RoutingAlert() {
  const { config, status, navigate } = useApp();
  const health = status.routing;
  if (!health) return null;
  const unavailable = health.pools.flatMap((p) => {
    const name = config.pools.find((v) => v.id === p.id)?.name || p.id;
    return p.models?.length
      ? p.models
          .filter((m) => m.available === 0)
          .map((m) => `${name} · ${m.model}`)
      : p.available === 0
        ? [name]
        : [];
  });
  if (!unavailable.length && !health.cooldowns.length) return null;
  return (
    <div
      className={`routing-alert ${unavailable.length ? "critical" : "warning"}`}
      role="alert"
    >
      <div>
        <strong>
          {unavailable.length ? "来源池无可用成员" : "来源异常，正在冷却"}
        </strong>
        <p>
          {unavailable.length
            ? unavailable.join("、") +
              " 无法接收新会话，请检查来源、冷却状态及权重。"
            : `${health.cooldowns.length} 个来源模型正在冷却，新会话将避开这些成员。`}
        </p>
      </div>
      <Button variant="secondary" size="sm" onClick={() => navigate("pools")}>
        查看来源池
      </Button>
    </div>
  );
}

export function CooldownDetails({
  source,
  model,
}: {
  source: string;
  model?: string;
}) {
  const { status } = useApp();
  const health = status.routing;
  const entries =
    health?.cooldowns.filter(
      (c) => c.source_id === source && (!model || c.model === model),
    ) || [];
  if (!entries.length) return null;
  return (
    <div className="cooldown-details">
      {entries.map((c) => (
        <div key={c.model}>
          <div className="title-line">
            <Badge tone="amber">
              {c.kind === "throttle" ? "限流冷却" : "错误冷却"} · 约{" "}
              {Math.max(
                0,
                Math.ceil(
                  (Date.parse(c.until) - Date.parse(health!.observed_at)) /
                    1000,
                ),
              )}{" "}
              秒
            </Badge>
            {!model && <strong>{c.model}</strong>}
          </div>
          <p className="cooldown-reason">{c.error}</p>
          <small>
            最近失败 {new Date(c.failed_at).toLocaleTimeString()} ·{" "}
            {new Date(c.until).toLocaleTimeString()} 后允许重试
          </small>
        </div>
      ))}
    </div>
  );
}

export function PoolAvailability({
  id,
  model,
}: {
  id: string;
  model?: string;
}) {
  const { status } = useApp();
  const entry = status.routing?.pools.find((p) => p.id === id);
  const pool = model ? entry?.models?.find((m) => m.model === model) : entry;
  if (!pool) return <p className="muted">路由状态暂不可用</p>;
  return (
    <div className="pool-availability">
      <Badge
        tone={
          pool.available === 0
            ? "rose"
            : pool.available < pool.total
              ? "amber"
              : "mint"
        }
      >
        {pool.available === 0
          ? "无可用成员"
          : pool.available < pool.total
            ? "部分成员不可用"
            : "可分配"}
      </Badge>
      <span>
        新会话可用 {pool.available} / {pool.total}
      </span>
    </div>
  );
}
