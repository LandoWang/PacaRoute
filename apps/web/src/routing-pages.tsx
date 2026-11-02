import { useEffect, useRef, useState } from "react";
import { Plus, Pencil, Trash2, Terminal, Download, Copy } from "lucide-react";
import {
  api,
  type Client,
  type Agent,
  enabledAgents,
  agentNames,
  generatedID,
  suggestedName,
  sourceName,
  sources,
} from "./api";
import { useApp } from "./state";
import {
  readModelSelection,
  selectedModelRoutes,
  connectionModelRoutes,
  type ModelSelection,
} from "./model-routes";
import { ModelMultiSelect } from "./model-multiselect";
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
      description="一个 Client 对应一个调用方。启用 Agent 后，在此 Client 下选择来源或来源池。"
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
          : "保存后生成专属密钥，接入脚本会自动包含它。停用 Agent 会立即拒绝它的新请求。"}
      </p>
    </Editor>
  );
}
export function Clients() {
  const { config, status, save, show, close, reload } = useApp();
  const [search, setSearch] = useState(
    () =>
      new URLSearchParams(location.hash.split("?")[1] || "").get("client") ||
      "",
  );
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
          <strong>每个 Client，在这里配好来源和接入。</strong>
          <p>
            为每种 Agent
            选择来源或来源池，保存后生成接入脚本。后续切换来源无需重配客户端。
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
                正在沿用旧版直连配置。请在下方为 default 选择来源和模型。
              </p>
            )}
            {c.local && !status.local_access && (
              <p className="text-warning">
                服务未开启本机免密接入；请使用桌面模式或 --allow-local。
              </p>
            )}
            <div className="client-routes">
              {enabledAgents(c).map((agent) => (
                <RouteCard
                  key={c.id + agent + config.revision}
                  client={c}
                  agent={agent}
                />
              ))}
            </div>
            {enabledAgents(c).length === 0 && (
              <p className="muted">没有启用的 Agent；所有请求都会被拒绝。</p>
            )}
            <div className="entity-footer">
              <Button
                variant="ghost"
                size="sm"
                onClick={() =>
                  show(
                    <AdvancedRoutesEditor
                      config={config}
                      save={save}
                      close={close}
                      original={c}
                    />,
                  )
                }
              >
                高级模型规则
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
  const { status, config } = useApp();
  const [platform, setPlatform] = useState<ScriptPlatform>("shell"),
    [address, setAddress] = useState(status.gateway_url || location.origin),
    [copied, setCopied] = useState(false),
    [copyError, setCopyError] = useState("");
  const [apiKey, setApiKey] = useState(""),
    [keyLoading, setKeyLoading] = useState(!client.local),
    [keyError, setKeyError] = useState(""),
    [importKey, setImportKey] = useState("");
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    if (!client.local) {
      api<{ api_key: string }>(`clients/${client.id}/key`)
        .then((v) => {
          if (active.current) setApiKey(v.api_key);
        })
        .catch((e) => {
          if (active.current) setKeyError(e.message);
        })
        .finally(() => {
          if (active.current) setKeyLoading(false);
        });
    }
    return () => {
      active.current = false;
    };
  }, [client.id, client.local]);
  const agentRoutes = client.routes.filter((r) => r.agent === agent);
  const kind =
    sources(config).find((s) => s.id === agentRoutes[0]?.source_id)?.kind || "";
  const matches = connectionModelRoutes(agentRoutes, kind),
    [routeID, setRouteID] = useState(matches[0]?.id || "");
  const route = matches.find((r) => r.id === routeID);
  const model =
    route?.match_model === "*" ? route.model : route?.match_model || "";
  let script = "",
    preview = "",
    error = "";
  try {
    if (client.local || apiKey) {
      script = connectionScript(
        client,
        agent,
        address,
        model,
        platform,
        apiKey,
      );
      preview = connectionScript(
        client,
        agent,
        address,
        model,
        platform,
        "<Client Key 已隐藏>",
      );
    }
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
          0.8+；不需要启动本地服务。脚本会备份并修改 Agent 配置。
          {client.local
            ? "本机 default 无需 Key。"
            : "脚本自动包含此 Client 的 Key，预览已隐藏；请只交给对应调用方。"}
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
        {keyLoading && <p className="muted">正在加载 Client Key…</p>}
        {keyError && <ErrorNote>{keyError}</ErrorNote>}
        {!client.local && !keyLoading && !apiKey && (
          <div className="form-body">
            <p className="muted">
              旧 Client 只保存了 Key 的摘要。粘贴现有 Key
              一次，以后脚本会自动包含它；如果已经遗失，可关闭此窗口，在 Client
              下轮换密钥。
            </p>
            <Field label="现有 Client Key">
              <Input
                type="password"
                autoComplete="off"
                value={importKey}
                onChange={(e) => setImportKey(e.target.value)}
              />
            </Field>
            <Button
              disabled={!importKey}
              variant="secondary"
              onClick={async () => {
                setKeyError("");
                try {
                  await api(`clients/${client.id}/key`, "PUT", {
                    key: importKey,
                  });
                  if (active.current) {
                    setApiKey(importKey);
                    setImportKey("");
                  }
                } catch (e) {
                  if (active.current) setKeyError((e as Error).message);
                }
              }}
            >
              验证并保存 Key
            </Button>
          </div>
        )}
        {error && <ErrorNote>{error}</ErrorNote>}
        {copyError && <ErrorNote>{copyError}</ErrorNote>}
        {preview && <pre className="script-preview">{preview}</pre>}
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
function RouteCard({ client, agent }: { client: Client; agent: Agent }) {
  const { config, status, save, show, close } = useApp();
  const existing = client.routes.filter((r) => r.agent === agent);
  const kind = (id: string) =>
    sources(config).find((s) => s.id === id)?.kind || "";
  const initial = readModelSelection(existing, kind(existing[0]?.source_id));
  const [selection, setSelection] = useState<ModelSelection>(
    initial || {
      source: "",
      models: [],
      defaultModel: "",
      fixed: false,
    },
  );
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const complex = initial === null;
  const pool = config.pools.find((p) => "pool:" + p.id === selection.source);
  function selectModels(models: string[]) {
    setSelection((v) => ({
      ...v,
      models,
      defaultModel: models.includes(v.defaultModel)
        ? v.defaultModel
        : models[0] || "",
    }));
  }
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
                  ...(remove
                    ? []
                    : selectedModelRoutes(
                        agent,
                        kind(selection.source),
                        selection,
                        existing,
                      )),
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
          <div className="route-selection">
            <div className="form-grid">
              <Field label="来源 / 来源池">
                <TargetPicker
                  config={config}
                  agent={agent}
                  value={selection.source}
                  onChange={(source) => {
                    const targetPool = config.pools.find(
                      (p) => "pool:" + p.id === source,
                    );
                    setSelection({
                      source,
                      models: targetPool ? [targetPool.model] : [],
                      defaultModel: targetPool?.model || "",
                      fixed: !!targetPool,
                    });
                  }}
                />
              </Field>
              {selection.source && !pool && (
                <Field label="模型选择方式">
                  <Select
                    value={selection.fixed ? "fixed" : "multiple"}
                    onChange={(e) =>
                      setSelection((v) => ({
                        ...v,
                        fixed: e.target.value === "fixed",
                      }))
                    }
                  >
                    <option value="multiple">客户端选模型 · 多选</option>
                    <option value="fixed">固定为一个模型</option>
                  </Select>
                </Field>
              )}
            </div>
            {selection.source &&
              (selection.fixed ? (
                <>
                  <Field label={pool ? "来源池模型" : "固定模型"}>
                    <ModelInput
                      config={config}
                      source={selection.source}
                      value={selection.defaultModel}
                      id={client.id + agent}
                      onChange={(model) =>
                        setSelection((v) => ({
                          ...v,
                          models: [model],
                          defaultModel: model,
                        }))
                      }
                    />
                  </Field>
                  <small className="muted">
                    所有请求使用此模型，包括 Agent 中选择的其他模型。
                  </small>
                </>
              ) : (
                <ModelMultiSelect
                  key={selection.source}
                  config={config}
                  source={selection.source}
                  agent={agent}
                  selected={selection.models}
                  defaultModel={selection.defaultModel}
                  onChange={selectModels}
                  onDefault={(defaultModel) =>
                    setSelection((v) => ({ ...v, defaultModel }))
                  }
                />
              ))}
          </div>
          <div className="actions">
            <Button
              disabled={
                busy ||
                !selection.source ||
                !selection.defaultModel ||
                !selection.models.length
              }
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
      <div className="client-connection-actions actions">
        <Button
          variant="secondary"
          size="sm"
          disabled={
            client.disabled ||
            !existing.length ||
            (!!client.local && !status.local_access)
          }
          onClick={() =>
            show(
              <ConnectionScript
                client={client}
                agent={agent}
                onClose={close}
              />,
            )
          }
        >
          <Terminal size={14} />
          接入脚本
        </Button>
        {client.local && status.local_management && (
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
        <small>脚本使用已保存的配置</small>
      </div>
      {error && <ErrorNote>{error}</ErrorNote>}
    </Panel>
  );
}
