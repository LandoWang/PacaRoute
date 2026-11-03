import { useState } from "react";
import { api } from "../../api/transport";
import { type Client, type Agent } from "../../api/types";
import { useApp } from "../../app/state";
import { Identity } from "../../components/identity";
import { Editor, Field } from "../../components/ui";
import { generatedID, suggestedName } from "../../domain/config";
import { enabledAgents, agentNames } from "../../domain/routing";
import { QuotaFields } from "./quota";
export function ClientEditor({
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
          name: suggestedName("客户端", config.clients),
          disabled: false,
          enabled_agents: ["codex", "claude-code"],
          routes: [],
        },
  );
  return (
    <Editor
      title={original ? "编辑客户端" : "添加客户端"}
      description="一个客户端对应一个调用方。启用 Agent 后，在此客户端下选择来源或来源池。"
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
        {(["codex", "claude-code", "pacaroute"] as Agent[]).map((agent) => (
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
      <QuotaFields value={v.quota} onChange={(quota) => set({ ...v, quota })} />
      <p className="muted">
        PacaRoute 用于下游路由器，可转发其 Codex / Claude 请求。未启用的 Agent
        路径返回 404。
      </p>
      <p className="muted">
        {v.local
          ? "default 仅供本机使用，不需要密钥。删除它可关闭本机免密接入。"
          : "保存后生成专属密钥，接入脚本会自动包含它。停用 Agent 会立即拒绝它的新请求。"}
      </p>
    </Editor>
  );
}
