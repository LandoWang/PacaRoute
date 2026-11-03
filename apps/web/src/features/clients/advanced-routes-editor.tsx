import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { type Client, type Route, type Grant } from "../../api/types";
import { type Common } from "../../app/editor-props";
import {
  Button,
  Input,
  Select,
  Field,
  Editor,
  Badge,
} from "../../components/ui";
import { generatedID, suggestedName, sources } from "../../domain/config";
import { routeAgent, routeTarget } from "../../domain/routing";
import { ModelInput } from "../models/model-input";
import { TargetPicker } from "../models/target-picker";
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
                  update(i, routeAgent(r, e.target.value as Grant, config))
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
