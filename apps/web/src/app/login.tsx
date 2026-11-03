import { ArrowLeftRight, RefreshCw } from "lucide-react";
import { useState } from "react";
import pacaRouteMark from "../assets/pacaroute-mark.png";
import { Button, Input, Field, ErrorNote } from "../components/ui";
export function Login({
  loading,
  error,
  onLogin,
}: {
  loading: boolean;
  error: string;
  onLogin: (key: string) => Promise<void>;
}) {
  const [key, setKey] = useState(""),
    [busy, setBusy] = useState(false),
    [failure, setFailure] = useState("");
  return (
    <div className="login-screen">
      <div className="login-card">
        <img src={pacaRouteMark} alt="戴霓虹眼镜的 PacaRoute 羊驼" />
        <div className="eyebrow">WELCOME TO PACAROUTE</div>
        <h1>连接你的模型网络</h1>
        <p>
          从桌面托盘打开可自动连接。
          <br />
          管理独立服务时，请输入该服务的管理密钥。
        </p>
        {loading ? (
          <p className="loading">
            <RefreshCw size={16} className="spin" />
            正在连接…
          </p>
        ) : (
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              setBusy(true);
              setFailure("");
              try {
                await onLogin(key);
                setKey("");
              } catch (e) {
                setFailure((e as Error).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            <Field label="管理密钥">
              <Input
                type="password"
                autoComplete="off"
                required
                value={key}
                onChange={(e) => setKey(e.target.value)}
                placeholder="服务数据目录中的 admin.key"
              />
            </Field>
            {(failure || error) && <ErrorNote>{failure || error}</ErrorNote>}
            <Button disabled={busy}>
              {busy ? "正在连接…" : "连接服务"}
              <ArrowLeftRight size={16} />
            </Button>
          </form>
        )}
        <small>服务地址：{location.origin}</small>
      </div>
    </div>
  );
}
