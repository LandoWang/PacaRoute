import { Terminal } from "lucide-react";
import { useState } from "react";
import { type Client, type Grant } from "../../api/types";
import { useEntityDraft } from "../../app/entity-draft";
import { useApp } from "../../app/state";
import { Button, Panel, Field, Select, ErrorNote } from "../../components/ui";
import { sourceName, sources } from "../../domain/config";
import { agentNames } from "../../domain/routing";
import { ModelInput } from "../models/model-input";
import { ModelMultiSelect } from "../models/model-multiselect";
import { TargetPicker } from "../models/target-picker";
import { ClientQR } from "../peers/peer-connection";
import { BackupPicker } from "../settings/backup-picker";
import { ConnectionScript } from "./connection-script-dialog";
import {
  readModelSelection,
  selectedModelRoutes,
  type ModelSelection,
} from "./model-routes";
const emptySelection: ModelSelection = {
  source: "",
  models: [],
  defaultModel: "",
  fixed: false,
};
export function RouteCard({ client, agent }: { client: Client; agent: Grant }) {
  const { config, status, save, show, close } = useApp();
  const existing = client.routes.filter((r) => r.agent === agent);
  const kind = (id: string) =>
    sources(config).find((s) => s.id === id)?.kind || "";
  const initial = readModelSelection(existing, kind(existing[0]?.source_id));
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const draft = useEntityDraft<ModelSelection>(
    existing,
    () => initial || emptySelection,
    busy,
  );
  const selection = draft.value,
    setSelection = draft.set,
    conflict = draft.conflict;
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
    if (conflict) {
      setError(
        "此 Agent 的路由已在其他位置更改。请先载入最新路由，再重新选择。",
      );
      return;
    }
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
      draft.markSaved();
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
                      fixed: !!targetPool && !targetPool.multi_model,
                    });
                  }}
                />
              </Field>
              {selection.source && (!pool || pool.multi_model) && (
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
            {conflict && (
              <Button
                variant="ghost"
                onClick={() => {
                  draft.reset();
                  setError("");
                }}
              >
                放弃修改并载入最新路由
              </Button>
            )}
          </div>
        </>
      )}
      <div className="client-connection-actions actions">
        {agent === "pacaroute" ? (
          <>
            {!client.local && (
              <Button
                variant="secondary"
                size="sm"
                disabled={client.disabled || !existing.length}
                onClick={() =>
                  show(<ClientQR client={client} onClose={close} />)
                }
              >
                接入二维码
              </Button>
            )}
            <small>
              在下游 PacaRoute
              添加账号，使用本客户端的地址与密钥；模型目录只公开这里配置的路由。
            </small>
          </>
        ) : (
          <>
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
          </>
        )}
      </div>
      {conflict && (
        <p className="text-warning">
          此 Agent
          的路由已在其他位置更改；你未保存的选择仍保留，保存前请载入最新路由。
        </p>
      )}
      {error && <ErrorNote>{error}</ErrorNote>}
    </Panel>
  );
}
