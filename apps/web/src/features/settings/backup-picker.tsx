import { useState } from "react";
import { api } from "../../api/transport";
import {
  type ProtocolAgent,
  type LocalBackups,
  type RestorePlan,
} from "../../api/types";
import { useLoad } from "../../app/state";
import { Loading } from "../../components/feedback";
import { Editor, Field, Select } from "../../components/ui";
export function BackupPicker({
  agent,
  onClose,
  onApplied,
}: {
  agent: ProtocolAgent;
  onClose: () => void;
  onApplied: () => void;
}) {
  const list = useLoad<LocalBackups>("local/backups?agent=" + agent);
  const [selected, setSelected] = useState(""),
    [plan, setPlan] = useState<RestorePlan>();
  return (
    <Editor
      title="恢复 Agent 配置"
      onClose={onClose}
      saveLabel={plan ? "备份当前配置并恢复" : "预览恢复"}
      onSave={async () => {
        if (!selected) throw Error("请选择备份");
        if (!plan) {
          setPlan(
            await api<RestorePlan>("local/restore", "POST", {
              agent,
              backup: selected,
            }),
          );
          return;
        }
        await api("local/restore", "POST", {
          agent,
          backup: selected,
          apply: true,
          fingerprint: plan.fingerprint,
        });
        onApplied();
        onClose();
      }}
    >
      <Loading loading={list.loading} error={list.error} />
      <Field label="备份版本">
        <Select
          required
          value={selected}
          onChange={(e) => {
            setSelected(e.target.value);
            setPlan(undefined);
          }}
        >
          <option value="">选择备份</option>
          {list.data?.backups.map((b) => (
            <option disabled={!b.valid} key={b.id} value={b.id}>
              {new Date(b.created_at).toLocaleString()}
              {b.missing_file ? " · 原配置不存在" : ""}
              {b.valid ? "" : " · 不可用"}
            </option>
          ))}
        </Select>
      </Field>
      {list.data?.backups.length === 0 && <p>没有可恢复的备份。</p>}
      {plan && (
        <ul className="preview-lines">
          {plan.changes.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      )}
    </Editor>
  );
}
