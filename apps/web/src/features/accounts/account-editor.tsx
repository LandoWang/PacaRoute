import { useState } from "react";
import { api } from "../../api/transport";
import {
  type Account,
  type AccountKind,
  type LoginMode,
} from "../../api/types";
import { type Common } from "../../app/editor-props";
import { BrandIcon } from "../../components/brand-icon";
import { identifier } from "../../components/identity";
import { Button, Input, Select, Field, Editor } from "../../components/ui";
import {
  accountProvider,
  deviceAccount,
  sharedAzure,
} from "../../domain/accounts";
import { generatedID, suggestedName, accounts } from "../../domain/config";
import { PeerScan } from "../peers/peer-scan";
import {
  normalizePeerEndpoint,
  type PeerConnection,
} from "../peers/peer-connection-data";
export function AccountEditor({
  config,
  save,
  close,
  onConnect,
}: Common & { onConnect: (account: Account) => void }) {
  const [v, set] = useState<Account>(() => ({
    id: generatedID("account"),
    name: suggestedName("GitHub", accounts(config)),
    kind: "github",
  }));
  const [scan, setScan] = useState(false);
  const [key, setKey] = useState("");
  const [created, setCreated] = useState(false);
  if (scan)
    return (
      <PeerScan
        onClose={() => setScan(false)}
        onRead={(connection: PeerConnection) => {
          set({
            id: v.id,
            kind: "pacaroute",
            name: suggestedName(
              connection.name || "PacaRoute",
              accounts(config),
            ),
            endpoint: connection.endpoint,
          });
          setKey(connection.client_key);
          setScan(false);
        }}
      />
    );
  return (
    <Editor
      title="添加账号"
      onClose={close}
      saveLabel={
        deviceAccount(v.kind) && !sharedAzure(v) ? "继续登录" : "添加账号"
      }
      onSave={async () => {
        if (!created) {
          const value =
            v.kind === "pacaroute"
              ? { ...v, endpoint: normalizePeerEndpoint(v.endpoint || "") }
              : v;
          await save({ ...config, accounts: [...config.accounts, value] });
          setCreated(true);
        }
        if (!deviceAccount(v.kind)) {
          await api(`accounts/${v.id}/key`, "PUT", { key });
          setKey("");
        }
        close();
        if (
          (deviceAccount(v.kind) && !sharedAzure(v)) ||
          v.kind === "pacaroute"
        )
          onConnect(v);
      }}
    >
      <div className="provider-choices" role="group" aria-label="账号类型">
        {(
          [
            "github",
            "openai",
            "anthropic",
            "azure",
            "pacaroute",
          ] as AccountKind[]
        ).map((kind) => (
          <Button
            key={kind}
            type="button"
            variant={v.kind === kind ? "default" : "secondary"}
            disabled={created}
            aria-pressed={v.kind === kind}
            onClick={() => {
              set({
                ...v,
                kind,
                login_mode:
                  kind === "azure"
                    ? "azure-cli"
                    : kind === "github"
                      ? "device"
                      : "",
                oauth_client_id: "",
                name: suggestedName(accountProvider(kind), accounts(config)),
              });
              setKey("");
            }}
          >
            <BrandIcon kind={kind} account size={20} />
            {accountProvider(kind)}
          </Button>
        ))}
      </div>
      {v.kind === "pacaroute" && (
        <>
          <Button
            type="button"
            variant="secondary"
            disabled={created}
            onClick={() => setScan(true)}
          >
            扫描二维码 / 粘贴接入信息
          </Button>
          <Field
            label="PacaRoute 服务地址"
            hint="对方提供的完整地址，可包含部署路径。"
          >
            <Input
              type="url"
              required
              disabled={created}
              value={v.endpoint || ""}
              placeholder="https://router.example.com"
              onChange={(e) => set({ ...v, endpoint: e.target.value })}
            />
          </Field>
        </>
      )}
      {deviceAccount(v.kind) ? (
        <>
          <Field label="登录方式">
            <Select
              disabled={created}
              value={v.login_mode || "device"}
              onChange={(e) =>
                set({
                  ...v,
                  login_mode: e.target.value as LoginMode,
                  oauth_client_id: "",
                })
              }
            >
              {v.kind === "azure" ? (
                <>
                  <option value="azure-cli">
                    复用这台服务主机的 Azure CLI
                  </option>
                  <option value="device">Device Login · 独立账号</option>
                </>
              ) : (
                <>
                  <option value="device">GitHub Device Login · Copilot</option>
                  <option value="oauth-app">自定义 OAuth App</option>
                </>
              )}
            </Select>
          </Field>
          {v.login_mode === "oauth-app" && (
            <Field label="OAuth App Client ID">
              <Input
                required
                disabled={created}
                value={v.oauth_client_id || ""}
                onChange={(e) => set({ ...v, oauth_client_id: e.target.value })}
              />
            </Field>
          )}
          <p className="muted">
            {sharedAzure(v)
              ? "直接使用已有 az login 身份，由 Azure CLI 管理令牌续期；不会复制登录缓存。"
              : v.kind === "azure"
                ? "通过浏览器授权，独立保存此账号的 Azure CLI 登录状态。"
                : v.login_mode === "oauth-app"
                  ? "自动使用 GitHub 返回的 refresh token；是否提供由 OAuth App 决定。"
                  : "设备授权会持久保存，Copilot exchange token 自动续期。"}
          </p>
        </>
      ) : (
        <Field
          label={v.kind === "pacaroute" ? "客户端密钥" : "API Key"}
          hint={
            v.kind === "pacaroute"
              ? "使用对方分配的客户端密钥；只发现这个客户端可访问的模型。"
              : "使用 API 账号的密钥，与 ChatGPT / Claude 聊天订阅独立。"
          }
        >
          <Input
            type="password"
            autoComplete="off"
            required
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="粘贴 API Key"
          />
        </Field>
      )}
      <details className="advanced-options">
        <summary>备注与高级设置</summary>
        <Field label="账号备注">
          <Input
            value={v.name}
            disabled={created}
            required
            onChange={(e) => set({ ...v, name: e.target.value })}
          />
        </Field>
        <Field label="内部 ID" hint="已自动生成，通常无需修改">
          <Input
            {...identifier}
            disabled={created}
            value={v.id}
            onChange={(e) => set({ ...v, id: e.target.value })}
          />
        </Field>
      </details>
      {created && (
        <p className="muted">账号已创建。若保存密钥失败，可在此重试。</p>
      )}
    </Editor>
  );
}
