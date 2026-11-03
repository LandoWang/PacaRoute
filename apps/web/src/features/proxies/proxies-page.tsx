import { Plus, Trash2, Pencil } from "lucide-react";
import { useState } from "react";
import { type Config } from "../../api/types";
import { useApp } from "../../app/state";
import { Confirm } from "../../components/feedback";
import {
  Button,
  Badge,
  Panel,
  Empty,
  Select,
  ErrorNote,
} from "../../components/ui";
import { sourceName } from "../../domain/config";
import { ProxyEditor } from "./proxy-editor";
import { errorMessage } from "../../domain/errors";
export function Proxies() {
  const { config, save, show, close } = useApp();
  const common = { config, save, close };
  return (
    <>
      <div className="page-toolbar">
        <p>创建一次，在多个来源之间复用。更新后新请求立即生效。</p>
        <Button onClick={() => show(<ProxyEditor {...common} />)}>
          <Plus size={16} />
          添加代理
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
                      title="删除代理？"
                      description="请先解除来源和账号登录对这个代理的引用。"
                      onClose={close}
                      action={async () => {
                        if (
                          Object.values(config.proxies.bindings).includes(
                            p.id,
                          ) ||
                          config.proxies.account_proxy_id === p.id
                        )
                          throw Error("这个代理仍被引用");
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
          <Empty title="还没有自定义代理">
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
        aria-label="账号登录代理"
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
            setError(errorMessage(e));
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
