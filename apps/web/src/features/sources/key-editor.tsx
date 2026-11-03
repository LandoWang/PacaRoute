import { useState } from "react";
import { api } from "../../api/transport";
import { type Source } from "../../api/types";
import { Editor, Field, Input } from "../../components/ui";
export function KeyEditor({
  source,
  onClose,
}: {
  source: Source;
  onClose: () => void;
}) {
  const [key, setKey] = useState("");
  return (
    <Editor
      title={"设置密钥 · " + source.name}
      onClose={onClose}
      onSave={async () => {
        await api("keys/" + source.id, "PUT", { key });
        setKey("");
        onClose();
      }}
    >
      <Field label="API Key" hint="保存后立即用于新请求，不需要重启">
        <Input
          type="password"
          autoComplete="off"
          required
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
      </Field>
    </Editor>
  );
}
