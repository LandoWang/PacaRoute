import { useEffect, useState } from "react";
import {
  ArrowRight,
  Check,
  ChevronLeft,
  MessageSquare,
  Monitor,
  RefreshCw,
  Sparkles,
} from "lucide-react";
import { api, sources, sourceName, type Agent } from "./api";
import { useApp, useLoad } from "./state";
import {
  Button,
  Badge,
  Panel,
  Empty,
  Editor,
  Modal,
  Field,
  Input,
  Select,
  ErrorNote,
} from "./components/ui";
import { ModelInput, TargetPicker } from "./editors";
import { Loading } from "./pages";
type AgentState = {
  id: Agent;
  path: string;
  connected: boolean;
  drifted: boolean;
  source: string;
  model: string;
  error?: string;
};
type Plan = {
  path: string;
  fingerprint: string;
  changes: string[];
  backup_path?: string;
};
function LocalOnly() {
  return (
    <Panel>
      <Empty title="这是本机功能">
        请从本机 PacaRoute 托盘打开管理界面。远程服务不会读取这台电脑的 Agent
        配置或聊天历史。
      </Empty>
    </Panel>
  );
}
export function Agents() {
  const { status } = useApp();
  if (!status.local_management) return <LocalOnly />;
  return <LocalAgents />;
}
function LocalAgents() {
  const state = useLoad<{ clients: AgentState[] }>("local/agents");
  return (
    <>
      <div className="connection-strip">
        <Monitor size={22} />
        <div>
          <strong>连接这台电脑上的 Agent</strong>
          <p>
            先预览，再应用；每次修改都会备份原配置。来源切换不需要重启服务。
          </p>
        </div>
      </div>
      <Loading loading={state.loading} error={state.error} />
      <div className="cards-grid">
        {state.data?.clients.map((c) => (
          <AgentCard
            key={c.id + ":" + c.source + ":" + c.model}
            client={c}
            refresh={state.refresh}
          />
        ))}
      </div>
    </>
  );
}
function AgentCard({
  client,
  refresh,
}: {
  client: AgentState;
  refresh: () => void;
}) {
  const { config, show, close, notice, status } = useApp();
  const [source, setSource] = useState(client.source || "copilot"),
    [model, setModel] = useState(client.model || ""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const pool = config.pools.find((p) => "pool:" + p.id === source);
  return (
    <Panel
      title={client.id === "codex" ? "Codex" : "Claude Code"}
      actions={
        <Badge tone={client.connected && !client.drifted ? "mint" : "neutral"}>
          {client.drifted
            ? "配置需更新"
            : client.connected
              ? "已连接"
              : "未连接"}
        </Badge>
      }
    >
      <div className="form-body compact">
        <small className="path">{client.path}</small>
        <Field label="来源 / 来源池">
          <TargetPicker
            config={config}
            agent={client.id}
            value={source}
            onChange={(v) => {
              setSource(v);
              setModel(
                config.pools.find((p) => "pool:" + p.id === v)?.model || "",
              );
            }}
          />
        </Field>
        <Field label="默认模型">
          <ModelInput
            key={source}
            config={config}
            source={source}
            value={pool?.model || model}
            id={"agent-" + client.id}
            onChange={setModel}
          />
        </Field>
        {(error || client.error) && (
          <ErrorNote>{error || client.error}</ErrorNote>
        )}
        <div className="actions">
          <Button
            disabled={busy || !model || !status.local_access}
            onClick={async () => {
              setBusy(true);
              setError("");
              const request = {
                client: client.id,
                source,
                model: pool?.model || model,
              };
              try {
                const plan = await api<Plan>(
                  "local/configure",
                  "POST",
                  request,
                );
                show(
                  <Editor
                    title="预览 Agent 配置"
                    saveLabel="备份并应用"
                    onClose={close}
                    onSave={async () => {
                      const result = await api<Plan>(
                        "local/configure",
                        "POST",
                        {
                          ...request,
                          apply: true,
                          fingerprint: plan.fingerprint,
                        },
                      );
                      close();
                      refresh();
                      notice(
                        "配置已应用，请新建 Agent 会话。原配置备份：" +
                          result.backup_path,
                      );
                    }}
                  >
                    <p className="path">{plan.path}</p>
                    <ul className="preview-lines">
                      {plan.changes.map((line) => (
                        <li key={line}>{line}</li>
                      ))}
                    </ul>
                  </Editor>,
                );
              } catch (e) {
                setError((e as Error).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            预览配置
            <ArrowRight size={15} />
          </Button>
          <Button
            variant="ghost"
            onClick={() =>
              show(
                <BackupPicker
                  agent={client.id}
                  onClose={close}
                  onApplied={refresh}
                />,
              )
            }
          >
            恢复备份
          </Button>
        </div>
        {!status.local_access && (
          <p className="text-warning">
            服务未启用本机 Agent 访问，无法应用本机连接配置。
          </p>
        )}
      </div>
    </Panel>
  );
}
function BackupPicker({
  agent,
  onClose,
  onApplied,
}: {
  agent: Agent;
  onClose: () => void;
  onApplied: () => void;
}) {
  const list = useLoad<{
    backups: {
      id: string;
      created_at: string;
      missing_file: boolean;
      valid: boolean;
    }[];
  }>("local/backups?agent=" + agent);
  const [selected, setSelected] = useState(""),
    [plan, setPlan] = useState<Plan>();
  return (
    <Editor
      title="恢复 Agent 配置"
      onClose={onClose}
      saveLabel={plan ? "备份当前配置并恢复" : "预览恢复"}
      onSave={async () => {
        if (!selected) throw Error("请选择备份");
        if (!plan) {
          setPlan(
            await api<Plan>("local/restore", "POST", {
              agent,
              backup: selected,
            }),
          );
          return;
        }
        await api("local/restore", "POST", {
          agent,
          backup: selected,
          apply: true,
          fingerprint: plan.fingerprint,
        });
        onApplied();
        onClose();
      }}
    >
      <Loading loading={list.loading} error={list.error} />
      <Field label="备份版本">
        <Select
          required
          value={selected}
          onChange={(e) => {
            setSelected(e.target.value);
            setPlan(undefined);
          }}
        >
          <option value="">选择备份</option>
          {list.data?.backups.map((b) => (
            <option disabled={!b.valid} key={b.id} value={b.id}>
              {new Date(b.created_at).toLocaleString()}
              {b.missing_file ? " · 原配置不存在" : ""}
              {b.valid ? "" : " · 不可用"}
            </option>
          ))}
        </Select>
      </Field>
      {list.data?.backups.length === 0 && <p>没有可恢复的备份。</p>}
      {plan && (
        <ul className="preview-lines">
          {plan.changes.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      )}
    </Editor>
  );
}
type Session = {
  id: string;
  agent: Agent;
  title: string;
  cwd: string;
  provider: string;
  model: string;
  updated: number;
  archived: boolean;
};
type Item = {
  id: string;
  kind: string;
  title: string;
  text: string;
  timestamp?: string;
};
export function Sessions() {
  const { status } = useApp();
  if (!status.local_management) return <LocalOnly />;
  return <LocalSessions />;
}
function LocalSessions() {
  const [agent, setAgent] = useState<Agent>("codex"),
    [search, setSearch] = useState(""),
    [query, setQuery] = useState(""),
    [offset, setOffset] = useState(0),
    [selected, setSelected] = useState<Session>();
  const list = useLoad<{
    sessions: Session[];
    has_more: boolean;
    notice: string;
    root: string;
  }>(
    `local/sessions?agent=${agent}&offset=${offset}&q=${encodeURIComponent(query)}`,
  );
  useEffect(() => {
    const timer = setTimeout(() => {
      setQuery(search);
      setOffset(0);
      setSelected(undefined);
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);
  return (
    <>
      <div className="page-toolbar">
        <div className="actions">
          <Select
            aria-label="会话 Agent"
            value={agent}
            onChange={(e) => {
              setAgent(e.target.value as Agent);
              setOffset(0);
              setSelected(undefined);
            }}
          >
            <option value="codex">Codex</option>
            <option value="claude-code">Claude Code</option>
          </Select>
          <Input
            aria-label="搜索会话"
            className="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="搜索标题或工作目录…"
          />
        </div>
        <Button variant="secondary" onClick={list.refresh}>
          <RefreshCw size={15} />
          刷新
        </Button>
      </div>
      <p className="muted">
        按需读取本机历史，默认不会发送到模型。记录内容不等于模型此刻的完整上下文。
      </p>
      <Loading loading={list.loading} error={list.error} />
      {list.data?.notice && <p className="muted">{list.data.notice}</p>}
      <div className="sessions-layout">
        <Panel className="session-list">
          {list.data?.sessions.length === 0 && (
            <Empty title="没有找到会话">
              在这个 Agent 中创建会话后，再刷新列表。
            </Empty>
          )}
          {list.data?.sessions.map((s) => (
            <button
              key={s.id}
              className={
                "session-row " + (selected?.id === s.id ? "selected" : "")
              }
              onClick={() => setSelected(s)}
            >
              <MessageSquare size={17} />
              <div>
                <strong>{s.title || "未命名会话"}</strong>
                <small>{s.cwd}</small>
                <span>
                  {new Date(s.updated * 1000).toLocaleDateString()} ·{" "}
                  {s.provider || s.agent}
                  {s.archived ? " · 已归档" : ""}
                </span>
              </div>
            </button>
          ))}
          <div className="pagination">
            <Button
              variant="ghost"
              size="sm"
              disabled={offset === 0 || list.loading}
              onClick={() => {
                setOffset(Math.max(0, offset - 80));
                setSelected(undefined);
              }}
            >
              上一页
            </Button>
            <span>{Math.floor(offset / 80) + 1}</span>
            <Button
              variant="ghost"
              size="sm"
              disabled={!list.data?.has_more || list.loading}
              onClick={() => {
                setOffset(offset + 80);
                setSelected(undefined);
              }}
            >
              下一页
            </Button>
          </div>
        </Panel>
        {selected ? (
          <SessionHistory key={selected.id} session={selected} />
        ) : (
          <Panel>
            <Empty title="选择一个会话">
              查看用户消息、模型回复与工具执行记录。
            </Empty>
          </Panel>
        )}
      </div>
    </>
  );
}
function SessionHistory({ session }: { session: Session }) {
  const { config, show, close } = useApp();
  const [cursor, setCursor] = useState(""),
    [items, setItems] = useState<Item[]>([]),
    [older, setOlder] = useState("");
  const page = useLoad<{ items: Item[]; older: string; notice?: string }>(
    `local/session?agent=${session.agent}&id=${encodeURIComponent(session.id)}&before=${encodeURIComponent(cursor)}`,
  );
  useEffect(() => {
    if (page.data) {
      setItems((previous) => {
        const seen = new Set<string>();
        return [...page.data!.items, ...(cursor ? previous : [])].filter(
          (item) => !seen.has(item.id) && !!seen.add(item.id),
        );
      });
      setOlder(page.data.older);
    }
  }, [page.data]);
  return (
    <Panel
      className="history-panel"
      title={session.title || "未命名会话"}
      description={session.cwd}
      actions={
        <Button
          variant="secondary"
          size="sm"
          disabled={!items.length}
          onClick={() =>
            show(
              <AnalyzeSession
                items={items}
                title={session.title}
                onClose={close}
              />,
            )
          }
        >
          <Sparkles size={14} />
          理解会话
        </Button>
      }
    >
      <Loading loading={page.loading} error={page.error} />
      {older && (
        <Button
          variant="ghost"
          className="older-button"
          disabled={page.loading}
          onClick={() => setCursor(older)}
        >
          加载更早历史
        </Button>
      )}
      {page.data?.notice && <p className="muted">{page.data.notice}</p>}
      <div className="history-items">
        {items.map((item) => (
          <article
            key={item.id}
            className={"history-item history-" + item.kind}
          >
            <div>
              <Badge
                tone={
                  item.kind === "user"
                    ? "mint"
                    : item.kind === "assistant"
                      ? "violet"
                      : item.kind === "error"
                        ? "amber"
                        : "neutral"
                }
              >
                {item.title}
              </Badge>
              {item.timestamp && (
                <small>{new Date(item.timestamp).toLocaleTimeString()}</small>
              )}
            </div>
            {["tool", "result", "context"].includes(item.kind) ? (
              <details>
                <summary>
                  {item.text.split("\n")[0].slice(0, 100) || "查看记录"}
                </summary>
                <pre>{item.text}</pre>
              </details>
            ) : (
              <pre>{item.text}</pre>
            )}
          </article>
        ))}
      </div>
    </Panel>
  );
}
function AnalyzeSession({
  items,
  title,
  onClose,
}: {
  items: Item[];
  title: string;
  onClose: () => void;
}) {
  const { config } = useApp();
  const [source, setSource] = useState("copilot"),
    [model, setModel] = useState(""),
    [question, setQuestion] = useState(
      "这个会话在做什么？已经完成了什么，接下来该做什么？",
    ),
    [result, setResult] = useState("");
  return (
    <Editor
      wide
      title="理解会话"
      description="仅在点击分析后，将当前已加载的历史发送到你选择的模型来源。"
      saveLabel="发送所选历史并分析"
      onClose={onClose}
      onSave={async () => {
        const v = await api<{ text: string }>("local/analyze", "POST", {
          source,
          model,
          question,
          title,
          items,
        });
        setResult(v.text);
      }}
    >
      <div className="form-grid">
        <Field label="分析来源">
          <TargetPicker
            config={config}
            agent="codex"
            value={source}
            onChange={(v) => {
              setSource(v);
              setModel(
                config.pools.find((p) => "pool:" + p.id === v)?.model || "",
              );
            }}
          />
        </Field>
        <Field label="分析模型">
          <ModelInput
            key={source}
            config={config}
            source={source}
            value={model}
            onChange={setModel}
            id="analysis-model"
          />
        </Field>
      </div>
      <Field label="希望了解什么">
        <Input
          required
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
        />
      </Field>
      <p className="muted">
        当前加载 {items.length} 条记录，最多发送约 48,000
        个字符。历史中的文字作为待分析资料处理。
      </p>
      {result && (
        <div className="analysis-result">
          <Badge tone="mint">
            {sourceName(config, source)} · {model}
          </Badge>
          <pre>{result}</pre>
        </div>
      )}
    </Editor>
  );
}
export function Settings() {
  const { status, reload, notice } = useApp();
  const [port, setPort] = useState(String(status.port || 8766)),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <>
      <Panel
        title="PacaRoute 服务"
        description="前端通过 HTTP API 连接 Go 服务。发布版内嵌网页，无需安装 Node。"
      >
        <dl className="details-grid">
          <div>
            <dt>版本</dt>
            <dd>{status.version}</dd>
          </div>
          <div>
            <dt>服务地址</dt>
            <dd>{status.gateway_url || location.origin}</dd>
          </div>
          <div>
            <dt>运行时间</dt>
            <dd>{Math.floor(status.uptime_seconds / 60)} 分钟</dd>
          </div>
          <div>
            <dt>管理模式</dt>
            <dd>
              {status.desktop
                ? "本机桌面托盘"
                : status.local_management
                  ? "本机 CLI"
                  : "服务管理"}
            </dd>
          </div>
        </dl>
      </Panel>
      {status.desktop ? (
        <Panel
          title="本机服务设置"
          description="修改端口会创建新监听地址；原端口上的请求会有最多 30 秒完成。"
        >
          <div className="form-body compact">
            <Field label="监听端口">
              <Input
                type="number"
                min="1024"
                max="65535"
                value={port}
                onChange={(e) => setPort(e.target.value)}
              />
            </Field>
            <Button
              disabled={busy || Number(port) === status.port}
              onClick={async () => {
                setBusy(true);
                setError("");
                try {
                  const result = await api<{ url: string }>(
                    "local/service",
                    "PUT",
                    { port: Number(port) },
                  );
                  location.assign(result.url);
                } catch (e) {
                  setError((e as Error).message);
                  setBusy(false);
                }
              }}
            >
              应用端口并重新连接
            </Button>
            <label className="switch-row">
              <div>
                <strong>登录系统时启动</strong>
                <small>在 Mac 顶栏或 Windows 系统托盘中运行</small>
              </div>
              <input
                type="checkbox"
                checked={status.startup}
                disabled={busy}
                onChange={async (e) => {
                  setBusy(true);
                  setError("");
                  try {
                    await api("local/startup", "PUT", {
                      enabled: e.target.checked,
                    });
                    await reload();
                  } catch (e) {
                    setError((e as Error).message);
                  } finally {
                    setBusy(false);
                  }
                }}
              />
            </label>
            {error && <ErrorNote>{error}</ErrorNote>}
          </div>
        </Panel>
      ) : (
        <Panel title="启动方式">
          <pre>
            pacaroute desktop{`\n`}pacaroute serve --port 8766{`\n`}pacaroute ui
            --port 8766
          </pre>
          <p className="muted">
            CLI 服务通过启动参数设置监听地址和端口；账号、来源、Proxy 和 Client
            路由都支持热更新。
          </p>
        </Panel>
      )}
      <Panel title="本机访问">
        <p>
          {status.local_access
            ? "已允许本机 Agent 使用本地路由。网络调用方仍需独立 Client 密钥。"
            : "所有 Agent 请求均需独立 Client 密钥。"}
        </p>
        <p className="muted">
          {status.local_management
            ? "本机配置与历史读取已启用，仅限本地访问。"
            : "本机文件功能未开放。桌面模式会自动启用，CLI 可使用 --local-management。"}
        </p>
      </Panel>
    </>
  );
}
