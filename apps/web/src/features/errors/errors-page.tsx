import { type Activity } from "../../api/types";
import { useApp, useLoad } from "../../app/state";
import { Loading } from "../../components/feedback";
import { Badge, Panel, Empty } from "../../components/ui";
import { sourceName } from "../../domain/config";
export function Errors() {
  const { config } = useApp();
  const result = useLoad<Activity[]>("errors", 15000);
  return (
    <>
      <Loading loading={result.loading} error={result.error} />
      <Panel
        title="失败请求"
        description="记录来源、模型与失败原因，不记录请求正文和密钥。"
      >
        {result.data?.length === 0 ? (
          <Empty title="暂无错误">服务运行中的失败请求会显示在这里。</Empty>
        ) : (
          result.data?.map((v, i) => (
            <div className="error-row" key={v.id || i}>
              <div className="row-between">
                <strong>
                  {sourceName(config, v.source)} · {v.model}
                </strong>
                <Badge tone="amber">{v.status || "连接失败"}</Badge>
              </div>
              <pre>{v.error}</pre>
              <small>
                {new Date(v.timestamp).toLocaleString()} ·{" "}
                {v.client_id || "本机"} · {v.duration_ms} ms
              </small>
            </div>
          ))
        )}
      </Panel>
    </>
  );
}
