import { type Totals } from "../../api/types";
import { format } from "../../domain/format";
export function Stats({ total }: { total?: Totals }) {
  return (
    <div className="stats-grid">
      {[
        ["请求数", total?.requests, "过去一段时间的模型请求"],
        ["输入 token", total?.input_tokens, "含缓存读取"],
        ["缓存 token", total?.cached_tokens, "输入 token 中的缓存读取"],
        ["输出 token", total?.output_tokens, "模型生成内容"],
      ].map(([label, value, hint]) => (
        <div className="stat" key={String(label)}>
          <span>{label}</span>
          <strong>{format(value as number)}</strong>
          <small>{hint}</small>
        </div>
      ))}
    </div>
  );
}
