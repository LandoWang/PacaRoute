import { useState } from "react";
import { type Proxy } from "../../api/types";
import { type Common } from "../../app/editor-props";
import { Identity } from "../../components/identity";
import { Input, Select, Field, Editor } from "../../components/ui";
import { generatedID, suggestedName, upsertByID } from "../../domain/config";
export function ProxyEditor({
  config,
  save,
  close,
  original,
}: { original?: Proxy } & Common) {
  const [v, set] = useState<Proxy>(
    original
      ? { ...original }
      : {
          id: generatedID("proxy"),
          name: suggestedName("代理", config.proxies.profiles),
          mode: "custom",
          address: "",
        },
  );
  return (
    <Editor
      title={original ? "编辑代理" : "添加代理"}
      onClose={close}
      onSave={async () => {
        await save({
          ...config,
          proxies: {
            ...config.proxies,
            profiles: upsertByID(config.proxies.profiles, v, original?.id),
          },
        });
        close();
      }}
    >
      <Identity
        id={v.id}
        name={v.name}
        existing={!!original}
        onID={(id) => set({ ...v, id })}
        onName={(name) => set({ ...v, name })}
      />
      <Field label="模式">
        <Select
          value={v.mode}
          onChange={(e) => set({ ...v, mode: e.target.value })}
        >
          <option value="custom">自定义代理</option>
          <option value="direct">直连</option>
          <option value="system">跟随系统环境</option>
        </Select>
      </Field>
      {v.mode === "custom" && (
        <Field label="代理地址" hint="支持 HTTP、HTTPS、SOCKS5">
          <Input
            type="url"
            required
            value={v.address}
            onChange={(e) => set({ ...v, address: e.target.value })}
            placeholder="http://127.0.0.1:7890"
          />
        </Field>
      )}
    </Editor>
  );
}
