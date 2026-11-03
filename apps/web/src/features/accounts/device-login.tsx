import { ExternalLink, RefreshCw, ShieldCheck } from "lucide-react";
import { useEffect, useState, useRef } from "react";
import { api } from "../../api/transport";
import {
  type Account,
  type DeviceLoginStart,
  type LoginJob,
} from "../../api/types";
import { useApp } from "../../app/state";
import { Button, Modal, ErrorNote } from "../../components/ui";
export function DeviceLogin({
  account,
  onClose,
}: {
  account: Account;
  onClose: () => void;
}) {
  const { reload } = useApp();
  const [device, setDevice] = useState<DeviceLoginStart>(),
    [done, setDone] = useState(false),
    [error, setError] = useState("");
  const job = useRef<Promise<DeviceLoginStart> | null>(null);
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    job.current ??= api(`accounts/${account.id}/login`, "POST", {});
    job.current
      .then(async (d) => {
        if (!active) return;
        if (d.done) {
          setDone(true);
          return;
        }
        const url = new URL(d.verification_uri);
        if (
          url.protocol !== "https:" ||
          !["github.com", "microsoft.com"].includes(url.hostname)
        )
          throw Error("验证地址无效");
        setDevice(d);
        const until = Date.now() + d.expires_in * 1000;
        const poll = async () => {
          try {
            const result = await api<LoginJob>("logins/" + d.job_id);
            if (!active) return;
            if (result.done) {
              if (result.error) throw Error(result.error);
              setDone(true);
              await reload();
              return;
            }
            if (Date.now() > until) throw Error("验证码已过期，请重新登录");
            timer = setTimeout(poll, 3000);
          } catch (e) {
            if (active) setError((e as Error).message);
          }
        };
        void poll();
      })
      .catch((e) => {
        if (active) setError(e.message);
      });
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [job]);
  return (
    <Modal title={"登录 " + account.name} onClose={onClose}>
      <div className="form-body">
        {done ? (
          <div className="success-message">
            <ShieldCheck size={32} />
            <h3>登录成功</h3>
            <p>现在可以添加或使用这个账号的模型来源。</p>
          </div>
        ) : device ? (
          <>
            <p>打开验证页面，输入下方代码。</p>
            <div className="device-code">{device.user_code}</div>
            <Button asChild>
              <a
                href={device.verification_uri}
                target="_blank"
                rel="noreferrer"
              >
                打开授权页面
                <ExternalLink size={16} />
              </a>
            </Button>
            <p className="muted">
              <RefreshCw size={14} className="spin" /> 等待授权完成…
            </p>
          </>
        ) : (
          !error && <p>正在获取验证码…</p>
        )}
        {error && <ErrorNote>{error}</ErrorNote>}
      </div>
    </Modal>
  );
}
