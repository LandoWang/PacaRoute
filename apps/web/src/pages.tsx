import { BrandIcon } from "./components/brand-icon";
import { useEffect, useState, useRef, type ReactNode } from "react";
import {
  ArrowRight,
  Plus,
  Pause,
  Play,
  KeyRound,
  Network,
  Users,
  Server,
  Activity as ActivityIcon,
  Copy,
  ExternalLink,
  Trash2,
  Pencil,
  RefreshCw,
  ShieldCheck,
  Check,
} from "lucide-react";
import {
  api,
  generatedID,
  suggestedName,
  accountProvider,
  deviceAccount,
  sharedAzure,
  type ModelDiscovery,
  accounts,
  sources,
  sourceName,
  format,
  share,
  type Config,
  type Account,
  type Source,
  type Pool,
  type Proxy,
  type Usage,
  type Totals,
  type Activity,
} from "./api";
import { useApp, useLoad } from "./state";
import {
  Button,
  Badge,
  Panel,
  Empty,
  Modal,
  Editor,
  Field,
  Input,
  Select,
  ErrorNote,
} from "./components/ui";
import {
  AccountEditor,
  SourceEditor,
  PoolEditor,
  ProxyEditor,
} from "./editors";
export function Loading({
  loading,
  error,
}: {
  loading: boolean;
  error?: string;
}) {
  return (
    <>
      {loading && (
        <p className="muted loading">
          <RefreshCw className="spin" size={16} />
          加载中…
        </p>
      )}
      {error && <ErrorNote>{error}</ErrorNote>}
    </>
  );
}
export function Confirm({
  title,
  description,
  action,
  onClose,
}: {
  title: string;
  description: string;
  action: () => Promise<void>;
  onClose: () => void;
}) {
  return (
    <Editor
      title={title}
      onClose={onClose}
      onSave={async () => {
        await action();
        onClose();
      }}
      saveLabel="确认"
    >
      <p>{description}</p>
    </Editor>
  );
}
export function Secret({
  value,
  id,
  onClose,
}: {
  value: string;
  id: string;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false),
    [error, setError] = useState("");
  return (
    <Modal
      title="保存 Client 密钥"
      description="这个密钥只显示一次。请交给对应的调用方，并保存在安全的位置。"
      onClose={onClose}
    >
      <div className="form-body">
        <Badge tone="mint">{id}</Badge>
        <pre className="secret">{value}</pre>
        <Button
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(value);
              setCopied(true);
            } catch {
              setError("浏览器不允许复制，请选中密钥手动保存。");
            }
          }}
        >
          {copied ? <Check size={16} /> : <Copy size={16} />}{" "}
          {copied ? "已复制" : "复制密钥"}
        </Button>
        {error && <ErrorNote>{error}</ErrorNote>}
      </div>
      <div className="modal-footer">
        <Button variant="secondary" onClick={onClose}>
          我已保存
        </Button>
      </div>
    </Modal>
  );
}
export function Stats({ total }: { total?: Totals }) {
  return (
    <div className="stats-grid">
      {[
        ["请求数", total?.requests, "过去一段时间的模型请求"],
        ["输入 token", total?.input_tokens, "含缓存读取"],
        ["输出 token", total?.output_tokens, "模型生成内容"],
        ["缓存 token", total?.cached_tokens, "输入 token 的一部分"],
      ].map(([label, value, hint]) => (
        <div className="stat" key={String(label)}>
          <span>{label}</span>
          <strong>{format(value as number)}</strong>
          <small>{hint}</small>
        </div>
      ))}
    </div>
  );
}
export function Overview() {
  const state = useApp(),
    { config, navigate } = state;
  const usage = useLoad<Usage>("usage", 15000),
    recent = useLoad<Activity[]>("activity", 15000);
  const steps = [
    ["账号", "连接 GitHub 或 Azure", "accounts"],
    ["来源", "选择资源、模型与网络", "sources"],
    ["Client", "选择 Agent，生成接入脚本", "clients"],
    ["路由", "分配来源或来源池", "routes"],
  ];
  return (
    <>
      <div className="hero">
        <div>
          <div className="eyebrow">YOUR MODELS. YOUR ROUTES.</div>
          <h2>
            让每个 Agent，
            <br />
            找到合适的模型。
          </h2>
          <p>账号、模型来源与请求路由，在一个地方管理。</p>
          <div className="actions">
            <Button onClick={() => navigate("routes")}>
              配置路由
              <ArrowRight size={16} />
            </Button>
            <Button variant="secondary" onClick={() => navigate("sources")}>
              查看来源
            </Button>
          </div>
        </div>
        <img src="/mark.svg" alt="极光电子羊驼" />
      </div>
      <Stats total={usage.data?.total} />
      <div className="overview-split">
        <Panel
          title="连接路径"
          description="从账号开始，逐步建立你的模型网络。"
        >
          <div className="steps">
            {steps.map(([name, detail, page], i) => (
              <button
                key={page}
                className="step"
                onClick={() => navigate(page)}
              >
                <span className="step-number">0{i + 1}</span>
                <span>
                  <strong>{name}</strong>
                  <small>{detail}</small>
                </span>
                <ArrowRight size={18} />
              </button>
            ))}
          </div>
          <div className="inventory">
            <span>
              <Server size={15} />
              {sources(config).length} 个来源
            </span>
            <span>
              <Network size={15} />
              {config.pools.length} 个来源池
            </span>
            <span>
              <Users size={15} />
              {config.clients.length} 个 Client
            </span>
          </div>
        </Panel>
        <Panel
          title="最近请求"
          actions={
            <Button variant="ghost" size="sm" onClick={() => navigate("usage")}>
              查看用量
              <ArrowRight size={14} />
            </Button>
          }
        >
          <Loading loading={recent.loading} error={recent.error} />
          {recent.data?.length === 0 ? (
            <Empty title="等待第一个请求">
              连接 Agent 后，这里会显示请求动态。
            </Empty>
          ) : (
            <div className="request-list">
              {recent.data?.slice(0, 5).map((v, i) => (
                <div className="request-row" key={v.id || i}>
                  <ActivityIcon size={16} />
                  <div>
                    <strong>{v.model}</strong>
                    <small>
                      {sourceName(config, v.source)} ·{" "}
                      {new Date(v.timestamp).toLocaleTimeString()}
                    </small>
                  </div>
                  <Badge tone={v.status === 200 ? "mint" : "amber"}>
                    {v.status}
                  </Badge>
                </div>
              ))}
            </div>
          )}
        </Panel>
      </div>
      <Loading loading={false} error={usage.error} />
    </>
  );
}
function DeviceLogin({
  account,
  onClose,
}: {
  account: Account;
  onClose: () => void;
}) {
  const { reload } = useApp();
  const [device, setDevice] = useState<{
      verification_uri: string;
      user_code: string;
    }>(),
    [done, setDone] = useState(false),
    [error, setError] = useState("");
  const job = useRef<Promise<{
    done?: boolean;
    job_id: string;
    verification_uri: string;
    user_code: string;
    expires_in: number;
  }> | null>(null);
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    job.current ??= api(`accounts/${account.id}/login`, "POST", {});
    job.current
      .then(async (d) => {
        if (!active) return;
        if (d.done) {
          setDone(true);
          return;
        }
        const url = new URL(d.verification_uri);
        if (
          url.protocol !== "https:" ||
          !["github.com", "microsoft.com"].includes(url.hostname)
        )
          throw Error("验证地址无效");
        setDevice(d);
        const until = Date.now() + d.expires_in * 1000;
        const poll = async () => {
          try {
            const result = await api<{ done: boolean; error?: string }>(
              "logins/" + d.job_id,
            );
            if (!active) return;
            if (result.done) {
              if (result.error) throw Error(result.error);
              setDone(true);
              await reload();
              return;
            }
            if (Date.now() > until) throw Error("验证码已过期，请重新登录");
            timer = setTimeout(poll, 3000);
          } catch (e) {
            if (active) setError((e as Error).message);
          }
        };
        void poll();
      })
      .catch((e) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [job]);
  return (
    <Modal title={"登录 " + account.name} onClose={onClose}>
      <div className="form-body">
        {done ? (
          <div className="success-message">
            <ShieldCheck size={32} />
            <h3>登录成功</h3>
            <p>现在可以添加或使用这个账号的模型来源。</p>
          </div>
        ) : device ? (
          <>
            <p>打开验证页面，输入下方代码。</p>
            <div className="device-code">{device.user_code}</div>
            <Button asChild>
              <a
                href={device.verification_uri}
                target="_blank"
                rel="noreferrer"
              >
                打开授权页面
                <ExternalLink size={16} />
              </a>
            </Button>
            <p className="muted">
              <RefreshCw size={14} className="spin" /> 等待授权完成…
            </p>
          </>
        ) : (
          !error && <p>正在获取验证码…</p>
        )}
        {error && <ErrorNote>{error}</ErrorNote>}
      </div>
    </Modal>
  );
}
function AccountDetail({
  account,
  onClose,
}: {
  account: Account;
  onClose: () => void;
}) {
  const result = useLoad<Record<string, unknown>>(
    `accounts/${account.id}/info`,
  );
  return (
    <Modal title={account.name + " · 账号与额度"} onClose={onClose}>
      <div className="form-body">
        <Loading loading={result.loading} error={result.error} />
        {result.data && (
          <>
            <p>{String(result.data.login || "")}</p>
            <pre>
              {JSON.stringify(
                result.data.quota || result.data.subscriptions || result.data,
                null,
                2,
              )}
            </pre>
            <p className="muted">
              账号额度来自上游；本机请求与 token 统计在「用量」中查看。
            </p>
          </>
        )}
      </div>
    </Modal>
  );
}
function AddSource({ onClose }: { onClose: () => void }) {
  const { config, save, show, close, navigate } = useApp();
  return (
    <Modal
      title="添加来源"
      description="选择账号，再选择它的资源和模型。"
      fixedHeight
      onClose={onClose}
    >
      <div className="form-body account-choices">
        {accounts(config).map((a) => (
          <Button
            key={a.id}
            variant="secondary"
            onClick={() =>
              show(
                a.kind === "azure" ? (
                  <AzurePicker account={a} onClose={close} />
                ) : (
                  <SourceEditor
                    config={config}
                    save={save}
                    close={close}
                    account={a.id}
                  />
                ),
              )
            }
          >
            <BrandIcon kind={a.kind} />
            <span>
              <strong>{a.name}</strong>
              <small>
                {a.kind === "github"
                  ? "GitHub Copilot"
                  : a.kind === "azure"
                    ? "Azure Foundry / AOAI"
                    : accountProvider(a.kind) + " API"}
              </small>
            </span>
            <ArrowRight size={16} />
          </Button>
        ))}
        <Button
          variant="ghost"
          onClick={() => {
            close();
            navigate("accounts");
          }}
        >
          添加其他账号
        </Button>
      </div>
    </Modal>
  );
}
function AzurePicker({
  account,
  onClose,
}: {
  account: Account;
  onClose: () => void;
}) {
  const { config, save, show, close } = useApp();
  const subs = useLoad<{
    subscriptions: { id: string; name: string; is_default?: boolean }[];
  }>(`accounts/${account.id}/subscriptions`);
  const [subscription, setSubscription] = useState(""),
    [subscriptionFilter, setSubscriptionFilter] = useState(""),
    [resourceFilter, setResourceFilter] = useState(""),
    [resource, setResource] = useState(""),
    [resources, setResources] = useState<
      { id: string; name: string; endpoint: string }[]
    >([]),
    [models, setModels] = useState<string[]>([]),
    [selected, setSelected] = useState<string[]>([]),
    [id, setID] = useState(() => generatedID("azure")),
    [name, setName] = useState(""),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(false);
  const selectedDefault = useRef(false);
  useEffect(() => {
    if (!subs.data || selectedDefault.current) return;
    selectedDefault.current = true;
    const preferred =
      subs.data.subscriptions.find((s) => s.is_default) ||
      (subs.data.subscriptions.length === 1
        ? subs.data.subscriptions[0]
        : undefined);
    if (preferred) void loadResources(preferred.id);
  }, [subs.data]);
  async function loadResources(value: string) {
    setSubscription(value);
    setResource("");
    setModels([]);
    setResources([]);
    setResourceFilter("");
    setError("");
    if (!value) return;
    setLoading(true);
    try {
      const v = await api<{ resources: typeof resources }>(
        `accounts/${account.id}/resources?subscription=${encodeURIComponent(value)}`,
      );
      setResources(v.resources);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  async function loadModels(value: string) {
    setResource(value);
    setModels([]);
    setSelected([]);
    setError("");
    const r = resources.find((r) => r.id === value);
    if (!r) return;
    setName(suggestedName(r.name, config.sources));
    setLoading(true);
    try {
      const v = await api<{ models: { id: string }[] }>(
        `accounts/${account.id}/deployments?subscription=${encodeURIComponent(subscription)}&resource=${encodeURIComponent(value)}`,
      );
      setModels(v.models.map((m) => m.id));
      setSelected(v.models.map((m) => m.id));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  return (
    <Editor
      title="添加 Azure 来源"
      description="选择这个账号下需要使用的资源和模型。"
      fixedHeight
      saveLabel="添加来源"
      onClose={onClose}
      onSave={async () => {
        const r = resources.find((r) => r.id === resource);
        if (!r || !selected.length)
          throw Error("请选择资源和至少一个 deployment");
        await save({
          ...config,
          sources: [
            ...config.sources,
            {
              id,
              name,
              kind: "azure-openai",
              auth_mode: "azure-cli",
              account_id: account.id,
              subscription_id: subscription,
              resource_id: resource,
              endpoint: r.endpoint,
              deployments: selected,
            },
          ],
        });
        onClose();
      }}
    >
      <Loading loading={subs.loading || loading} error={subs.error || error} />
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={() =>
          show(
            <SourceEditor
              config={config}
              save={save}
              close={close}
              account={account.id}
            />,
          )
        }
      >
        已有 Endpoint？手动添加
      </Button>
      {(subs.data?.subscriptions.length || 0) > 10 && (
        <Field label="筛选订阅">
          <Input
            value={subscriptionFilter}
            onChange={(e) => setSubscriptionFilter(e.target.value)}
            placeholder="按订阅名称搜索"
          />
        </Field>
      )}
      <Field label="订阅">
        <Select
          value={subscription}
          disabled={loading}
          onChange={(e) => void loadResources(e.target.value)}
          required
        >
          <option value="">选择订阅</option>
          {subs.data?.subscriptions
            .filter(
              (s) =>
                s.id === subscription ||
                s.name.toLowerCase().includes(subscriptionFilter.toLowerCase()),
            )
            .map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
        </Select>
      </Field>
      {resources.length > 10 && (
        <Field label="筛选资源">
          <Input
            value={resourceFilter}
            onChange={(e) => setResourceFilter(e.target.value)}
            placeholder="按资源名称搜索"
          />
        </Field>
      )}
      <Field label="AOAI / Foundry 资源">
        <Select
          value={resource}
          disabled={loading || !subscription}
          onChange={(e) => void loadModels(e.target.value)}
          required
        >
          <option value="">选择资源</option>
          {resources
            .filter(
              (r) =>
                r.id === resource ||
                r.name.toLowerCase().includes(resourceFilter.toLowerCase()),
            )
            .map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
        </Select>
      </Field>
      {resource && (
        <>
          <details className="advanced-options">
            <summary>备注与高级设置</summary>
            <Field label="来源名称">
              <Input
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </Field>
            <Field label="来源 ID">
              <Input
                required
                pattern="[a-z][a-z0-9-]{0,62}"
                value={id}
                onChange={(e) => setID(e.target.value)}
              />
            </Field>
          </details>
          <Field label="模型部署">
            {!loading && models.length === 0 && !error && (
              <p className="muted">
                这个资源尚未发现 OpenAI 模型部署。可以选择其他资源，或在 Azure
                中部署模型后重试。
              </p>
            )}
            <div className="checkbox-list">
              {models.map((m) => (
                <label key={m}>
                  <input
                    type="checkbox"
                    checked={selected.includes(m)}
                    onChange={(e) =>
                      setSelected(
                        e.target.checked
                          ? [...selected, m]
                          : selected.filter((v) => v !== m),
                      )
                    }
                  />
                  {m}
                </label>
              ))}
            </div>
          </Field>
        </>
      )}
    </Editor>
  );
}
export function Accounts() {
  const app = useApp(),
    { config, show, close, save, notice } = app;
  const common = { config, save, close };
  return (
    <>
      <div className="page-toolbar">
        <p>账号保存身份，来源选择资源。一个账号可以连接多个模型来源。</p>
        <Button
          onClick={() =>
            show(
              <AccountEditor
                {...common}
                onConnect={(a) =>
                  show(<DeviceLogin account={a} onClose={close} />)
                }
              />,
            )
          }
        >
          <Plus size={16} />
          添加账号
        </Button>
      </div>
      <div className="cards-grid">
        {accounts(config).map((a) => (
          <Panel key={a.id}>
            <div className="entity-heading">
              <div
                className={
                  "entity-icon " + (a.kind === "github" ? "violet" : "mint")
                }
              >
                <BrandIcon kind={a.kind} account />
              </div>
              <div>
                <h2>{a.name}</h2>
                <small>{accountProvider(a.kind)}</small>
              </div>
              <Badge>
                {sources(config).filter((s) => s.account_id === a.id).length}{" "}
                个来源
              </Badge>
            </div>
            <div className="account-resources">
              {sources(config)
                .filter((s) => s.account_id === a.id)
                .map((s) => (
                  <div key={s.id}>
                    <BrandIcon kind={s.kind} size={16} />
                    {s.name}
                  </div>
                ))}
            </div>
            <div className="actions">
              {!deviceAccount(a.kind) && (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() =>
                    show(<AccountKey account={a} onClose={close} />)
                  }
                >
                  <KeyRound size={14} />
                  设置账号密钥
                </Button>
              )}
              {deviceAccount(a.kind) && (
                <>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() =>
                      sharedAzure(a)
                        ? notice(
                            "本机共享 Azure 身份请运行 az login；添加独立 Azure 账号可直接使用 Device Login。",
                          )
                        : show(<DeviceLogin account={a} onClose={close} />)
                    }
                  >
                    <KeyRound size={14} />
                    {sharedAzure(a) ? "复用 Azure CLI" : "Device Login"}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      show(<AccountDetail account={a} onClose={close} />)
                    }
                  >
                    {a.kind === "github" ? "账号 / 额度" : "查看账号"}
                  </Button>
                </>
              )}
              <Button
                variant="ghost"
                size="sm"
                onClick={() =>
                  show(
                    a.kind === "azure" ? (
                      <AzurePicker account={a} onClose={close} />
                    ) : (
                      <SourceEditor {...common} account={a.id} />
                    ),
                  )
                }
              >
                添加来源
              </Button>
            </div>
            <div className="entity-footer">
              <span>
                {a.kind === "github"
                  ? a.oauth_client_id
                    ? "自定义 OAuth：自动使用 GitHub 返回的 refresh token"
                    : "设备授权持久保存，Copilot exchange token 自动续期"
                  : sharedAzure(a)
                    ? "使用此服务主机上的 Azure CLI 身份"
                    : "来源可以在服务运行期间添加或移除"}
              </span>
              <div className="actions">
                {deviceAccount(a.kind) && !sharedAzure(a) && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      show(
                        <Confirm
                          title="退出账号？"
                          description="后续请求将无法使用这个账号，相关来源配置会保留。"
                          onClose={close}
                          action={async () => {
                            await api(`accounts/${a.id}/logout`, "POST", {});
                            notice("账号已退出");
                          }}
                        />,
                      )
                    }
                  >
                    退出
                  </Button>
                )}
                {!["copilot", "azure-local"].includes(a.id) && (
                  <Button
                    aria-label={"移除 " + a.name}
                    variant="ghost"
                    size="icon"
                    onClick={() =>
                      show(
                        <Confirm
                          title="移除账号？"
                          description="请先移除引用此账号的来源。这个操作只移除账号配置。"
                          onClose={close}
                          action={() =>
                            save({
                              ...config,
                              accounts: config.accounts.filter(
                                (x) => x.id !== a.id,
                              ),
                            })
                          }
                        />,
                      )
                    }
                  >
                    <Trash2 size={15} />
                  </Button>
                )}
              </div>
            </div>
          </Panel>
        ))}
      </div>
    </>
  );
}
function KeyEditor({
  source,
  onClose,
}: {
  source: Source;
  onClose: () => void;
}) {
  const [key, setKey] = useState("");
  return (
    <Editor
      title={"设置密钥 · " + source.name}
      onClose={onClose}
      onSave={async () => {
        await api("keys/" + source.id, "PUT", { key });
        setKey("");
        onClose();
      }}
    >
      <Field label="API Key" hint="保存后立即用于新请求，不需要重启">
        <Input
          type="password"
          autoComplete="off"
          required
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
      </Field>
    </Editor>
  );
}
function AccountKey({
  account,
  onClose,
}: {
  account: Account;
  onClose: () => void;
}) {
  const [key, setKey] = useState("");
  return (
    <Editor
      title={"设置账号密钥 · " + account.name}
      onClose={onClose}
      onSave={async () => {
        await api(`accounts/${account.id}/key`, "PUT", { key });
        setKey("");
        onClose();
      }}
    >
      <Field
        label="API Key"
        hint="此账号下的来源默认继承该密钥；来源可设置独立密钥覆盖"
      >
        <Input
          type="password"
          required
          autoComplete="off"
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
      </Field>
    </Editor>
  );
}
function ModelList({
  source,
  onClose,
}: {
  source: Source;
  onClose: () => void;
}) {
  const { config, save } = useApp();
  const [result, setResult] = useState<ModelDiscovery>(),
    [selected, setSelected] = useState(source.deployments || []),
    [loading, setLoading] = useState(false),
    [error, setError] = useState("");
  async function discover() {
    setLoading(true);
    setError("");
    try {
      setResult(
        await api<ModelDiscovery>(`sources/${source.id}/discover-models`),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    if (source.kind === "copilot") void discover();
  }, []);
  const content = (
    <>
      <div className="row-between">
        <p className="muted">
          {source.kind === "azure-openai"
            ? "从 Azure 资源发现已部署模型"
            : "从来源发现可用模型；目录不代表所有 Agent 能力都经过验证"}
        </p>
        <Button
          type="button"
          variant="secondary"
          size="sm"
          disabled={loading}
          onClick={() => void discover()}
        >
          <RefreshCw className={loading ? "spin" : ""} size={14} />
          重新发现
        </Button>
      </div>
      <Loading loading={loading} error={error} />
      {source.kind === "copilot" ? (
        result?.models.map((m) => (
          <div className="model-row" key={m.id}>
            <code>{m.id}</code>
            <span>{m.name}</span>
          </div>
        ))
      ) : (
        <div className="model-discovery-list">
          {[
            ...new Set([
              ...(source.deployments || []),
              ...(result?.models.map((m) => m.id) || []),
            ]),
          ].map((id) => (
            <label key={id}>
              <input
                type="checkbox"
                checked={selected.includes(id)}
                onChange={(e) =>
                  setSelected(
                    e.target.checked
                      ? [...selected, id]
                      : selected.filter((v) => v !== id),
                  )
                }
              />
              <span>{id}</span>
              {result && !result.models.some((m) => m.id === id) && (
                <Badge>本次未发现</Badge>
              )}
            </label>
          ))}
        </div>
      )}
    </>
  );
  if (source.kind === "copilot")
    return (
      <Modal title={source.name + " · 模型"} onClose={onClose}>
        <div className="form-body">{content}</div>
      </Modal>
    );
  return (
    <Editor
      title={source.name + " · 模型部署"}
      onClose={onClose}
      saveLabel="更新来源模型"
      onSave={async () => {
        if (!selected.length) throw Error("至少保留一个模型");
        await save({
          ...config,
          sources: config.sources.map((s) =>
            s.id === source.id
              ? {
                  ...s,
                  deployments: selected,
                  resource_id: result?.resource_id || s.resource_id,
                  subscription_id: result?.subscription_id || s.subscription_id,
                }
              : s,
          ),
        });
        onClose();
      }}
    >
      {content}
      <p className="muted">
        取消勾选前请先修改引用该模型的 Client、Agent
        或来源池。更新立即用于后续请求。
      </p>
    </Editor>
  );
}
export function Sources() {
  const { config, save, show, close, reload } = useApp();
  const [error, setError] = useState(""),
    [busy, setBusy] = useState("");
  const common = { config, save, close };
  async function pause(s: Source) {
    if (busy) return;
    setBusy(s.id);
    setError("");
    try {
      await api(`sources/${s.id}/pause`, "POST", {
        paused: !config.source_state.paused.includes(s.id),
      });
      await reload();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy("");
    }
  }
  return (
    <>
      <div className="page-toolbar">
        <p>只添加需要的资源和模型，每个来源独立选择 Proxy。</p>
        <div className="actions">
          <Button onClick={() => show(<AddSource onClose={close} />)}>
            <Plus size={16} />
            添加来源
          </Button>
        </div>
      </div>
      {error && <ErrorNote>{error}</ErrorNote>}
      <Panel className="source-list">
        {sources(config).map((s) => (
          <div className="source-row" key={s.id}>
            <div
              className={
                "entity-icon " + (s.kind === "copilot" ? "violet" : "mint")
              }
            >
              <BrandIcon kind={s.kind} size={26} />
            </div>
            <div className="source-main">
              <div className="title-line">
                <h3>{s.name}</h3>
                <Badge
                  tone={
                    config.source_state.paused.includes(s.id) ? "amber" : "mint"
                  }
                >
                  {config.source_state.paused.includes(s.id)
                    ? "已暂停"
                    : "可用"}
                </Badge>
              </div>
              <p>{s.endpoint || "GitHub Copilot API"}</p>
              <div className="metadata">
                <span>
                  {accounts(config).find((a) => a.id === s.account_id)?.name ||
                    s.account_id}
                </span>
                <span>
                  <label className="source-proxy">
                    Proxy{" "}
                    <Select
                      aria-label={"Proxy · " + s.name}
                      value={config.proxies.bindings[s.id] || "system"}
                      disabled={!!busy}
                      onChange={async (e) => {
                        const proxy = e.target.value;
                        setBusy(s.id);
                        setError("");
                        try {
                          await save({
                            ...config,
                            proxies: {
                              ...config.proxies,
                              bindings: {
                                ...config.proxies.bindings,
                                [s.id]: proxy,
                              },
                            },
                          });
                        } catch (e) {
                          setError((e as Error).message);
                        } finally {
                          setBusy("");
                        }
                      }}
                    >
                      <option value="system">系统</option>
                      <option value="direct">直连</option>
                      {config.proxies.profiles.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name}
                        </option>
                      ))}
                    </Select>
                  </label>
                </span>
                {s.deployments?.length > 0 && (
                  <span>{s.deployments.length} 个模型</span>
                )}
              </div>
            </div>
            <div className="actions source-actions">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => show(<ModelList source={s} onClose={close} />)}
              >
                模型
              </Button>
              {s.auth_mode === "api-key" && (
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={"设置密钥 " + s.name}
                  onClick={() => show(<KeyEditor source={s} onClose={close} />)}
                >
                  <KeyRound size={16} />
                </Button>
              )}
              <Button
                variant="secondary"
                size="icon"
                aria-label={
                  (config.source_state.paused.includes(s.id)
                    ? "恢复 "
                    : "暂停 ") + s.name
                }
                disabled={!!busy}
                onClick={() => void pause(s)}
              >
                {config.source_state.paused.includes(s.id) ? (
                  <Play size={15} />
                ) : (
                  <Pause size={15} />
                )}
              </Button>
              {s.id !== "copilot" && (
                <>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={"编辑 " + s.name}
                    onClick={() =>
                      show(<SourceEditor {...common} original={s} />)
                    }
                  >
                    <Pencil size={15} />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={"删除 " + s.name}
                    onClick={() =>
                      show(
                        <Confirm
                          title="删除来源？"
                          description="请先解除 Client 和来源池的引用。正在进行的请求会继续完成。"
                          onClose={close}
                          action={async () => {
                            const next = structuredClone(config);
                            next.sources = next.sources.filter(
                              (v) => v.id !== s.id,
                            );
                            next.source_state.paused =
                              next.source_state.paused.filter(
                                (id) => id !== s.id,
                              );
                            delete next.proxies.bindings[s.id];
                            await save(next);
                          }}
                        />,
                      )
                    }
                  >
                    <Trash2 size={15} />
                  </Button>
                </>
              )}
            </div>
          </div>
        ))}
      </Panel>
    </>
  );
}
export function Pools() {
  const { config, save, show, close } = useApp();
  const common = { config, save, close };
  return (
    <>
      <div className="page-toolbar">
        <p>新会话按权重分配，同一会话保持来源固定。</p>
        <Button onClick={() => show(<PoolEditor {...common} />)}>
          <Plus size={16} />
          添加来源池
        </Button>
      </div>
      <div className="section-heading">
        <div>
          <h2>来源池</h2>
          <p>按比例分配新会话，同一个 session 保持固定。</p>
        </div>
        <Badge>{config.pools.length}</Badge>
      </div>
      {config.pools.length === 0 ? (
        <Panel>
          <Empty title="还没有来源池">
            添加多个来源后，可以按自定义比例分配请求。
          </Empty>
        </Panel>
      ) : (
        <div className="cards-grid">
          {config.pools.map((p) => (
            <Panel
              key={p.id}
              title={p.name}
              description={`${p.agent} · 模型别名 ${p.model}`}
              actions={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={"编辑池 " + p.name}
                  onClick={() => show(<PoolEditor {...common} original={p} />)}
                >
                  <Pencil size={16} />
                </Button>
              }
            >
              <div className="pool-members">
                {p.members.map((m) => (
                  <div key={m.id}>
                    <div>
                      <strong>{sourceName(config, m.source_id)}</strong>
                      <small>{m.model}</small>
                    </div>
                    <span className="weight">
                      {share(m.weight, p.members)}
                      <small>%</small>
                    </span>
                  </div>
                ))}
              </div>
              <div className="entity-footer">
                <span>
                  {p.failover ? "允许响应开始前故障切换" : "来源故障时报告错误"}
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={"删除池 " + p.name}
                  onClick={() =>
                    show(
                      <Confirm
                        title="删除来源池？"
                        description="请先解除 Client 对来源池的引用。"
                        onClose={close}
                        action={() =>
                          save({
                            ...config,
                            pools: config.pools.filter((x) => x.id !== p.id),
                          })
                        }
                      />,
                    )
                  }
                >
                  <Trash2 size={15} />
                </Button>
              </div>
            </Panel>
          ))}
        </div>
      )}
    </>
  );
}
export function Proxies() {
  const { config, save, show, close } = useApp();
  const common = { config, save, close };
  return (
    <>
      <div className="page-toolbar">
        <p>创建一次，在多个来源之间复用。更新后新请求立即生效。</p>
        <Button onClick={() => show(<ProxyEditor {...common} />)}>
          <Plus size={16} />
          添加 Proxy
        </Button>
      </div>
      <Panel
        title="账号登录网络"
        description="用于 GitHub 登录、额度查询以及 Azure 账号操作。"
      >
        <AccountProxy config={config} save={save} />
      </Panel>
      <div className="cards-grid">
        {config.proxies.profiles.map((p) => (
          <Panel
            key={p.id}
            title={p.name}
            description={
              p.mode === "custom"
                ? p.address
                : p.mode === "direct"
                  ? "直连"
                  : "跟随系统环境"
            }
            actions={
              <Button
                variant="ghost"
                size="icon"
                aria-label={"编辑 " + p.name}
                onClick={() => show(<ProxyEditor {...common} original={p} />)}
              >
                <Pencil size={16} />
              </Button>
            }
          >
            <div className="tags">
              {Object.entries(config.proxies.bindings)
                .filter(([, id]) => id === p.id)
                .map(([id]) => (
                  <Badge key={id}>{sourceName(config, id)}</Badge>
                ))}
            </div>
            <div className="entity-footer">
              <span>
                {
                  Object.values(config.proxies.bindings).filter(
                    (id) => id === p.id,
                  ).length
                }{" "}
                个来源使用
              </span>
              <Button
                variant="ghost"
                size="icon"
                aria-label={"删除 " + p.name}
                onClick={() =>
                  show(
                    <Confirm
                      title="删除 Proxy？"
                      description="请先解除来源和账号登录对这个 Proxy 的引用。"
                      onClose={close}
                      action={async () => {
                        if (
                          Object.values(config.proxies.bindings).includes(
                            p.id,
                          ) ||
                          config.proxies.account_proxy_id === p.id
                        )
                          throw Error("这个 Proxy 仍被引用");
                        await save({
                          ...config,
                          proxies: {
                            ...config.proxies,
                            profiles: config.proxies.profiles.filter(
                              (v) => v.id !== p.id,
                            ),
                          },
                        });
                      }}
                    />,
                  )
                }
              >
                <Trash2 size={15} />
              </Button>
            </div>
          </Panel>
        ))}
      </div>
      {!config.proxies.profiles.length && (
        <Panel>
          <Empty title="还没有自定义 Proxy">
            来源仍可选择「直连」或「跟随系统环境」。
          </Empty>
        </Panel>
      )}
    </>
  );
}
function AccountProxy({
  config,
  save,
}: {
  config: Config;
  save: (c: Config) => Promise<void>;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <>
      <Select
        aria-label="账号登录 Proxy"
        value={config.proxies.account_proxy_id || "system"}
        disabled={busy}
        onChange={async (e) => {
          setBusy(true);
          setError("");
          try {
            await save({
              ...config,
              proxies: { ...config.proxies, account_proxy_id: e.target.value },
            });
          } catch (e) {
            setError((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <option value="system">跟随系统环境</option>
        <option value="direct">直连</option>
        {config.proxies.profiles.map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
      </Select>
      {error && <ErrorNote>{error}</ErrorNote>}
    </>
  );
}
export function UsagePage() {
  const { config } = useApp();
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
  const hours: [string, Totals][] = [];
  if (result.data) {
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
        result.data.timeline[key] || {
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
  const max = Math.max(
    1,
    ...hours.map(([, v]) => v.input_tokens + v.output_tokens),
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
        <Field label="Client">
          <Select value={client} onChange={(e) => setClient(e.target.value)}>
            <option value="">全部 Client</option>
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
      {!!result.data?.total.missing_usage && (
        <p className="text-warning">
          {result.data.total.missing_usage} 次请求缺少完整 token
          用量；未知用量不会按 0 估算。
        </p>
      )}
      <Panel
        title="Token 时间分布"
        description="按小时聚合 · 横轴以本地时间显示"
      >
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
                  {new Date(time).toLocaleString()} · 输入 {v.input_tokens} /
                  输出 {v.output_tokens}
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
      <Panel title="按来源统计">
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>来源</th>
                <th>请求</th>
                <th>输入 token</th>
                <th>输出 token</th>
                <th>缓存 token</th>
                <th>失败</th>
              </tr>
            </thead>
            <tbody>
              {Object.entries(result.data?.sources || {}).map(([id, v]) => (
                <tr key={id}>
                  <td>{sourceName(config, id)}</td>
                  <td>{format(v.requests)}</td>
                  <td>{format(v.input_tokens)}</td>
                  <td>{format(v.output_tokens)}</td>
                  <td>{format(v.cached_tokens)}</td>
                  <td>{v.failures}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Panel>
    </>
  );
}
export function Errors() {
  const { config } = useApp();
  const result = useLoad<Activity[]>("errors", 15000);
  return (
    <>
      <Loading loading={result.loading} error={result.error} />
      <Panel
        title="失败请求"
        description="记录来源、模型与失败原因，不记录请求正文和密钥。"
      >
        {result.data?.length === 0 ? (
          <Empty title="暂无错误">服务运行中的失败请求会显示在这里。</Empty>
        ) : (
          result.data?.map((v, i) => (
            <div className="error-row" key={v.id || i}>
              <div className="row-between">
                <strong>
                  {sourceName(config, v.source)} · {v.model}
                </strong>
                <Badge tone="amber">{v.status || "连接失败"}</Badge>
              </div>
              <pre>{v.error}</pre>
              <small>
                {new Date(v.timestamp).toLocaleString()} ·{" "}
                {v.client_id || "本机"} · {v.duration_ms} ms
              </small>
            </div>
          ))
        )}
      </Panel>
    </>
  );
}
