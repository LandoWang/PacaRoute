import { type ClientKeyResponse } from "../../api/types";
import { Plus, Pencil, Trash2, Terminal } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/transport";
import { useApp } from "../../app/state";
import { Confirm } from "../../components/feedback";
import {
  Button,
  Badge,
  Panel,
  Empty,
  Editor,
  Input,
} from "../../components/ui";
import { enabledAgents } from "../../domain/routing";
import { ClientQR } from "../peers/client-qr";
import { AdvancedRoutesEditor } from "./advanced-routes-editor";
import { ClientEditor } from "./client-editor";
import { QuotaMeter } from "./quota";
import { RouteCard } from "./route-card";
import { Secret } from "./secret";
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
          aria-label="搜索客户端"
          placeholder="搜索客户端…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <Button
          onClick={() => show(<ClientEditor onClose={close} onKey={key} />)}
        >
          <Plus size={16} />
          添加客户端
        </Button>
      </div>
      <div className="connection-strip">
        <Terminal size={20} />
        <div>
          <strong>每个客户端，在这里配好来源和接入。</strong>
          <p>
            为每种 Agent
            选择来源或来源池，保存后生成接入脚本。后续切换来源无需重配客户端。
          </p>
        </div>
      </div>
      {list.length === 0 ? (
        <Panel>
          <Empty title={search ? "没有匹配的客户端" : "还没有客户端"}>
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
            <QuotaMeter key={c.id + config.revision} client={c} />
            <div className="client-routes">
              {enabledAgents(c).map((agent) => (
                <RouteCard key={c.id + agent} client={c} agent={agent} />
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
                {!c.local && enabledAgents(c).includes("pacaroute") && (
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() =>
                      show(<ClientQR client={c} onClose={close} />)
                    }
                  >
                    接入二维码
                  </Button>
                )}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    show(
                      <Confirm
                        title={c.disabled ? "启用客户端？" : "停用客户端？"}
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
                          title="轮换客户端密钥？"
                          saveLabel="生成新密钥"
                          onClose={close}
                          onSave={async () => {
                            const v = await api<ClientKeyResponse>(
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
                        title="删除客户端？"
                        description={
                          c.local
                            ? "将关闭本机免密访问。重启不会重新添加 default。"
                            : "这个客户端的密钥和路由将失效。"
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
