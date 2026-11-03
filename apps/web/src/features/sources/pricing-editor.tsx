import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { type ModelPrice, type Source } from "../../api/types";
import { useApp } from "../../app/state";
import { Button, Editor, Field, Input } from "../../components/ui";
const priceFields = [
  ["input_usd", "普通输入"],
  ["cached_usd", "缓存读取"],
  ["output_usd", "输出"],
  ["cache_write_usd", "缓存写入（可选）"],
] as const;
export function PricingEditor({
  source,
  onClose,
}: {
  source: Source;
  onClose: () => void;
}) {
  const { config, save } = useApp();
  const [rows, setRows] = useState(() =>
    Object.entries(source.prices || {}).map(([model, price]) => ({
      model,
      ...price,
    })),
  );
  const update = (i: number, patch: Partial<ModelPrice> & { model?: string }) =>
    setRows((v) => v.map((row, n) => (n === i ? { ...row, ...patch } : row)));
  return (
    <Editor
      title={"模型费率 · " + source.name}
      onClose={onClose}
      onSave={async () => {
        const prices: Record<string, ModelPrice> = {};
        for (const row of rows) {
          const { model, ...price } = row;
          if (!model || prices[model]) throw Error("模型 ID 不能为空或重复");
          prices[model] = price;
        }
        await save({
          ...config,
          sources: config.sources.map((s) =>
            s.id === source.id ? { ...s, prices } : s,
          ),
        });
        onClose();
      }}
    >
      <p>
        单位：USD / 百万 token。按这个来源实际命中的模型或部署 ID
        计价；普通输入不含缓存。不会使用来源池的公开别名计价。
      </p>
      <p className="muted">
        费率由你维护。未配置的模型显示待计价；改价只影响新请求，历史记录保留当时费率。
      </p>
      {rows.map((row, i) => (
        <div className="pricing-row" key={i}>
          <div className="input-action">
            <Field label="实际模型 / 部署 ID">
              <Input
                required
                list={"prices-" + source.id}
                value={row.model}
                onChange={(e) => update(i, { model: e.target.value })}
              />
            </Field>
            <Button
              variant="ghost"
              type="button"
              aria-label={"移除费率 " + row.model}
              onClick={() => setRows(rows.filter((_, n) => n !== i))}
            >
              <Trash2 size={16} />
            </Button>
          </div>
          <div className="form-grid">
            {priceFields.map(([key, label]) => (
              <Field key={key} label={label}>
                <Input
                  required={key !== "cache_write_usd"}
                  inputMode="decimal"
                  placeholder={
                    key === "cache_write_usd" ? "无此项可留空" : "例如 2.00"
                  }
                  value={row[key] || ""}
                  onChange={(e) => update(i, { [key]: e.target.value })}
                />
              </Field>
            ))}
          </div>
        </div>
      ))}
      <datalist id={"prices-" + source.id}>
        {source.deployments.map((model) => (
          <option key={model} value={model} />
        ))}
      </datalist>
      <Button
        type="button"
        variant="secondary"
        onClick={() =>
          setRows([
            ...rows,
            { model: "", input_usd: "", cached_usd: "", output_usd: "" },
          ])
        }
      >
        <Plus size={16} />
        添加模型费率
      </Button>
    </Editor>
  );
}
