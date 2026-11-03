import { RefreshCw } from "lucide-react";
import { useState } from "react";
import { type Agent, type Config, type Model } from "../../api/types";
import { useLoad } from "../../app/state";
import { Button, Input, Select, Field, ErrorNote } from "../../components/ui";
import { sources } from "../../domain/config";
import { agentNames, poolModels } from "../../domain/routing";
export function ModelMultiSelect({
  config,
  source,
  agent,
  selected,
  defaultModel,
  onChange,
  onDefault,
}: {
  config: Config;
  source: string;
  agent: Agent;
  selected: string[];
  defaultModel: string;
  onChange: (models: string[]) => void;
  onDefault: (model: string) => void;
}) {
  const catalog = useLoad<{ models: Model[] }>(
    "models?source=" + encodeURIComponent(source),
  );
  const [query, setQuery] = useState(""),
    [manual, setManual] = useState("");
  const resource = sources(config).find((s) => s.id === source);
  const pool = config.pools.find((p) => "pool:" + p.id === source);
  const all = new Map((catalog.data?.models || []).map((m) => [m.id, m]));
  for (const id of [
    ...(resource?.deployments || []),
    ...(pool ? poolModels(pool) : []),
    ...selected,
  ])
    if (!all.has(id))
      all.set(id, {
        id,
        name: id,
        vendor: "",
        codex:
          resource?.kind === "pacaroute"
            ? !!resource.agent_models?.codex?.includes(id)
            : resource?.kind !== "copilot",
        claude:
          resource?.kind === "pacaroute"
            ? !!resource.agent_models?.["claude-code"]?.includes(id)
            : resource?.kind !== "copilot",
      });
  const models = [...all.values()];
  const supported = (m: Model) =>
    agent === "pacaroute"
      ? m.codex || m.claude
      : agent === "codex"
        ? m.codex
        : m.claude;
  const visible = models.filter((m) =>
    `${m.id} ${m.name} ${m.vendor}`.toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <div className="model-selection">
      <div className="row-between">
        <strong>
          可用模型 · 多选{" "}
          <span className="muted">
            {selected.length} 已选 ·{" "}
            {catalog.loading ? "正在获取目录…" : `${models.length} 个模型`}
          </span>
        </strong>
        <Button
          variant="ghost"
          size="icon"
          aria-label="刷新模型列表"
          disabled={catalog.loading}
          onClick={catalog.refresh}
        >
          <RefreshCw size={15} className={catalog.loading ? "spin" : ""} />
        </Button>
      </div>
      <Input
        aria-label="搜索模型"
        placeholder="搜索模型或提供商"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      <div
        className="model-options"
        role="group"
        aria-label={`${agentNames[agent]} 可用模型`}
      >
        {visible.map((m) => (
          <label key={m.id} className="model-option">
            <input
              type="checkbox"
              checked={selected.includes(m.id)}
              disabled={!supported(m) && !selected.includes(m.id)}
              onChange={(e) =>
                onChange(
                  e.target.checked
                    ? [...selected, m.id]
                    : selected.filter((id) => id !== m.id),
                )
              }
            />
            <span>
              <strong>{m.name}</strong>
              <small>
                {m.id}
                {!supported(m) && !catalog.loading
                  ? catalog.data?.models.some((v) => v.id === m.id)
                    ? ` · 暂未适配 ${agentNames[agent]}`
                    : " · 本次目录未确认，保留已选配置"
                  : ""}
              </small>
            </span>
          </label>
        ))}
        {!visible.length && (
          <p className="muted">
            {catalog.loading ? "正在获取模型…" : "没有匹配的模型"}
          </p>
        )}
      </div>
      {catalog.error && <ErrorNote>{catalog.error}</ErrorNote>}
      <div className="actions">
        <Button
          variant="ghost"
          size="sm"
          disabled={catalog.loading}
          onClick={() =>
            onChange([
              ...new Set([
                ...selected,
                ...visible.filter(supported).map((m) => m.id),
              ]),
            ])
          }
        >
          选择列表中可用模型
        </Button>
        <Button variant="ghost" size="sm" onClick={() => onChange([])}>
          清空
        </Button>
      </div>
      {!pool && resource?.kind !== "copilot" && (
        <details>
          <summary>手动添加模型 / deployment</summary>
          <div className="input-action">
            <Input
              aria-label="模型 ID"
              value={manual}
              onChange={(e) => setManual(e.target.value)}
              placeholder="模型 ID"
            />
            <Button
              variant="secondary"
              disabled={!manual.trim() || selected.includes(manual.trim())}
              onClick={() => {
                onChange([...selected, manual.trim()]);
                setManual("");
              }}
            >
              添加
            </Button>
          </div>
        </details>
      )}
      {agent !== "pacaroute" && (
        <Field
          label="默认接入模型"
          hint="用于生成接入脚本。Agent 切换模型时，按请求里的 model 匹配；未勾选的模型会被拒绝。"
        >
          <Select
            value={defaultModel}
            disabled={!selected.length}
            onChange={(e) => onDefault(e.target.value)}
          >
            {!selected.length && <option value="">先勾选模型</option>}
            {selected.map((id) => (
              <option key={id} value={id}>
                {id}
              </option>
            ))}
          </Select>
        </Field>
      )}
    </div>
  );
}
