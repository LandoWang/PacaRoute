import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { type Pool, type Agent } from "../../api/types";
import { type Common } from "../../app/editor-props";
import { Identity } from "../../components/identity";
import {
  Button,
  Input,
  Select,
  Field,
  Editor,
  Badge,
} from "../../components/ui";
import { generatedID, suggestedName, sources } from "../../domain/config";
import { share, poolModels, memberModel } from "../../domain/routing";
import { ModelInput } from "../models/model-input";
import { TargetPicker } from "../models/target-picker";
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
