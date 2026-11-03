import { Download, Upload } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/transport";
import { useApp } from "../../app/state";
import {
  Badge,
  Button,
  Editor,
  Field,
  Input,
  Panel,
  Select,
} from "../../components/ui";
const labels = {
  accounts: "账号",
  sources: "来源",
  pools: "来源池",
  proxies: "代理",
  clients: "客户端",
};
type Counts = Record<keyof typeof labels, number>;
export type ImportPlan = {
  mode: "add" | "replace";
  revision: string;
  namespace: string;
  fingerprint: string;
  added: Counts;
  removed: Counts;
  changes: {
    kind: keyof Counts;
    from_id: string;
    to_id: string;
    from_name: string;
    to_name: string;
  }[];
  issues: { code: string; id?: string; name?: string }[];
};
const issueLabels: Record<string, string> = {
  account_credentials_required: "需要重新登录或填写账号 Key",
  source_key_required: "需要填写资源 Key，或使用账号 Key",
  azure_login_isolated:
    "Azure CLI 账号改为独立登录，避免复用目标机器上的其他身份",
  local_client_converted: "已有本机客户端，导入的这份将改为带独立 Key 的客户端",
  client_key_generated: "生成新客户端密钥，导入后到客户端页获取接入脚本",
  account_proxy_preserved: "保留当前账号登录代理；来源各自的代理绑定会完整导入",
};

export function ImportSummary({ plan }: { plan: ImportPlan }) {
  const renamed = plan.changes.filter((c) => c.from_name !== c.to_name);
  return (
    <div className="transfer-preview">
      <div className="transfer-counts">
        {(Object.keys(labels) as (keyof Counts)[]).map((kind) => (
          <div key={kind}>
            <span>{labels[kind]}</span>
            <strong>{plan.added[kind]}</strong>
            {plan.mode === "replace" && (
              <small>替换现有 {plan.removed[kind]} 项</small>
            )}
          </div>
        ))}
      </div>
      {plan.mode === "replace" && (
        <p className="transfer-warning" role="note">
          覆盖会移除现有账号、来源、池、代理和客户端配置，网络客户端 将使用新
          Key。建议先导出当前配置。
        </p>
      )}
      <p className="muted">
        模型、权重、暂停状态和路由顺序会保留；导入项目使用独立内部
        ID，引用自动关联。
      </p>
      {renamed.length > 0 && (
        <div>
          <h3>自动调整名称</h3>
          <ul className="preview-lines">
            {renamed.map((c) => (
              <li key={c.kind + c.to_id}>
                {labels[c.kind]}：{c.from_name} → <strong>{c.to_name}</strong>
              </li>
            ))}
          </ul>
        </div>
      )}
      {plan.issues.length > 0 && (
        <details open>
          <summary>导入后需要完成的设置</summary>
          <ul className="preview-lines">
            {plan.issues.map((issue, i) => (
              <li key={i}>
                {issue.name && <strong>{issue.name}： </strong>}
                {issueLabels[issue.code] || issue.code}
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}

export function ConfigTransfer() {
  const { show, close, notice } = useApp();
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <Panel
      title="配置导入与导出"
      description="将账号、来源、来源池、客户端路由和代理配置迁移到另一台 PacaRoute。"
    >
      <div className="transfer-actions">
        <Badge tone="mint">仅配置 · 不含凭据</Badge>
        <div className="actions">
          <Button
            variant="secondary"
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              setError("");
              try {
                const bundle = await api("config/export");
                const url = URL.createObjectURL(
                  new Blob([JSON.stringify(bundle, null, 2)], {
                    type: "application/json",
                  }),
                );
                const link = document.createElement("a");
                link.href = url;
                link.download = `PacaRoute-config-${new Date().toLocaleDateString("sv-SE")}.json`;
                document.body.appendChild(link);
                link.click();
                link.remove();
                setTimeout(() => URL.revokeObjectURL(url), 1000);
                notice(
                  "已导出配置，不含账号凭据、客户端密钥、用量或错误记录。",
                );
              } catch (e) {
                setError((e as Error).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            <Download size={16} />
            导出配置
          </Button>
          <Button onClick={() => show(<ImportConfig onClose={close} />)}>
            <Upload size={16} />
            导入配置
          </Button>
        </div>
      </div>
      <p className="muted">
        导入时可添加或覆盖。账号需要重新授权，网络客户端会生成新
        Key；服务地址、端口和管理密码保持当前机器的设置。
      </p>
      {error && <p role="alert">{error}</p>}
    </Panel>
  );
}

function ImportConfig({ onClose }: { onClose: () => void }) {
  const { config, reload, notice, navigate } = useApp();
  const [bundle, setBundle] = useState<unknown>(),
    [mode, setMode] = useState<"add" | "replace">("add"),
    [plan, setPlan] = useState<ImportPlan>(),
    [fileError, setFileError] = useState(""),
    [reading, setReading] = useState(false),
    [filename, setFilename] = useState(""),
    [applied, setApplied] = useState(false);
  return (
    <Editor
      title={applied ? "配置已导入" : "导入 PacaRoute 配置"}
      description="先选择文件和导入方式，预览后再应用。"
      onClose={onClose}
      wide
      fixedHeight
      saveLabel={
        applied
          ? "去账号完成授权"
          : plan
            ? mode === "replace"
              ? "确认覆盖配置"
              : "确认添加配置"
            : "预览导入"
      }
      onSave={async () => {
        if (applied) {
          onClose();
          navigate("accounts");
          return;
        }
        if (reading) throw Error("正在读取文件，请稍候");
        if (!bundle || fileError)
          throw Error(fileError || "请选择 PacaRoute 导出的 JSON 文件");
        if (!plan) {
          setPlan(
            await api<ImportPlan>("config/import", "POST", {
              bundle,
              mode,
              revision: config.revision,
            }),
          );
          return;
        }
        try {
          await api("config/import", "POST", {
            bundle,
            mode,
            revision: plan.revision,
            namespace: plan.namespace,
            fingerprint: plan.fingerprint,
            apply: true,
          });
        } catch (e) {
          setPlan(undefined);
          await reload().catch(() => {});
          throw e;
        }
        setApplied(true);
        setBundle(undefined);
        notice(
          "配置已导入。请重新登录账号或填写 Key，并在客户端页获取新的接入脚本。",
        );
        await reload().catch(() => {
          notice("配置已导入，请刷新页面查看最新配置。");
        });
      }}
    >
      {!applied && (
        <>
          <Field
            label="配置文件"
            hint="PacaRoute 导出的 JSON 文件，最大 24 MiB。"
          >
            <Input
              type="file"
              accept=".json,application/json"
              disabled={reading}
              onChange={async (e) => {
                const file = e.target.files?.[0];
                setPlan(undefined);
                setBundle(undefined);
                setFilename(file?.name || "");
                setFileError("");
                if (!file) return;
                if (file.size > 24 * 1024 * 1024) {
                  setFileError("配置文件不能超过 24 MiB");
                  return;
                }
                setReading(true);
                try {
                  setBundle(JSON.parse(await file.text()));
                } catch {
                  setFileError("无法读取 JSON 配置文件");
                } finally {
                  setReading(false);
                }
              }}
            />
          </Field>
          <Field label="导入方式">
            <Select
              value={mode}
              onChange={(e) => {
                setMode(e.target.value as "add" | "replace");
                setPlan(undefined);
              }}
            >
              <option value="add">添加 — 保留现有配置，冲突时自动改名</option>
              <option value="replace">覆盖 — 替换现有配置</option>
            </Select>
          </Field>
          {fileError && <p role="alert">{fileError}</p>}
        </>
      )}
      {plan && (
        <>
          <p className="muted">
            {filename}
            {applied ? " · 已应用" : " · 导入预览"}
          </p>
          <ImportSummary plan={plan} />
        </>
      )}
    </Editor>
  );
}
