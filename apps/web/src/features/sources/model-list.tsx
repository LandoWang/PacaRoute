import { RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "../../api/transport";
import { type ModelDiscovery, type Source } from "../../api/types";
import { useApp, useDraftBase } from "../../app/state";
import { Loading } from "../../components/feedback";
import { Button, Badge, Modal, Editor } from "../../components/ui";
import { peerCapabilities } from "../../domain/accounts";
export function ModelList({
  source,
  onClose,
}: {
  source: Source;
  onClose: () => void;
}) {
  const { save } = useApp(),
    config = useDraftBase();
  const [result, setResult] = useState<ModelDiscovery>(),
    [selected, setSelected] = useState(source.deployments || []),
    [loading, setLoading] = useState(false),
    [error, setError] = useState("");
  async function discover() {
    setLoading(true);
    setError("");
    try {
      setResult(
        await api<ModelDiscovery>(`sources/${source.id}/discover-models`),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    if (source.kind === "copilot" || source.kind === "pacaroute")
      void discover();
  }, []);
  const content = (
    <>
      <div className="row-between">
        <p className="muted">
          {source.kind === "azure-openai"
            ? "从 Azure 资源发现已部署模型"
            : "从来源发现可用模型；目录不代表所有 Agent 能力都经过验证"}
        </p>
        <Button
          type="button"
          variant="secondary"
          size="sm"
          disabled={loading}
          onClick={() => void discover()}
        >
          <RefreshCw className={loading ? "spin" : ""} size={14} />
          重新发现
        </Button>
      </div>
      <Loading loading={loading} error={error} />
      {source.kind === "copilot" ? (
        result?.models.map((m) => (
          <div className="model-row" key={m.id}>
            <code>{m.id}</code>
            <span>{m.name}</span>
          </div>
        ))
      ) : (
        <div className="model-discovery-list">
          {[
            ...new Set([
              ...(source.deployments || []),
              ...(result?.models.map((m) => m.id) || []),
            ]),
          ].map((id) => (
            <label key={id}>
              <input
                type="checkbox"
                checked={selected.includes(id)}
                onChange={(e) =>
                  setSelected(
                    e.target.checked
                      ? [...selected, id]
                      : selected.filter((v) => v !== id),
                  )
                }
              />
              <span>{id}</span>
              {result && !result.models.some((m) => m.id === id) && (
                <Badge>本次未发现</Badge>
              )}
            </label>
          ))}
        </div>
      )}
    </>
  );
  if (source.kind === "copilot")
    return (
      <Modal title={source.name + " · 模型"} onClose={onClose}>
        <div className="form-body">{content}</div>
      </Modal>
    );
  return (
    <Editor
      title={source.name + " · 模型部署"}
      onClose={onClose}
      saveLabel="更新来源模型"
      onSave={async () => {
        if (!selected.length) throw Error("至少保留一个模型");
        await save({
          ...config,
          sources: config.sources.map((s) =>
            s.id === source.id
              ? {
                  ...s,
                  deployments: selected,
                  endpoint: result?.endpoint || s.endpoint,
                  agent_models:
                    s.kind === "pacaroute" && result
                      ? peerCapabilities(result.models)
                      : s.agent_models,
                  resource_id: result?.resource_id || s.resource_id,
                  subscription_id: result?.subscription_id || s.subscription_id,
                }
              : s,
          ),
        });
        onClose();
      }}
    >
      {content}
      <p className="muted">
        取消勾选前请先修改引用该模型的客户端、Agent
        或来源池。更新立即用于后续请求。
      </p>
    </Editor>
  );
}
