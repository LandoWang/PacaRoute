import { RefreshCw } from "lucide-react";
import { useState, useEffect } from "react";
import { api } from "../../api/transport";
import { type Config, type Model, type ModelCatalog } from "../../api/types";
import { Button, Input, Select } from "../../components/ui";
import { sources, targetPool } from "../../domain/config";
import { poolModels } from "../../domain/routing";
export function ModelInput({
  config,
  source,
  value,
  onChange,
  id,
}: {
  config: Config;
  source: string;
  value: string;
  onChange: (v: string) => void;
  id: string;
}) {
  const pool = targetPool(config, source);
  const [models, setModels] = useState<Model[]>([]),
    [loading, setLoading] = useState(false),
    [error, setError] = useState("");
  const [manual, setManual] = useState(false),
    [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setModels([]);
    setError("");
    setManual(false);
    if (pool || !source) {
      setLoading(false);
      return;
    }
    setLoading(true);
    api<ModelCatalog>(
      "models?source=" + encodeURIComponent(source),
      "GET",
      undefined,
      controller.signal,
    )
      .then((result) => setModels(result.models))
      .catch((e) => {
        if (!controller.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [source, revision]);
  const suggestions = [
    ...new Set([
      ...(sources(config).find((s) => s.id === source)?.deployments || []),
      ...models.map((m) => m.id),
    ]),
  ];
  useEffect(() => {
    if (!value && !pool && suggestions[0]) onChange(suggestions[0]);
  }, [value, pool, suggestions[0], onChange]);
  return (
    <>
      <div className="input-action">
        {manual ? (
          <Input
            id={id}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            required
            placeholder="模型名称"
          />
        ) : (
          <Select
            id={id}
            required
            disabled={loading && suggestions.length === 0 && !pool}
            value={value || pool?.model || ""}
            onChange={(e) => onChange(e.target.value)}
          >
            <option value="">{loading ? "正在获取模型…" : "选择模型"}</option>
            {(pool
              ? poolModels(pool)
              : [...new Set([...suggestions, ...(value ? [value] : [])])]
            ).map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </Select>
        )}
        {!pool && (
          <Button
            type="button"
            variant="secondary"
            size="icon"
            aria-label="刷新模型列表"
            disabled={loading}
            onClick={() => setRevision((v) => v + 1)}
          >
            <RefreshCw size={15} className={loading ? "spin" : ""} />
          </Button>
        )}
      </div>
      {!pool && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setManual(!manual)}
        >
          {manual ? "返回模型列表" : "手动输入模型"}
        </Button>
      )}
      {error && (
        <small role="alert" className="text-error">
          {error}
        </small>
      )}
    </>
  );
}
