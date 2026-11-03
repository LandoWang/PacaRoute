import { useState } from "react";
import { api } from "../../api/transport";
import { type Account } from "../../api/types";
import { Editor, Field, Input } from "../../components/ui";
export function AccountKey({
  account,
  onClose,
}: {
  account: Account;
  onClose: () => void;
}) {
  const [key, setKey] = useState("");
  return (
    <Editor
      title={"设置账号密钥 · " + account.name}
      onClose={onClose}
      onSave={async () => {
        await api(`accounts/${account.id}/key`, "PUT", { key });
        setKey("");
        onClose();
      }}
    >
      <Field
        label="API Key"
        hint="此账号下的来源默认继承该密钥；来源可设置独立密钥覆盖"
      >
        <Input
          type="password"
          required
          autoComplete="off"
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
      </Field>
    </Editor>
  );
}
