import { RefreshCw } from "lucide-react";
import { useState, useEffect, useRef } from "react";
import { api } from "../../api/transport";
import { type ModelDiscovery, type Source } from "../../api/types";
import { type Common } from "../../app/editor-props";
import { BrandIcon } from "../../components/brand-icon";
import { identifier } from "../../components/identity";
import {
  Button,
  Input,
  Select,
  Field,
  Editor,
  Badge,
} from "../../components/ui";
import {
  peerCapabilities,
  accountProvider,
  compatibleAccount,
  resourceKind,
  defaultEndpoint,
} from "../../domain/accounts";
import {
  generatedID,
  suggestedName,
  accounts,
  sources,
  upsertByID,
} from "../../domain/config";
export function SourceEditor({
  config,
  save,
  close,
  original,
  account,
}: { original?: Source; account?: string } & Common) {
  const [v, set] = useState<Source>(() => {
    if (original) return structuredClone(original);
    const a =
      accounts(config).find((a) => a.id === account) || accounts(config)[0];
    const kind = resourceKind(a.kind);
    return {
      id: generatedID("source"),
      name: suggestedName(
        a.name +
          (kind === "copilot"
            ? " · Copilot"
            : kind === "azure-openai"
              ? " · Foundry"
              : " · API"),
        sources(config),
      ),
      kind,
      account_id: a.id,
      auth_mode: kind === "azure-openai" ? "azure-cli" : "api-key",
      endpoint: kind === "pacaroute" ? a.endpoint || "" : defaultEndpoint(kind),
      deployments: [],
    };
  });
  const [deployments, setDeployments] = useState(
    (v.deployments || []).join(", "),
  );
  const [proxy, setProxy] = useState(config.proxies.bindings[v.id] || "system");
  const [discovery, setDiscovery] = useState<ModelDiscovery>();
  const [discovering, setDiscovering] = useState(false),
    [discoveryError, setDiscoveryError] = useState("");
  const selected = deployments
    .split(/[,\n]/)
    .map((s) => s.trim())
    .filter(Boolean);
  const discoveredOnce = useRef(false);
  const discoveryRequest = useRef<AbortController | undefined>(undefined);
  useEffect(() => {
    if (
      discoveredOnce.current ||
      original ||
      v.kind === "copilot" ||
      v.kind === "azure-openai"
    )
      return;
    discoveredOnce.current = true;
    void discover();
  }, []);
  useEffect(() => () => discoveryRequest.current?.abort(), []);
  // A discovery result describes the endpoint and account it was started
  // with; editing either cancels it instead of letting it overwrite the draft.
  function cancelDiscovery() {
    discoveryRequest.current?.abort();
    discoveryRequest.current = undefined;
    setDiscovering(false);
  }
  async function discover() {
    discoveryRequest.current?.abort();
    const controller = new AbortController();
    discoveryRequest.current = controller;
    setDiscovering(true);
    setDiscoveryError("");
    try {
      const result = await api<ModelDiscovery>(
        "model-discovery",
        "POST",
        { ...v, proxy_id: proxy },
        controller.signal,
      );
      if (controller.signal.aborted) return;
      setDiscovery(result);
      set((current) => ({
        ...current,
        endpoint: result.endpoint || current.endpoint,
        agent_models:
          current.kind === "pacaroute"
            ? peerCapabilities(result.models)
            : current.agent_models,
        resource_id: result.resource_id,
        subscription_id: result.subscription_id || current.subscription_id,
      }));
    } catch (e) {
      if (!controller.signal.aborted) setDiscoveryError((e as Error).message);
    } finally {
      if (discoveryRequest.current === controller) {
        discoveryRequest.current = undefined;
        setDiscovering(false);
      }
    }
  }
  return (
    <Editor
      title={original ? "编辑来源" : "添加来源"}
      fixedHeight
      description={
        v.kind === "copilot"
          ? "连接这个 GitHub 账号的 Copilot，模型列表自动同步。"
          : "选择这个账号要开放的模型。"
      }
      onClose={close}
      saveLabel={original ? "保存" : "添加来源"}
      onSave={async () => {
        if (v.kind !== "copilot" && !selected.length)
          throw Error("请选择至少一个模型，或展开手动填写模型。");
        if (
          v.kind === "pacaroute" &&
          selected.some(
            (id) =>
              !Object.values(v.agent_models || {}).some((models) =>
                models.includes(id),
              ),
          )
        )
          throw Error("请先发现模型，再选择远端客户端可访问的模型。");
        const value = { ...v, deployments: selected };
        await save({
          ...config,
          sources: upsertByID(config.sources, value, original?.id),
          proxies: {
            ...config.proxies,
            bindings: { ...config.proxies.bindings, [v.id]: proxy },
          },
        });
        close();
      }}
    >
      <div className="source-identity">
        <BrandIcon kind={v.kind} size={26} />
        <Badge tone="mint">
          {accountProvider(
            accounts(config).find((a) => a.id === v.account_id)?.kind || "",
          )}
        </Badge>
        <strong>
          {accounts(config).find((a) => a.id === v.account_id)?.name}
        </strong>
        <span>
          {v.kind === "copilot"
            ? "GitHub Copilot"
            : v.kind === "azure-openai"
              ? "Azure Foundry / AOAI"
              : "API"}
        </span>
      </div>
      {v.kind !== "copilot" && (
        <>
          <div className="row-between">
            <strong>可用模型</strong>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={discovering || !v.endpoint}
              onClick={() => void discover()}
            >
              <RefreshCw size={14} className={discovering ? "spin" : ""} />
              {discovering ? "正在发现…" : "发现模型"}
            </Button>
          </div>
          {discoveryError && (
            <p role="alert" className="text-error">
              {discoveryError}
            </p>
          )}
          {discovery ? (
            <div className="model-discovery-list">
              {discovery.models.length === 0 ? (
                <p className="muted">未发现模型，可以手动填写。</p>
              ) : (
                discovery.models.map((m) => (
                  <label key={m.id}>
                    <input
                      type="checkbox"
                      checked={selected.includes(m.id)}
                      onChange={(e) =>
                        setDeployments(
                          (e.target.checked
                            ? [...new Set([...selected, m.id])]
                            : selected.filter((s) => s !== m.id)
                          ).join(", "),
                        )
                      }
                    />
                    <span>
                      {m.name}
                      <small>{m.id}</small>
                    </span>
                  </label>
                ))
              )}
            </div>
          ) : (
            <p className="muted">
              {selected.length
                ? `已选：${selected.join("、")}`
                : "点击发现模型，再勾选需要使用的模型。"}
            </p>
          )}
          {v.kind !== "pacaroute" && (
            <details className="advanced-options">
              <summary>手动填写模型</summary>
              <Field
                label="模型 / Deployment"
                hint="使用逗号分隔；Azure 使用 deployment 名称"
              >
                <Input
                  value={deployments}
                  onChange={(e) => setDeployments(e.target.value)}
                  placeholder="模型名称"
                />
              </Field>
            </details>
          )}
        </>
      )}
      <details
        className="advanced-options"
        open={v.kind === "azure-openai" && !original ? true : undefined}
      >
        <summary>高级设置</summary>
        <Field label="来源名称">
          <Input
            value={v.name}
            required
            onChange={(e) => set({ ...v, name: e.target.value })}
          />
        </Field>
        <Field label="内部 ID" hint="已自动生成，通常无需修改">
          <Input
            {...identifier}
            disabled={!!original}
            value={v.id}
            onChange={(e) => set({ ...v, id: e.target.value })}
          />
        </Field>
        <Field label="账号">
          <Select
            value={v.account_id}
            onChange={(e) => {
              set({
                ...v,
                account_id: e.target.value,
                ...(v.kind === "pacaroute"
                  ? {
                      endpoint:
                        accounts(config).find((a) => a.id === e.target.value)
                          ?.endpoint || "",
                      agent_models: undefined,
                    }
                  : {}),
              });
              cancelDiscovery();
              setDiscovery(undefined);
              setDiscoveryError("");
            }}
          >
            {accounts(config)
              .filter((a) => compatibleAccount(v.kind, a.kind))
              .map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
          </Select>
        </Field>
        {v.kind !== "copilot" && (
          <Field label="资源 Endpoint">
            <Input
              type="url"
              required
              value={v.endpoint}
              onChange={(e) => {
                set({
                  ...v,
                  endpoint: e.target.value,
                  resource_id: undefined,
                  agent_models: undefined,
                });
                cancelDiscovery();
                setDiscovery(undefined);
              }}
            />
          </Field>
        )}
        {v.kind === "azure-openai" && (
          <>
            <Field label="认证方式">
              <Select
                value={v.auth_mode}
                onChange={(e) => set({ ...v, auth_mode: e.target.value })}
              >
                <option value="azure-cli">Azure CLI 登录</option>
                <option value="api-key">API Key</option>
              </Select>
            </Field>
            <Field label="订阅 ID">
              <Input
                value={v.subscription_id || ""}
                onChange={(e) => set({ ...v, subscription_id: e.target.value })}
              />
            </Field>
          </>
        )}
        <Field label="网络 / 代理">
          <Select value={proxy} onChange={(e) => setProxy(e.target.value)}>
            <option value="system">跟随系统环境</option>
            <option value="direct">直连</option>
            {config.proxies.profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </Select>
        </Field>
      </details>
    </Editor>
  );
}
