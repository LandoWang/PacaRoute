import { Download, Copy } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { api } from "../../api/transport";
import {
  type Client,
  type ProtocolAgent,
  type ClientKeyResponse,
} from "../../api/types";
import { useApp } from "../../app/state";
import {
  Button,
  Modal,
  Field,
  Input,
  Select,
  ErrorNote,
} from "../../components/ui";
import { sources } from "../../domain/config";
import { agentNames } from "../../domain/routing";
import {
  connectionScript,
  connectionScriptPreview,
  type ScriptPlatform,
} from "./connection-script";
import { connectionModelRoutes } from "./model-routes";
import { errorMessage } from "../../domain/errors";
export function ConnectionScript({
  client,
  agent,
  onClose,
}: {
  client: Client;
  agent: ProtocolAgent;
  onClose: () => void;
}) {
  const { status, config } = useApp();
  const [platform, setPlatform] = useState<ScriptPlatform>("shell"),
    [address, setAddress] = useState(status.gateway_url || location.origin),
    [copied, setCopied] = useState(false),
    [copyError, setCopyError] = useState("");
  const [apiKey, setApiKey] = useState(""),
    [keyLoading, setKeyLoading] = useState(!client.local),
    [keyError, setKeyError] = useState(""),
    [importKey, setImportKey] = useState("");
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    if (!client.local) {
      api<ClientKeyResponse>(`clients/${client.id}/key`)
        .then((v) => {
          if (active.current) setApiKey(v.api_key);
        })
        .catch((e) => {
          if (active.current) setKeyError(e.message);
        })
        .finally(() => {
          if (active.current) setKeyLoading(false);
        });
    }
    return () => {
      active.current = false;
    };
  }, [client.id, client.local]);
  const agentRoutes = client.routes.filter((r) => r.agent === agent);
  const kind =
    sources(config).find((s) => s.id === agentRoutes[0]?.source_id)?.kind || "";
  const matches = connectionModelRoutes(agentRoutes, kind),
    [routeID, setRouteID] = useState(matches[0]?.id || "");
  const route = matches.find((r) => r.id === routeID);
  const model =
    route?.match_model === "*" ? route.model : route?.match_model || "";
  let script = "",
    preview = "",
    error = "";
  try {
    if (client.local || apiKey) {
      script = connectionScript(
        client,
        agent,
        address,
        model,
        platform,
        apiKey,
      );
      preview = connectionScriptPreview(
        client,
        agent,
        address,
        model,
        platform,
      );
    }
  } catch (e) {
    error = errorMessage(e);
  }
  const filename = `pacaroute-${client.id}-${agent}.${platform === "shell" ? "sh" : "ps1"}`;
  return (
    <Modal
      title={`${client.name} · ${agentNames[agent]}`}
      onClose={onClose}
      wide
    >
      <div className="form-body">
        <p>
          在调用方电脑执行。需要 PacaRoute CLI
          0.8+；不需要启动本地服务。脚本会备份并修改 Agent 配置。
          {client.local
            ? "本机 default 无需 Key。"
            : "脚本自动包含此客户端的 Key，预览已隐藏；请只交给对应调用方。"}
        </p>
        <div className="form-grid">
          <Field label="系统">
            <Select
              value={platform}
              onChange={(e) => setPlatform(e.target.value as ScriptPlatform)}
            >
              <option value="shell">macOS / Linux · Bash</option>
              <option value="powershell">Windows · PowerShell</option>
            </Select>
          </Field>
          <Field
            label="服务地址"
            hint={
              client.local
                ? "default 仅连接本机"
                : "填写调用方可访问的地址；跨网络建议使用 HTTPS"
            }
          >
            <Input
              type="url"
              value={address}
              onChange={(e) => setAddress(e.target.value)}
            />
          </Field>
        </div>
        {matches.length > 1 && (
          <Field label="接入模型别名">
            <Select
              value={routeID}
              onChange={(e) => setRouteID(e.target.value)}
            >
              {matches.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.match_model === "*" ? r.model : r.match_model}
                </option>
              ))}
            </Select>
          </Field>
        )}
        {keyLoading && <p className="muted">正在加载客户端密钥…</p>}
        {keyError && <ErrorNote>{keyError}</ErrorNote>}
        {!client.local && !keyLoading && !apiKey && (
          <div className="form-body">
            <p className="muted">
              旧客户端只保存了 Key 的摘要。粘贴现有 Key
              一次，以后脚本会自动包含它；如果已经遗失，可关闭此窗口，在客户端
              下轮换密钥。
            </p>
            <Field label="现有客户端密钥">
              <Input
                type="password"
                autoComplete="off"
                value={importKey}
                onChange={(e) => setImportKey(e.target.value)}
              />
            </Field>
            <Button
              disabled={!importKey}
              variant="secondary"
              onClick={async () => {
                setKeyError("");
                try {
                  await api(`clients/${client.id}/key`, "PUT", {
                    key: importKey,
                  });
                  if (active.current) {
                    setApiKey(importKey);
                    setImportKey("");
                  }
                } catch (e) {
                  if (active.current) setKeyError(errorMessage(e));
                }
              }}
            >
              验证并保存 Key
            </Button>
          </div>
        )}
        {error && <ErrorNote>{error}</ErrorNote>}
        {copyError && <ErrorNote>{copyError}</ErrorNote>}
        {preview && <pre className="script-preview">{preview}</pre>}
        <div className="actions">
          <Button
            disabled={!script}
            onClick={() => {
              const blob = new Blob([script], {
                type: "text/plain;charset=utf-8",
              });
              const url = URL.createObjectURL(blob);
              const a = document.createElement("a");
              a.href = url;
              a.download = filename;
              a.click();
              setTimeout(() => URL.revokeObjectURL(url), 1000);
            }}
          >
            <Download size={15} />
            下载脚本
          </Button>
          <Button
            disabled={!script}
            variant="secondary"
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(script);
                setCopied(true);
              } catch {
                setCopyError("无法访问剪贴板，请下载脚本。");
              }
            }}
          >
            <Copy size={15} />
            {copied ? "已复制" : "复制"}
          </Button>
        </div>
        <small className="muted">
          执行：
          {platform === "shell"
            ? `bash ${filename}`
            : `powershell -File .\\${filename}`}
          。执行后重新打开 Agent。
        </small>
      </div>
    </Modal>
  );
}
