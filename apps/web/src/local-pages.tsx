import { useState } from "react";
import { api, type Agent } from "./api";
import { useApp, useLoad } from "./state";
import {
  Button,
  Badge,
  Panel,
  Empty,
  Editor,
  Modal,
  Field,
  Input,
  Select,
  ErrorNote,
} from "./components/ui";
import { Loading } from "./pages";
type Plan = {
  path: string;
  fingerprint: string;
  changes: string[];
  backup_path?: string;
};
export function BackupPicker({
  agent,
  onClose,
  onApplied,
}: {
  agent: Agent;
  onClose: () => void;
  onApplied: () => void;
}) {
  const list = useLoad<{
    backups: {
      id: string;
      created_at: string;
      missing_file: boolean;
      valid: boolean;
    }[];
  }>("local/backups?agent=" + agent);
  const [selected, setSelected] = useState(""),
    [plan, setPlan] = useState<Plan>();
  return (
    <Editor
      title="恢复 Agent 配置"
      onClose={onClose}
      saveLabel={plan ? "备份当前配置并恢复" : "预览恢复"}
      onSave={async () => {
        if (!selected) throw Error("请选择备份");
        if (!plan) {
          setPlan(
            await api<Plan>("local/restore", "POST", {
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
export function Settings() {
  const { status, reload, notice } = useApp();
  const [port, setPort] = useState(String(status.port || 8766)),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <>
      <Panel
        title="PacaRoute 服务"
        description="前端通过 HTTP API 连接 Go 服务。发布版内嵌网页，无需安装 Node。"
      >
        <dl className="details-grid">
          <div>
            <dt>版本</dt>
            <dd>{status.version}</dd>
          </div>
          <div>
            <dt>服务地址</dt>
            <dd>{status.gateway_url || location.origin}</dd>
          </div>
          <div>
            <dt>运行时间</dt>
            <dd>{Math.floor(status.uptime_seconds / 60)} 分钟</dd>
          </div>
          <div>
            <dt>管理模式</dt>
            <dd>
              {status.desktop
                ? "本机桌面托盘"
                : status.local_management
                  ? "本机 CLI"
                  : "服务管理"}
            </dd>
          </div>
        </dl>
      </Panel>
      {status.desktop ? (
        <Panel
          title="本机服务设置"
          description="修改端口会创建新监听地址；原端口上的请求会有最多 30 秒完成。"
        >
          <div className="form-body compact">
            <Field label="监听端口">
              <Input
                type="number"
                min="1024"
                max="65535"
                value={port}
                onChange={(e) => setPort(e.target.value)}
              />
            </Field>
            <Button
              disabled={busy || Number(port) === status.port}
              onClick={async () => {
                setBusy(true);
                setError("");
                try {
                  const result = await api<{ url: string }>(
                    "local/service",
                    "PUT",
                    { port: Number(port) },
                  );
                  location.assign(result.url);
                } catch (e) {
                  setError((e as Error).message);
                  setBusy(false);
                }
              }}
            >
              应用端口并重新连接
            </Button>
            <label className="switch-row">
              <div>
                <strong>登录系统时启动</strong>
                <small>在 Mac 顶栏或 Windows 系统托盘中运行</small>
              </div>
              <input
                type="checkbox"
                checked={status.startup}
                disabled={busy}
                onChange={async (e) => {
                  setBusy(true);
                  setError("");
                  try {
                    await api("local/startup", "PUT", {
                      enabled: e.target.checked,
                    });
                    await reload();
                  } catch (e) {
                    setError((e as Error).message);
                  } finally {
                    setBusy(false);
                  }
                }}
              />
            </label>
            {error && <ErrorNote>{error}</ErrorNote>}
          </div>
        </Panel>
      ) : (
        <Panel title="启动方式">
          <pre>
            pacaroute desktop{`\n`}pacaroute serve --port 8766{`\n`}pacaroute ui
            --port 8766
          </pre>
          <p className="muted">
            CLI 服务通过启动参数设置监听地址和端口；账号、来源、Proxy 和 Client
            路由都支持热更新。
          </p>
        </Panel>
      )}
      <Panel title="本机访问">
        <p>
          {status.local_access
            ? "已允许本机 Agent 使用本地路由。网络调用方仍需独立 Client 密钥。"
            : "所有 Agent 请求均需独立 Client 密钥。"}
        </p>
        <p className="muted">
          {status.local_management
            ? "本机配置备份与恢复已启用，仅限本地访问。"
            : "本机文件功能未开放。桌面模式会自动启用，CLI 可使用 --local-management。"}
        </p>
      </Panel>
    </>
  );
}
