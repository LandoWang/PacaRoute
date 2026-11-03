import { Plus, Trash2, Pencil } from "lucide-react";
import { useApp } from "../../app/state";
import { Confirm } from "../../components/feedback";
import { Button, Badge, Panel, Empty } from "../../components/ui";
import { sourceName } from "../../domain/config";
import { share, poolModels, memberModel } from "../../domain/routing";
import { PoolEditor } from "./pool-editor";
import {
  CooldownDetails,
  PoolAvailability,
  poolAllocation,
} from "./routing-health";
export function Pools() {
  const { config, status, save, show, close } = useApp();
  const common = { config, save, close };
  return (
    <>
      <div className="page-toolbar">
        <p>状态每 5 秒刷新。冷却结束后允许重试，并不代表已验证恢复。</p>
        <Button onClick={() => show(<PoolEditor {...common} />)}>
          <Plus size={16} />
          添加来源池
        </Button>
      </div>
      <div className="section-heading">
        <div>
          <h2>来源池</h2>
          <p>按比例分配新会话，同一个 session 保持固定。</p>
        </div>
        <Badge>{config.pools.length}</Badge>
      </div>
      {config.pools.length === 0 ? (
        <Panel>
          <Empty title="还没有来源池">
            添加多个来源后，可以按自定义比例分配请求。
          </Empty>
        </Panel>
      ) : (
        <div className="cards-grid">
          {config.pools.map((p) => (
            <Panel
              key={p.id}
              title={p.name}
              description={`${p.agent} · ${p.multi_model ? `${poolModels(p).length} 个模型` : `模型别名 ${p.model}`}`}
              actions={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={"编辑池 " + p.name}
                  onClick={() => show(<PoolEditor {...common} original={p} />)}
                >
                  <Pencil size={16} />
                </Button>
              }
            >
              {poolModels(p).map((model) => (
                <div className="pool-model-group" key={model}>
                  {p.multi_model && <h3>{model}</h3>}
                  <PoolAvailability
                    id={p.id}
                    model={p.multi_model ? model : undefined}
                  />
                  <div className="pool-members">
                    {p.members
                      .filter((m) => memberModel(p, m) === model)
                      .map((m) => (
                        <div key={m.id}>
                          <div>
                            <strong>{sourceName(config, m.source_id)}</strong>
                            <small>
                              {m.model}
                              {config.source_state.paused.includes(m.source_id)
                                ? " · 已暂停"
                                : ""}
                            </small>
                            <CooldownDetails
                              source={m.source_id}
                              model={m.model}
                            />
                          </div>
                          <span className="weight">
                            {share(
                              poolAllocation(
                                config,
                                p,
                                status.routing,
                                model,
                              ).find((member) => member.id === m.id)?.weight ||
                                0,
                              poolAllocation(config, p, status.routing, model),
                            )}
                            <small>%</small>
                          </span>
                        </div>
                      ))}
                  </div>
                </div>
              ))}
              <div className="entity-footer">
                <span>
                  {p.failover
                    ? "允许故障切换；已开始的流由客户端重试"
                    : "来源故障时报告错误"}
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={"删除池 " + p.name}
                  onClick={() =>
                    show(
                      <Confirm
                        title="删除来源池？"
                        description="请先解除客户端对来源池的引用。"
                        onClose={close}
                        action={() =>
                          save({
                            ...config,
                            pools: config.pools.filter((x) => x.id !== p.id),
                          })
                        }
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
      )}
    </>
  );
}
