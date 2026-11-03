import { PeerScan } from "./peer-connection";
import {
  normalizePeerEndpoint,
  type PeerConnection,
} from "./peer-connection-data";
import { BrandIcon } from "./components/brand-icon";
import { useState, useEffect, useRef } from "react";
import { ArrowDown, ArrowUp, Plus, Trash2, RefreshCw } from "lucide-react";
import {
  api,
  peerCapabilities,
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
  poolModels,
  memberModel,
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
  const [scan, setScan] = useState(false);
  const [key, setKey] = useState("");
  const [created, setCreated] = useState(false);
  if (scan)
    return (
      <PeerScan
        onClose={() => setScan(false)}
        onRead={(connection: PeerConnection) => {
          set({
            id: v.id,
            kind: "pacaroute",
            name: suggestedName(
              connection.name || "PacaRoute",
              accounts(config),
            ),
            endpoint: connection.endpoint,
          });
          setKey(connection.client_key);
          setScan(false);
        }}
      />
    );
  return (
    <Editor
      title="添加账号"
      onClose={close}
      saveLabel={
        deviceAccount(v.kind) && !sharedAzure(v) ? "继续登录" : "添加账号"
      }
      onSave={async () => {
        if (!created) {
          const value =
            v.kind === "pacaroute"
              ? { ...v, endpoint: normalizePeerEndpoint(v.endpoint || "") }
              : v;
          await save({ ...config, accounts: [...config.accounts, value] });
          setCreated(true);
        }
        if (!deviceAccount(v.kind)) {
          await api(`accounts/${v.id}/key`, "PUT", { key });
          setKey("");
        }
        close();
        if (
          (deviceAccount(v.kind) && !sharedAzure(v)) ||
          v.kind === "pacaroute"
        )
          onConnect(v);
      }}
    >
      <div className="provider-choices" role="group" aria-label="账号类型">
        {["github", "openai", "anthropic", "azure", "pacaroute"].map((kind) => (
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
      {v.kind === "pacaroute" && (
        <>
          <Button
            type="button"
            variant="secondary"
            disabled={created}
            onClick={() => setScan(true)}
          >
            扫描二维码 / 粘贴接入信息
          </Button>
          <Field
            label="PacaRoute 服务地址"
            hint="对方提供的完整地址，可包含部署路径。"
          >
            <Input
              type="url"
              required
              disabled={created}
              value={v.endpoint || ""}
              placeholder="https://router.example.com"
              onChange={(e) => set({ ...v, endpoint: e.target.value })}
            />
          </Field>
        </>
      )}
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
          label={v.kind === "pacaroute" ? "客户端密钥" : "API Key"}
          hint={
            v.kind === "pacaroute"
              ? "使用对方分配的客户端密钥；只发现这个客户端可访问的模型。"
              : "使用 API 账号的密钥，与 ChatGPT / Claude 聊天订阅独立。"
          }
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
        endpoint: result.endpoint || v.endpoint,
        agent_models:
          v.kind === "pacaroute"
            ? peerCapabilities(result.models)
            : v.agent_models,
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
        config.pools.some(
          (p) => !agent || agent === "pacaroute" || p.agent === agent,
        ) && (
          <optgroup label="来源池">
            {config.pools
              .filter(
                (p) => !agent || agent === "pacaroute" || p.agent === agent,
              )
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
    if (pool || !source) {
      setLoading(false);
      return;
    }
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
            disabled={loading && suggestions.length === 0 && !pool}
            value={value || pool?.model || ""}
            onChange={(e) => onChange(e.target.value)}
          >
            <option value="">{loading ? "正在获取模型…" : "选择模型"}</option>
            {(pool
              ? poolModels(pool)
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
          name: suggestedName("客户端", config.clients),
          disabled: false,
          routes: [
            {
              id: "route-1",
              agent: "codex",
              match_model: "*",
              source_id: sources(config)[0]?.id || "",
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
      description="每个客户端对应一个人或调用方。路由从上到下匹配，首次匹配后停止。"
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
                <option value="pacaroute">PacaRoute</option>
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
                source_id: sources(config)[0]?.id || "",
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
            {
              id: "member-1",
              source_id: sources(config)[0]?.id || "",
              model: "",
              weight: 100,
            },
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
          pools: [
            ...config.pools.filter((p) => p.id !== v.id),
            {
              ...v,
              model:
                v.multi_model && !poolModels(v).includes(v.model)
                  ? poolModels(v)[0] || ""
                  : v.model,
            },
          ],
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
        <Field label={v.multi_model ? "默认接入模型" : "对外模型别名"}>
          {v.multi_model ? (
            <Select
              value={
                poolModels(v).includes(v.model)
                  ? v.model
                  : poolModels(v)[0] || ""
              }
              onChange={(e) => set({ ...v, model: e.target.value })}
            >
              {poolModels(v)
                .filter(Boolean)
                .map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
            </Select>
          ) : (
            <Input
              required
              value={v.model}
              onChange={(e) => set({ ...v, model: e.target.value })}
            />
          )}
        </Field>
      </div>
      <Field
        label="来源池模式"
        hint="多模型池像一个大来源。客户端选择模型后，仅在同模型组内分流、固定会话和故障切换。"
      >
        <Select
          value={v.multi_model ? "multiple" : "single"}
          onChange={(e) =>
            set({ ...v, multi_model: e.target.value === "multiple" })
          }
        >
          <option value="single">单模型别名 · 兼容现有来源池</option>
          <option value="multiple">多模型 · 作为一个大来源</option>
        </Select>
      </Field>
      {v.members.map((m, i) => (
        <div className="route-editor" key={m.id}>
          <div className="row-between">
            <strong>
              来源 {i + 1}{" "}
              <Badge tone="violet">
                {share(
                  m.weight,
                  v.members.filter(
                    (other) => memberModel(v, other) === memberModel(v, m),
                  ),
                )}
                %
              </Badge>
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
          {v.multi_model && (
            <Field
              label="对外模型名"
              hint="留空使用实际模型名称；相同名称的成员组成一个负载均衡组。"
            >
              <Input
                value={m.alias || ""}
                placeholder={m.model || "例如 gpt-6-astra"}
                onChange={(e) =>
                  set({
                    ...v,
                    members: v.members.map((old, j) =>
                      j === i ? { ...m, alias: e.target.value } : old,
                    ),
                  })
                }
              />
            </Field>
          )}
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
                source_id: sources(config)[0]?.id || "",
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
          name: suggestedName("代理", config.proxies.profiles),
          mode: "custom",
          address: "",
        },
  );
  return (
    <Editor
      title={original ? "编辑代理" : "添加代理"}
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
