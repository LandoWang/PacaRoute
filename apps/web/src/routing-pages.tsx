import { useState } from "react";
import {
  Plus,
  Pencil,
  Trash2,
  ArrowRight,
  Terminal,
  Download,
  Copy,
} from "lucide-react";
import {
  api,
  type Client,
  type Agent,
  type Route,
  enabledAgents,
  agentNames,
  generatedID,
  suggestedName,
  sourceName,
  routeTarget,
} from "./api";
import { useApp } from "./state";
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
import {
  Identity,
  TargetPicker,
  ModelInput,
  AdvancedRoutesEditor,
} from "./editors";
import { Confirm, Secret } from "./pages";
import { BackupPicker } from "./local-pages";
import { connectionScript, type ScriptPlatform } from "./connection-script";

function ClientEditor({
  original,
  onClose,
  onKey,
}: {
  original?: Client;
  onClose: () => void;
  onKey: (key: string, id: string) => void;
}) {
  const { config, save } = useApp();
  const [v, set] = useState<Client>(
    original
      ? structuredClone(original)
      : {
          id: generatedID("client"),
          name: suggestedName("Client", config.clients),
          disabled: false,
          enabled_agents: ["codex", "claude-code"],
          routes: [],
        },
  );
  return (
    <Editor
      title={original ? "编辑 Client" : "添加 Client"}
      description="一个 Client 对应一个调用方。先启用 Agent，再到路由页分配来源。"
      onClose={onClose}
      onSave={async () => {
        if (original) {
          await save({
            ...config,
            clients: config.clients.map((c) => (c.id === v.id ? v : c)),
          });
          onClose();
        } else {
          const result = await api<{ api_key: string }>("clients", "POST", v);
          onKey(result.api_key, v.id);
        }
      }}
    >
      <Identity
        id={v.id}
        name={v.name}
        existing={!!original}
        onID={(id) => set({ ...v, id })}
        onName={(name) => set({ ...v, name })}
      />
      <Field label="启用的 Agent">
        {(["codex", "claude-code"] as Agent[]).map((agent) => (
          <label className="switch-row" key={agent}>
            <span>{agentNames[agent]}</span>
            <input
              type="checkbox"
              checked={enabledAgents(v).includes(agent)}
              onChange={(e) =>
                set({
                  ...v,
                  enabled_agents: e.target.checked
                    ? [...enabledAgents(v), agent]
                    : enabledAgents(v).filter((a) => a !== agent),
                })
              }
            />
          </label>
        ))}
      </Field>
      <p className="muted">
        {v.local
          ? "default 仅供本机使用，不需要密钥。删除它可关闭本机免密接入。"
          : "保存后生成专属密钥，仅展示一次。停用 Agent 会立即拒绝它的新请求。"}
      </p>
    </Editor>
  );
}
export function Clients() {
  const { config, status, save, show, close, reload, navigate } = useApp();
  const [search, setSearch] = useState("");
  const key = (value: string, id: string) => {
    show(<Secret value={value} id={id} onClose={close} />);
    void reload();
  };
  const list = config.clients.filter((c) =>
    (c.name + " " + c.id).toLowerCase().includes(search.toLowerCase()),
  );
  return (
    <>
      <div className="page-toolbar">
        <Input
          className="search"
          aria-label="搜索 Client"
          placeholder="搜索 Client…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <Button
          onClick={() => show(<ClientEditor onClose={close} onKey={key} />)}
        >
          <Plus size={16} />
          添加 Client
        </Button>
      </div>
      <div className="connection-strip">
        <Terminal size={20} />
        <div>
          <strong>Client × Agent，一份接入脚本。</strong>
          <p>
            先配置路由，再生成脚本交给调用方执行。后续切换来源无需重配客户端。
          </p>
        </div>
      </div>
      {list.length === 0 ? (
        <Panel>
          <Empty title={search ? "没有匹配的 Client" : "还没有 Client"}>
            添加调用方并分配独立密钥。本机 default 被删除后不会自动恢复。
          </Empty>
        </Panel>
      ) : (
        list.map((c) => (
          <Panel
            key={c.id}
            title={
              <>
                {c.name}
                <Badge tone={c.disabled ? "amber" : "mint"}>
                  {c.disabled ? "已停用" : c.local ? "本机免密" : "远程"}
                </Badge>
              </>
            }
            description={c.id}
            actions={
              <Button
                variant="secondary"
                size="sm"
                onClick={() =>
                  show(
                    <ClientEditor original={c} onClose={close} onKey={key} />,
                  )
                }
              >
                <Pencil size={14} />
                编辑
              </Button>
            }
          >
            {c.legacy_direct && (
              <p className="text-warning">
                正在沿用旧版直连配置。请在路由页为 default 设置固定路由。
              </p>
            )}
            {c.local && !status.local_access && (
              <p className="text-warning">
                服务未开启本机免密接入；请使用桌面模式或 --allow-local。
              </p>
            )}
            <div className="client-agent-list">
              {enabledAgents(c).map((agent) => {
                const route = c.routes.find((r) => r.agent === agent);
                return (
                  <div className="client-agent-row" key={agent}>
                    <div>
                      <strong>{agentNames[agent]}</strong>
                      <small>
                        {route
                          ? sourceName(config, route.source_id) +
                            " · " +
                            route.model
                          : "尚未分配路由"}
                      </small>
                    </div>
                    <div className="actions">
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={
                          c.disabled ||
                          !route ||
                          (!!c.local && !status.local_access)
                        }
                        onClick={() =>
                          show(
                            <ConnectionScript
                              client={c}
                              agent={agent}
                              onClose={close}
                            />,
                          )
                        }
                      >
                        <Terminal size={14} />
                        接入脚本
                      </Button>
                      {c.local && status.local_management && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() =>
                            show(
                              <BackupPicker
                                agent={agent}
                                onClose={close}
                                onApplied={() => {}}
                              />,
                            )
                          }
                        >
                          恢复配置
                        </Button>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
            {enabledAgents(c).length === 0 && (
              <p className="muted">没有启用的 Agent；所有请求都会被拒绝。</p>
            )}
            <div className="entity-footer">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  navigate("routes");
                  location.hash = "/routes?client=" + encodeURIComponent(c.id);
                }}
              >
                配置路由
                <ArrowRight size={14} />
              </Button>
              <div className="actions">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    show(
                      <Confirm
                        title={c.disabled ? "启用 Client？" : "停用 Client？"}
                        description="新请求立即生效，进行中的请求会继续完成。"
                        onClose={close}
                        action={() =>
                          save({
                            ...config,
                            clients: config.clients.map((v) =>
                              v.id === c.id
                                ? { ...v, disabled: !v.disabled }
                                : v,
                            ),
                          })
                        }
                      />,
                    )
                  }
                >
                  {c.disabled ? "启用" : "停用"}
                </Button>
                {!c.local && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      show(
                        <Editor
                          title="轮换 Client 密钥？"
                          saveLabel="生成新密钥"
                          onClose={close}
                          onSave={async () => {
                            const v = await api<{ api_key: string }>(
                              `clients/${c.id}/rotate`,
                              "POST",
                              {},
                            );
                            key(v.api_key, c.id);
                          }}
                        >
                          <p>旧密钥会立即失效，需要重新执行接入脚本。</p>
                        </Editor>,
                      )
                    }
                  >
                    轮换密钥
                  </Button>
                )}
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={"删除 " + c.name}
                  onClick={() =>
                    show(
                      <Confirm
                        title="删除 Client？"
                        description={
                          c.local
                            ? "将关闭本机免密访问。重启不会重新添加 default。"
                            : "这个 Client 的密钥和路由将失效。"
                        }
                        onClose={close}
                        action={() =>
                          save({
                            ...config,
                            clients: config.clients.filter(
                              (v) => v.id !== c.id,
                            ),
                          })
                        }
                      />,
                    )
                  }
                >
                  <Trash2 size={15} />
                </Button>
              </div>
            </div>
          </Panel>
        ))
      )}
    </>
  );
}
function ConnectionScript({
  client,
  agent,
  onClose,
}: {
  client: Client;
  agent: Agent;
  onClose: () => void;
}) {
  const { status } = useApp();
  const [platform, setPlatform] = useState<ScriptPlatform>("shell"),
    [address, setAddress] = useState(status.gateway_url || location.origin),
    [copied, setCopied] = useState(false),
    [copyError, setCopyError] = useState("");
  const matches = client.routes.filter((r) => r.agent === agent),
    [routeID, setRouteID] = useState(matches[0]?.id || "");
  const route = matches.find((r) => r.id === routeID);
  const model =
    route?.match_model === "*" ? route.model : route?.match_model || "";
  let script = "",
    error = "";
  try {
    script = connectionScript(client, agent, address, model, platform);
  } catch (e) {
    error = (e as Error).message;
  }
  const filename = `pacaroute-${client.id}-${agent}.${platform === "shell" ? "sh" : "ps1"}`;
  return (
    <Modal
      title={`${client.name} · ${agentNames[agent]}`}
      onClose={onClose}
      wide
    >
      <div className="form-body">
        <p>
          在调用方电脑执行。需要 PacaRoute CLI
          0.8+；不需要启动本地服务。脚本会备份并修改 Agent
          配置，密钥仅在执行时输入。
        </p>
        <div className="form-grid">
          <Field label="系统">
            <Select
              value={platform}
              onChange={(e) => setPlatform(e.target.value as ScriptPlatform)}
            >
              <option value="shell">macOS / Linux · Bash</option>
              <option value="powershell">Windows · PowerShell</option>
            </Select>
          </Field>
          <Field
            label="服务地址"
            hint={
              client.local
                ? "default 仅连接本机"
                : "填写调用方可访问的地址；跨网络建议使用 HTTPS"
            }
          >
            <Input
              type="url"
              value={address}
              onChange={(e) => setAddress(e.target.value)}
            />
          </Field>
        </div>
        {matches.length > 1 && (
          <Field label="接入模型别名">
            <Select
              value={routeID}
              onChange={(e) => setRouteID(e.target.value)}
            >
              {matches.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.match_model === "*" ? r.model : r.match_model}
                </option>
              ))}
            </Select>
          </Field>
        )}
        {error && <ErrorNote>{error}</ErrorNote>}
        {copyError && <ErrorNote>{copyError}</ErrorNote>}
        <pre className="script-preview">{script}</pre>
        <div className="actions">
          <Button
            disabled={!script}
            onClick={() => {
              const blob = new Blob([script], {
                type: "text/plain;charset=utf-8",
              });
              const url = URL.createObjectURL(blob);
              const a = document.createElement("a");
              a.href = url;
              a.download = filename;
              a.click();
              setTimeout(() => URL.revokeObjectURL(url), 1000);
            }}
          >
            <Download size={15} />
            下载脚本
          </Button>
          <Button
            disabled={!script}
            variant="secondary"
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(script);
                setCopied(true);
              } catch {
                setCopyError("无法访问剪贴板，请下载脚本。");
              }
            }}
          >
            <Copy size={15} />
            {copied ? "已复制" : "复制"}
          </Button>
        </div>
        <small className="muted">
          执行：
          {platform === "shell"
            ? `bash ${filename}`
            : `powershell -File .\\${filename}`}
          。执行后重新打开 Agent。
        </small>
      </div>
    </Modal>
  );
}
export function Routes() {
  const { config, save, show, close } = useApp();
  const requested = new URLSearchParams(location.hash.split("?")[1] || "").get(
    "client",
  );
  const [selected, setSelected] = useState(
    requested || config.clients[0]?.id || "",
  );
  const client =
    config.clients.find((c) => c.id === selected) || config.clients[0];
  return (
    <>
      <div className="page-toolbar">
        <p>每个 Client 的每种 Agent，连接一个来源或来源池。</p>
        {client && (
          <Select
            aria-label="选择 Client"
            value={client.id}
            onChange={(e) => setSelected(e.target.value)}
          >
            {config.clients.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name} · {c.id}
              </option>
            ))}
          </Select>
        )}
      </div>
      {!client ? (
        <Panel>
          <Empty title="先添加 Client">
            Client 决定谁能访问，以及可以使用哪些 Agent。
          </Empty>
        </Panel>
      ) : (
        <>
          {client.disabled && (
            <p className="text-warning">
              此 Client 已停用，路由保存后仍不会接受请求。
            </p>
          )}
          {client.legacy_direct && (
            <Panel title="旧版本机接入">
              <p>
                目前沿用旧配置中的来源和模型。保存路由后只允许这里配置的目标；请重新生成接入脚本。
              </p>
            </Panel>
          )}
          {enabledAgents(client).map((agent) => (
            <RouteCard
              key={client.id + agent + config.revision}
              client={client}
              agent={agent}
            />
          ))}
          {enabledAgents(client).length === 0 && (
            <Panel>
              <Empty title="没有启用的 Agent">
                请到 Client 页面选择 Codex 或 Claude Code。
              </Empty>
            </Panel>
          )}
          <Button
            variant="ghost"
            onClick={() =>
              show(
                <AdvancedRoutesEditor
                  config={config}
                  save={save}
                  close={close}
                  original={client}
                />,
              )
            }
          >
            高级：按请求模型匹配多个规则
          </Button>
        </>
      )}
    </>
  );
}
function RouteCard({ client, agent }: { client: Client; agent: Agent }) {
  const { config, save } = useApp();
  const existing = client.routes.filter((r) => r.agent === agent);
  const [route, setRoute] = useState<Route>(
    existing[0] || {
      id: generatedID("route"),
      agent,
      match_model: "*",
      source_id: "",
      model: "",
    },
  );
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const complex =
    existing.length > 1 || existing.some((r) => r.match_model !== "*");
  async function persist(remove = false) {
    setBusy(true);
    setError("");
    try {
      await save({
        ...config,
        clients: config.clients.map((c) =>
          c.id === client.id
            ? {
                ...c,
                legacy_direct: false,
                routes: [
                  ...c.routes.filter((r) => r.agent !== agent),
                  ...(remove ? [] : [route]),
                ],
              }
            : c,
        ),
      });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Panel
      title={agentNames[agent]}
      description={
        existing.length
          ? "已配置路由，新请求实时生效"
          : client.legacy_direct
            ? "沿用旧版直连；保存后启用此路由"
            : "尚未配置，请求会被拒绝"
      }
    >
      {complex ? (
        <>
          <p>
            此 Agent 有 {existing.length}{" "}
            条按模型匹配的规则，请使用下方高级编辑器管理。
          </p>
          {existing.map((r) => (
            <p key={r.id}>
              <code>{r.match_model}</code> → {sourceName(config, r.source_id)} ·{" "}
              {r.model}
            </p>
          ))}
        </>
      ) : (
        <>
          <div className="form-grid">
            <Field label="来源 / 来源池">
              <TargetPicker
                config={config}
                agent={agent}
                value={route.source_id}
                onChange={(target) =>
                  setRoute(routeTarget(route, target, config))
                }
              />
            </Field>
            <Field label="模型">
              {route.source_id ? (
                <ModelInput
                  config={config}
                  source={route.source_id}
                  value={route.model}
                  id={route.id}
                  onChange={(model) => setRoute((r) => ({ ...r, model }))}
                />
              ) : (
                <Input disabled placeholder="先选择来源" />
              )}
            </Field>
          </div>
          <div className="actions">
            <Button
              disabled={busy || !route.source_id || !route.model}
              onClick={() => void persist()}
            >
              保存路由
            </Button>
            {existing.length > 0 && (
              <Button
                disabled={busy}
                variant="ghost"
                onClick={() => void persist(true)}
              >
                清除路由
              </Button>
            )}
          </div>
        </>
      )}
      {error && <ErrorNote>{error}</ErrorNote>}
    </Panel>
  );
}
