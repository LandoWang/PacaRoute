import {
  ArrowRight,
  Network,
  Users,
  Server,
  Activity as ActivityIcon,
} from "lucide-react";
import { type Usage, type Activity } from "../../api/types";
import { useApp, useLoad } from "../../app/state";
import pacaRouteMark from "../../assets/pacaroute-mark.png";
import { Loading } from "../../components/feedback";
import { Button, Badge, Panel, Empty } from "../../components/ui";
import { sources, sourceName } from "../../domain/config";
import { Stats } from "../usage/stats";
import { POLL_INTERVAL_MS } from "../../app/polling";
export function Overview() {
  const state = useApp(),
    { config, navigate } = state;
  const usage = useLoad<Usage>("usage", POLL_INTERVAL_MS),
    recent = useLoad<Activity[]>("activity", POLL_INTERVAL_MS);
  const steps = [
    ["账号", "连接 GitHub 或 Azure", "accounts"],
    ["来源", "选择资源、模型与网络", "sources"],
    ["客户端", "为 Agent 分配来源，生成接入脚本", "clients"],
  ];
  return (
    <>
      <div className="hero">
        <div>
          <div className="eyebrow">YOUR MODELS. YOUR ROUTES.</div>
          <h2>
            让每个 Agent，
            <br />
            找到合适的模型。
          </h2>
          <p>账号、模型来源与请求路由，在一个地方管理。</p>
          <div className="actions">
            <Button onClick={() => navigate("clients")}>
              配置客户端
              <ArrowRight size={16} />
            </Button>
            <Button variant="secondary" onClick={() => navigate("sources")}>
              查看来源
            </Button>
            <Button variant="secondary" onClick={() => navigate("settings")}>
              导入配置
            </Button>
          </div>
        </div>
        <img src={pacaRouteMark} alt="戴霓虹眼镜的 PacaRoute 羊驼" />
      </div>
      <Stats total={usage.data?.total} />
      <div className="overview-split">
        <Panel
          title="连接路径"
          description="从账号开始，逐步建立你的模型网络。"
        >
          <div className="steps">
            {steps.map(([name, detail, page], i) => (
              <button
                key={page}
                className="step"
                onClick={() => navigate(page)}
              >
                <span className="step-number">0{i + 1}</span>
                <span>
                  <strong>{name}</strong>
                  <small>{detail}</small>
                </span>
                <ArrowRight size={18} />
              </button>
            ))}
          </div>
          <div className="inventory">
            <span>
              <Server size={15} />
              {sources(config).length} 个来源
            </span>
            <span>
              <Network size={15} />
              {config.pools.length} 个来源池
            </span>
            <span>
              <Users size={15} />
              {config.clients.length} 个客户端
            </span>
          </div>
        </Panel>
        <Panel
          title="最近请求"
          actions={
            <Button variant="ghost" size="sm" onClick={() => navigate("usage")}>
              查看用量
              <ArrowRight size={14} />
            </Button>
          }
        >
          <Loading loading={recent.loading} error={recent.error} />
          {recent.data?.length === 0 ? (
            <Empty title="等待第一个请求">
              连接 Agent 后，这里会显示请求动态。
            </Empty>
          ) : (
            <div className="request-list">
              {recent.data?.slice(0, 5).map((v, i) => (
                <div className="request-row" key={v.id || i}>
                  <ActivityIcon size={16} />
                  <div>
                    <strong>{v.model}</strong>
                    <small>
                      {sourceName(config, v.source)} ·{" "}
                      {new Date(v.timestamp).toLocaleTimeString()}
                    </small>
                  </div>
                  <Badge tone={v.status === 200 ? "mint" : "amber"}>
                    {v.status}
                  </Badge>
                </div>
              ))}
            </div>
          )}
        </Panel>
      </div>
      <Loading loading={false} error={usage.error} />
    </>
  );
}
