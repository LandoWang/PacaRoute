import { type Config, type Grant } from "../../api/types";
import { isProtocolAgent } from "../../domain/routing";
import { Select } from "../../components/ui";
import { sources, poolTarget } from "../../domain/config";
export function TargetPicker({
  config,
  agent,
  value,
  onChange,
  includePools = true,
}: {
  config: Config;
  agent?: Grant;
  value: string;
  onChange: (v: string) => void;
  includePools?: boolean;
}) {
  return (
    <Select value={value} onChange={(e) => onChange(e.target.value)} required>
      <option value="">选择来源或来源池</option>
      <optgroup label="模型来源">
        {sources(config).map((s) => (
          <option key={s.id} value={s.id}>
            {s.name}
            {config.source_state.paused.includes(s.id) ? " · 已暂停" : ""}
          </option>
        ))}
      </optgroup>
      {includePools &&
        config.pools.some(
          (p) => !agent || !isProtocolAgent(agent) || p.agent === agent,
        ) && (
          <optgroup label="来源池">
            {config.pools
              .filter(
                (p) => !agent || !isProtocolAgent(agent) || p.agent === agent,
              )
              .map((p) => (
                <option key={p.id} value={poolTarget(p.id)}>
                  {p.name}
                </option>
              ))}
          </optgroup>
        )}
    </Select>
  );
}
