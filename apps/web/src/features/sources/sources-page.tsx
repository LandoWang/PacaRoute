import { Plus, Pause, Play, KeyRound, Trash2, Pencil } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/transport";
import { type Source } from "../../api/types";
import { useApp } from "../../app/state";
import { BrandIcon } from "../../components/brand-icon";
import { Confirm } from "../../components/feedback";
import {
  Button,
  Badge,
  Panel,
  Empty,
  Select,
  ErrorNote,
} from "../../components/ui";
import { accounts, sources } from "../../domain/config";
import { CooldownDetails } from "../pools/routing-health";
import { AddSource } from "./add-source";
import { KeyEditor } from "./key-editor";
import { ModelList } from "./model-list";
import { PricingEditor } from "./pricing-editor";
import { SourceEditor } from "./source-editor";
export function Sources() {
  const { config, status, save, show, close, reload } = useApp();
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
        <p>只添加需要的资源和模型，每个来源独立选择代理。</p>
        <div className="actions">
          <Button onClick={() => show(<AddSource onClose={close} />)}>
            <Plus size={16} />
            添加来源
          </Button>
        </div>
      </div>
      {error && <ErrorNote>{error}</ErrorNote>}
      {sources(config).length === 0 && (
        <Empty title="还没有来源">添加账号后，在这里选择资源和模型。</Empty>
      )}
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
                    config.source_state.paused.includes(s.id) ||
                    status.routing?.cooldowns.some((c) => c.source_id === s.id)
                      ? "amber"
                      : "mint"
                  }
                >
                  {config.source_state.paused.includes(s.id)
                    ? "已暂停"
                    : status.routing?.cooldowns.some(
                          (c) => c.source_id === s.id,
                        )
                      ? "模型冷却中"
                      : "已启用"}
                </Badge>
              </div>
              <p>{s.endpoint || "GitHub Copilot API"}</p>
              <CooldownDetails source={s.id} />
              <div className="metadata">
                <span>
                  {accounts(config).find((a) => a.id === s.account_id)?.name ||
                    s.account_id}
                </span>
                <span>
                  <label className="source-proxy">
                    代理{" "}
                    <Select
                      aria-label={"代理 · " + s.name}
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
                onClick={() =>
                  show(<PricingEditor source={s} onClose={close} />)
                }
              >
                费率
              </Button>
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
              {
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
                          description="请先解除客户端和来源池的引用。正在进行的请求会继续完成。"
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
              }
            </div>
          </div>
        ))}
      </Panel>
    </>
  );
}
