import { BrandIcon } from "./components/brand-icon";
import { useState, useEffect, useRef } from "react";
import { ArrowDown, ArrowUp, Plus, Trash2, RefreshCw } from "lucide-react";
import {
  api,
  generatedID,
  suggestedName,
  accountProvider,
  deviceAccount,
  sharedAzure,
  compatibleAccount,
  resourceKind,
  defaultEndpoint,
  type ModelDiscovery,
  accounts,
  sources,
  routeAgent,
  routeTarget,
  share,
  type Config,
  type Client,
  type Source,
  type Account,
  type Route,
  type Pool,
  type Proxy,
  type Model,
  type Agent,
} from "./api";
import { Button, Input, Select, Field, Editor, Badge } from "./components/ui";
type Common = {
  config: Config;
  save: (c: Config) => Promise<void>;
  close: () => void;
};
const identifier = {
  pattern: "[a-z][a-z0-9-]{0,62}",
  title: "小写字母开头，可使用小写字母、数字和短横线",
  required: true,
};
export function Identity({
  id,
  name,
  onID,
  onName,
  existing = false,
}: {
  id: string;
  name: string;
  onID: (v: string) => void;
  onName: (v: string) => void;
  existing?: boolean;
}) {
  return (
    <>
      <Field label="名称">
        <Input value={name} onChange={(e) => onName(e.target.value)} required />
      </Field>
      <details className="advanced-options">
        <summary>高级设置</summary>
        <Field label="内部 ID" hint="已自动生成，通常无需修改">
          <Input
            {...identifier}
            value={id}
            onChange={(e) => onID(e.target.value)}
            disabled={existing}
          />
        </Field>
      </details>
    </>
  );
}
export function AccountEditor({
  config,
  save,
  close,
  onConnect,
}: Common & { onConnect: (account: Account) => void }) {
  const [v, set] = useState<Account>(() => ({
    id: generatedID("account"),
    name: suggestedName("GitHub", accounts(config)),
    kind: "github",
  }));
  const [key, setKey] = useState("");
  const [created, setCreated] = useState(false);
  return (
    <Editor
      title="添加账号"
      onClose={close}
      saveLabel={
        deviceAccount(v.kind) && !sharedAzure(v) ? "继续登录" : "添加账号"
      }
      onSave={async () => {
        if (!created) {
          await save({ ...config, accounts: [...config.accounts, v] });
          setCreated(true);
        }
        if (!deviceAccount(v.kind)) {
          await api(`accounts/${v.id}/key`, "PUT", { key });
          setKey("");
        }
        close();
        if (deviceAccount(v.kind) && !sharedAzure(v)) onConnect(v);
      }}
    >
      <div className="provider-choices" role="group" aria-label="账号类型">
        {["github", "openai", "anthropic", "azure"].map((kind) => (
          <Button
            key={kind}
            type="button"
            variant={v.kind === kind ? "default" : "secondary"}
            disabled={created}
            aria-pressed={v.kind === kind}
            onClick={() => {
              set({
                ...v,
                kind,
                login_mode:
                  kind === "azure"
                    ? "azure-cli"
                    : kind === "github"
                      ? "device"
                      : "",
                oauth_client_id: "",
                name: suggestedName(accountProvider(kind), accounts(config)),
              });
              setKey("");
            }}
          >
            <BrandIcon kind={kind} account size={20} />
            {accountProvider(kind)}
          </Button>
        ))}
      </div>
      {deviceAccount(v.kind) ? (
        <>
          <Field label="登录方式">
            <Select
              disabled={created}
              value={v.login_mode || "device"}
              onChange={(e) =>
                set({ ...v, login_mode: e.target.value, oauth_client_id: "" })
              }
            >
              {v.kind === "azure" ? (
                <>
                  <option value="azure-cli">
                    复用这台服务主机的 Azure CLI
                  </option>
                  <option value="device">Device Login · 独立账号</option>
                </>
              ) : (
                <>
                  <option value="device">GitHub Device Login · Copilot</option>
                  <option value="oauth-app">自定义 OAuth App</option>
                </>
              )}
            </Select>
          </Field>
          {v.login_mode === "oauth-app" && (
            <Field label="OAuth App Client ID">
              <Input
                required
                disabled={created}
                value={v.oauth_client_id || ""}
                onChange={(e) => set({ ...v, oauth_client_id: e.target.value })}
              />
            </Field>
          )}
          <p className="muted">
            {sharedAzure(v)
              ? "直接使用已有 az login 身份，由 Azure CLI 管理令牌续期；不会复制登录缓存。"
              : v.kind === "azure"
                ? "通过浏览器授权，独立保存此账号的 Azure CLI 登录状态。"
                : v.login_mode === "oauth-app"
                  ? "自动使用 GitHub 返回的 refresh token；是否提供由 OAuth App 决定。"
                  : "设备授权会持久保存，Copilot exchange token 自动续期。"}
          </p>
        </>
      ) : (
        <Field
          label="API Key"
          hint="使用 API 账号的密钥，与 ChatGPT / Claude 聊天订阅独立。"
        >
          <Input
            type="password"
            autoComplete="off"
            required
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="粘贴 API Key"
          />
        </Field>
      )}
      <details className="advanced-options">
        <summary>备注与高级设置</summary>
        <Field label="账号备注">
          <Input
            value={v.name}
            disabled={created}
            required
            onChange={(e) => set({ ...v, name: e.target.value })}
          />
        </Field>
        <Field label="内部 ID" hint="已自动生成，通常无需修改">
          <Input
            {...identifier}
            disabled={created}
            value={v.id}
            onChange={(e) => set({ ...v, id: e.target.value })}
          />
        </Field>
      </details>
      {created && (
        <p className="muted">账号已创建。若保存密钥失败，可在此重试。</p>
      )}
    </Editor>
  );
}
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
      endpoint: defaultEndpoint(kind),
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
  async function discover() {
    setDiscovering(true);
    setDiscoveryError("");
    try {
      const result = await api<ModelDiscovery>("model-discovery", "POST", {
        ...v,
        proxy_id: proxy,
      });
      setDiscovery(result);
      set({
        ...v,
        resource_id: result.resource_id,
        subscription_id: result.subscription_id || v.subscription_id,
      });
    } catch (e) {
      setDiscoveryError((e as Error).message);
    } finally {
      setDiscovering(false);
    }
  }
  return (
    <Editor
      title={original ? "编辑来源" : "添加来源"}
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
        const value = { ...v, deployments: selected };
        await save({
          ...config,
          sources: [
            ...config.sources.filter((s) => s.id !== original?.id),
            value,
          ],
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
              set({ ...v, account_id: e.target.value });
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
                set({ ...v, endpoint: e.target.value, resource_id: undefined });
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
        <Field label="网络 / Proxy">
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
export function TargetPicker({
  config,
  agent,
  value,
  onChange,
  includePools = true,
}: {
  config: Config;
  agent?: Agent;
  value: string;
  onChange: (v: string) => void;
  includePools?: boolean;
}) {
  return (
    <Select value={value} onChange={(e) => onChange(e.target.value)} required>
      <option value="">选择来源或来源池</option>
      <optgroup label="模型来源">
        {sources(config).map((s) => (
          <option key={s.id} value={s.id}>
            {s.name}
            {config.source_state.paused.includes(s.id) ? " · 已暂停" : ""}
          </option>
        ))}
      </optgroup>
      {includePools &&
        config.pools.some((p) => !agent || p.agent === agent) && (
          <optgroup label="来源池">
            {config.pools
              .filter((p) => !agent || p.agent === agent)
              .map((p) => (
                <option key={p.id} value={"pool:" + p.id}>
                  {p.name}
                </option>
              ))}
          </optgroup>
        )}
    </Select>
  );
}
export function ModelInput({
  config,
  source,
  value,
  onChange,
  id,
}: {
  config: Config;
  source: string;
  value: string;
  onChange: (v: string) => void;
  id: string;
}) {
  const pool = config.pools.find((p) => "pool:" + p.id === source);
  const [models, setModels] = useState<Model[]>([]),
    [loading, setLoading] = useState(false),
    [error, setError] = useState("");
  const [manual, setManual] = useState(false),
    [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setModels([]);
    setError("");
    setManual(false);
    if (pool) return;
    setLoading(true);
    api<{ models: Model[] }>(
      "models?source=" + encodeURIComponent(source),
      "GET",
      undefined,
      controller.signal,
    )
      .then((result) => setModels(result.models))
      .catch((e) => {
        if (!controller.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [source, revision]);
  const suggestions = [
    ...new Set([
      ...(sources(config).find((s) => s.id === source)?.deployments || []),
      ...models.map((m) => m.id),
    ]),
  ];
  useEffect(() => {
    if (!value && !pool && suggestions[0]) onChange(suggestions[0]);
  }, [value, pool, suggestions[0], onChange]);
  return (
    <>
      <div className="input-action">
        {manual ? (
          <Input
            id={id}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            required
            placeholder="模型名称"
          />
        ) : (
          <Select
            id={id}
            required
            disabled={!!pool || (loading && suggestions.length === 0)}
            value={pool?.model || value}
            onChange={(e) => onChange(e.target.value)}
          >
            <option value="">{loading ? "正在获取模型…" : "选择模型"}</option>
            {(pool
              ? [pool.model]
              : [...new Set([...suggestions, ...(value ? [value] : [])])]
            ).map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </Select>
        )}
        {!pool && (
          <Button
            type="button"
            variant="secondary"
            size="icon"
            aria-label="刷新模型列表"
            disabled={loading}
            onClick={() => setRevision((v) => v + 1)}
          >
            <RefreshCw size={15} className={loading ? "spin" : ""} />
          </Button>
        )}
      </div>
      {!pool && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setManual(!manual)}
        >
          {manual ? "返回模型列表" : "手动输入模型"}
        </Button>
      )}
      {error && (
        <small role="alert" className="text-error">
          {error}
        </small>
      )}
    </>
  );
}
export function AdvancedRoutesEditor({
  config,
  save,
  close,
  original,
}: { original: Client } & Common) {
  const [v, set] = useState<Client>(
    original
      ? structuredClone(original)
      : {
          id: generatedID("client"),
          name: suggestedName("Client", config.clients),
          disabled: false,
          routes: [
            {
              id: "route-1",
              agent: "codex",
              match_model: "*",
              source_id: "copilot",
              model: "",
            },
          ],
        },
  );
  function update(i: number, r: Route) {
    set({ ...v, routes: v.routes.map((old, j) => (j === i ? r : old)) });
  }
  function move(i: number, d: number) {
    const routes = [...v.routes];
    [routes[i], routes[i + d]] = [routes[i + d], routes[i]];
    set({ ...v, routes });
  }
  return (
    <Editor
      wide
      title="高级路由规则"
      description="每个 Client 对应一个人或调用方。路由从上到下匹配，首次匹配后停止。"
      onClose={close}
      onSave={async () => {
        if (original) {
          await save({
            ...config,
            clients: config.clients.map((c) =>
              c.id === v.id ? { ...v, legacy_direct: false } : c,
            ),
          });
          close();
        }
      }}
    >
      <div className="section-label">
        路由规则 <Badge>{v.routes.length}</Badge>
      </div>
      {v.routes.map((r, i) => (
        <div className="route-editor" key={r.id}>
          <div className="row-between">
            <strong>规则 {i + 1}</strong>
            <div className="actions">
              <Button
                type="button"
                variant="ghost"
                size="icon"
                disabled={i === 0}
                aria-label={`上移规则 ${i + 1}`}
                onClick={() => move(i, -1)}
              >
                <ArrowUp size={15} />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                disabled={i === v.routes.length - 1}
                aria-label={`下移规则 ${i + 1}`}
                onClick={() => move(i, 1)}
              >
                <ArrowDown size={15} />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`删除规则 ${i + 1}`}
                onClick={() =>
                  set({ ...v, routes: v.routes.filter((_, j) => j !== i) })
                }
              >
                <Trash2 size={15} />
              </Button>
            </div>
          </div>
          <div className="form-grid">
            <Field label="Agent">
              <Select
                value={r.agent}
                onChange={(e) =>
                  update(i, routeAgent(r, e.target.value as Agent, config))
                }
              >
                <option value="codex">Codex</option>
                <option value="claude-code">Claude Code</option>
              </Select>
            </Field>
            <Field label="请求模型别名" hint="* 匹配任意模型">
              <Input
                required
                value={r.match_model}
                onChange={(e) =>
                  update(i, { ...r, match_model: e.target.value })
                }
              />
            </Field>
            <Field label="转发到来源 / 来源池">
              <TargetPicker
                config={config}
                agent={r.agent}
                value={r.source_id}
                onChange={(target) => update(i, routeTarget(r, target, config))}
              />
            </Field>
            <Field label="使用模型">
              <ModelInput
                key={r.source_id}
                config={config}
                source={r.source_id}
                value={r.model}
                id={r.id + "-models"}
                onChange={(model) => update(i, { ...r, model })}
              />
            </Field>
          </div>
          {i > 0 &&
            v.routes
              .slice(0, i)
              .some(
                (previous) =>
                  previous.agent === r.agent &&
                  (previous.match_model === "*" ||
                    previous.match_model === r.match_model),
              ) && (
              <p className="text-warning">
                前面的规则已覆盖这个 Agent 和模型，此规则不会被匹配。
              </p>
            )}
        </div>
      ))}
      <Button
        type="button"
        variant="secondary"
        onClick={() =>
          set({
            ...v,
            routes: [
              ...v.routes,
              {
                id: "route-" + crypto.randomUUID().slice(0, 8),
                agent: "codex",
                match_model: "*",
                source_id: "copilot",
                model: "",
              },
            ],
          })
        }
      >
        <Plus size={16} />
        添加规则
      </Button>
    </Editor>
  );
}
export function PoolEditor({
  config,
  save,
  close,
  original,
}: { original?: Pool } & Common) {
  const [v, set] = useState<Pool>(
    original
      ? structuredClone(original)
      : {
          id: generatedID("pool"),
          name: suggestedName("来源池", config.pools),
          agent: "codex",
          model: "balanced",
          failover: true,
          members: [
            { id: "member-1", source_id: "copilot", model: "", weight: 100 },
          ],
        },
  );
  return (
    <Editor
      wide
      title={original ? "编辑来源池" : "添加来源池"}
      description="权重决定新会话的分配比例；同一会话保持来源固定。"
      onClose={close}
      onSave={async () => {
        await save({
          ...config,
          pools: [...config.pools.filter((p) => p.id !== v.id), v],
        });
        close();
      }}
    >
      <Identity
        id={v.id}
        name={v.name}
        existing={!!original}
        onID={(id) => set({ ...v, id })}
        onName={(name) => set({ ...v, name })}
      />
      <div className="form-grid">
        <Field label="Agent">
          <Select
            value={v.agent}
            onChange={(e) => set({ ...v, agent: e.target.value as Agent })}
          >
            <option value="codex">Codex</option>
            <option value="claude-code">Claude Code</option>
          </Select>
        </Field>
        <Field label="对外模型别名">
          <Input
            required
            value={v.model}
            onChange={(e) => set({ ...v, model: e.target.value })}
          />
        </Field>
      </div>
      {v.members.map((m, i) => (
        <div className="route-editor" key={m.id}>
          <div className="row-between">
            <strong>
              来源 {i + 1}{" "}
              <Badge tone="violet">{share(m.weight, v.members)}%</Badge>
            </strong>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`移除来源 ${i + 1}`}
              disabled={v.members.length === 1}
              onClick={() =>
                set({ ...v, members: v.members.filter((_, j) => j !== i) })
              }
            >
              <Trash2 size={15} />
            </Button>
          </div>
          <div className="member-fields">
            <Field label="来源">
              <TargetPicker
                config={config}
                includePools={false}
                value={m.source_id}
                onChange={(source_id) =>
                  set({
                    ...v,
                    members: v.members.map((old, j) =>
                      j === i ? { ...m, source_id, model: "" } : old,
                    ),
                  })
                }
              />
            </Field>
            <Field label="模型">
              <ModelInput
                key={m.source_id}
                config={config}
                source={m.source_id}
                value={m.model}
                id={m.id + "-models"}
                onChange={(model) =>
                  set({
                    ...v,
                    members: v.members.map((old, j) =>
                      j === i ? { ...m, model } : old,
                    ),
                  })
                }
              />
            </Field>
            <Field label="权重">
              <Input
                type="number"
                min="0"
                max="1000000"
                step="1"
                required
                value={m.weight}
                onChange={(e) =>
                  set({
                    ...v,
                    members: v.members.map((old, j) =>
                      j === i ? { ...m, weight: Number(e.target.value) } : old,
                    ),
                  })
                }
              />
            </Field>
          </div>
        </div>
      ))}
      <Button
        type="button"
        variant="secondary"
        onClick={() =>
          set({
            ...v,
            members: [
              ...v.members,
              {
                id: "member-" + crypto.randomUUID().slice(0, 8),
                source_id: "copilot",
                model: "",
                weight: 100,
              },
            ],
          })
        }
      >
        <Plus size={16} />
        添加来源
      </Button>
      <p className="muted">权重为 0 时停止分配新会话，已有会话仍可使用。</p>
      <Field label="来源故障时">
        <Select
          value={String(v.failover)}
          onChange={(e) => set({ ...v, failover: e.target.value === "true" })}
        >
          <option value="true">允许切换到池内其他来源</option>
          <option value="false">保留原来源并报告错误</option>
        </Select>
      </Field>
    </Editor>
  );
}
export function ProxyEditor({
  config,
  save,
  close,
  original,
}: { original?: Proxy } & Common) {
  const [v, set] = useState<Proxy>(
    original
      ? { ...original }
      : {
          id: generatedID("proxy"),
          name: suggestedName("Proxy", config.proxies.profiles),
          mode: "custom",
          address: "",
        },
  );
  return (
    <Editor
      title={original ? "编辑 Proxy" : "添加 Proxy"}
      onClose={close}
      onSave={async () => {
        await save({
          ...config,
          proxies: {
            ...config.proxies,
            profiles: [
              ...config.proxies.profiles.filter((p) => p.id !== v.id),
              v,
            ],
          },
        });
        close();
      }}
    >
      <Identity
        id={v.id}
        name={v.name}
        existing={!!original}
        onID={(id) => set({ ...v, id })}
        onName={(name) => set({ ...v, name })}
      />
      <Field label="模式">
        <Select
          value={v.mode}
          onChange={(e) => set({ ...v, mode: e.target.value })}
        >
          <option value="custom">自定义代理</option>
          <option value="direct">直连</option>
          <option value="system">跟随系统环境</option>
        </Select>
      </Field>
      {v.mode === "custom" && (
        <Field label="代理地址" hint="支持 HTTP、HTTPS、SOCKS5">
          <Input
            type="url"
            required
            value={v.address}
            onChange={(e) => set({ ...v, address: e.target.value })}
            placeholder="http://127.0.0.1:7890"
          />
        </Field>
      )}
    </Editor>
  );
}
