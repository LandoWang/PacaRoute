import { useEffect, useState, type ReactNode } from "react";
import {
  LayoutDashboard,
  Users,
  Server,
  ArrowLeftRight,
  Layers,
  ChartNoAxesCombined,
  Network,
  Settings2,
  TriangleAlert,
  RefreshCw,
  LogOut,
  Menu,
  X,
  ShieldCheck,
} from "lucide-react";
import { api, APIError, type Config, type Status } from "./api";
import { StateContext } from "./state";
import { Button, Input, Field, ErrorNote } from "./components/ui";
import {
  Overview,
  Accounts,
  Sources,
  Pools,
  Proxies,
  UsagePage,
  Errors,
} from "./pages";
import { Settings } from "./local-pages";
import { Clients } from "./routing-pages";
const pages = [
  ["overview", "导览", LayoutDashboard, "你的模型网络，一目了然。"],
  ["accounts", "账号", Users, "连接身份，管理授权。"],
  ["sources", "来源", Server, "组合账号、模型和网络。"],
  ["pools", "来源池", Layers, "按比例分配会话，按需切换来源。"],
  ["clients", "Client", Users, "管理调用方，为 Agent 分配来源并生成接入脚本。"],
  ["usage", "用量", ChartNoAxesCombined, "从整体到来源，查看每一次调用。"],
  ["proxies", "Proxy", Network, "独立配置网络，按来源复用。"],
  ["settings", "服务配置", Settings2, "管理 PacaRoute 的运行方式。"],
  ["errors", "错误", TriangleAlert, "定位失败请求与来源问题。"],
] as const;
const initialPage = () => {
  const id = location.hash.replace("#/", "").split("?")[0];
  if (id === "routes") {
    history.replaceState(
      null,
      "",
      location.href.replace("#/routes", "#/clients"),
    );
    return "clients";
  }
  return pages.some((p) => p[0] === id) ? id : "overview";
};
let bootstrap: Promise<void> | undefined;
function browserLogin() {
  if (!bootstrap) {
    const params = new URLSearchParams(location.hash.split("?")[1] || "");
    const ticket = params.get("ticket");
    if (ticket) {
      history.replaceState(
        null,
        "",
        location.pathname + location.search + "#/overview",
      );
      bootstrap = api("login/ticket", "POST", { ticket }).then(() => {});
    } else bootstrap = Promise.resolve();
  }
  return bootstrap;
}
export function App() {
  const [config, setConfig] = useState<Config>(),
    [status, setStatus] = useState<Status>(),
    [auth, setAuth] = useState(false),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(""),
    [message, setMessage] = useState(""),
    [page, setPage] = useState(initialPage),
    [dialog, setDialog] = useState<ReactNode>(),
    [sidebar, setSidebar] = useState(false),
    [refreshing, setRefreshing] = useState(false);
  async function reload() {
    const [c, s] = await Promise.all([
      api<Config>("config"),
      api<Status>("status"),
    ]);
    setConfig(c);
    setStatus(s);
    setAuth(true);
    setError("");
  }
  useEffect(() => {
    let active = true;
    browserLogin()
      .then(reload)
      .catch((e) => {
        if (active && (!(e instanceof APIError) || e.status !== 401))
          setError(e.message);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    const unauthorized = () => {
      setAuth(false);
      setDialog(undefined);
    };
    const hash = () => {
      setPage(initialPage());
      setSidebar(false);
      setMessage("");
    };
    window.addEventListener("pacaroute:unauthorized", unauthorized);
    window.addEventListener("hashchange", hash);
    return () => {
      active = false;
      window.removeEventListener("pacaroute:unauthorized", unauthorized);
      window.removeEventListener("hashchange", hash);
    };
  }, []);
  useEffect(() => {
    if (!auth) return;
    const tick = setInterval(() => {
      if (!document.hidden)
        void api<Status>("status")
          .then((value) => {
            setStatus(value);
            setError("");
          })
          .catch((e) => {
            if (!(e instanceof APIError && e.status === 401))
              setError("服务连接中断，请从托盘检查服务状态。");
          });
    }, 10000);
    return () => clearInterval(tick);
  }, [auth]);
  function navigate(id: string) {
    location.hash = "/" + id;
    setPage(id);
    setSidebar(false);
    setMessage("");
  }
  async function save(next: Config) {
    const c = await api<Config>("config", "PUT", next);
    setConfig(c);
    setMessage("已保存，新请求立即使用新配置。");
  }
  if (!auth || !config || !status)
    return (
      <Login
        loading={loading}
        error={error}
        onLogin={async (key) => {
          await api("login", "POST", { key });
          await reload();
        }}
      />
    );
  const current = pages.find((p) => p[0] === page) || pages[0];
  const views: Record<string, ReactNode> = {
    overview: <Overview />,
    accounts: <Accounts />,
    sources: <Sources />,
    pools: <Pools />,
    clients: <Clients />,
    usage: <UsagePage />,
    proxies: <Proxies />,
    settings: <Settings />,
    errors: <Errors />,
  };
  return (
    <StateContext.Provider
      value={{
        config,
        status,
        save,
        reload,
        show: setDialog,
        close: () => setDialog(undefined),
        notice: setMessage,
        navigate,
      }}
    >
      <div className="app-shell">
        {sidebar && (
          <button
            className="sidebar-backdrop"
            aria-label="关闭导航"
            onClick={() => setSidebar(false)}
          />
        )}
        <aside className={sidebar ? "sidebar open" : "sidebar"}>
          <a href="#/overview" className="brand">
            <img src="/mark.svg" alt="PacaRoute 羊驼" />
            <div>
              <strong>PacaRoute</strong>
              <small>连接模型与 Agent</small>
            </div>
          </a>
          <span className="nav-caption">工作空间</span>
          <nav>
            {pages.map(([id, label, Icon]) => (
              <button
                key={id}
                aria-current={page === id ? "page" : undefined}
                className={page === id ? "nav-item active" : "nav-item"}
                onClick={() => navigate(id)}
              >
                <Icon size={18} />
                {label}
                {id === "errors" && status.config_error && (
                  <span className="error-dot" />
                )}
              </button>
            ))}
          </nav>
          <div className="sidebar-footer">
            <div className="service-badge">
              <span className={error ? "status-dot warning" : "status-dot"} />
              <div>
                <strong>{error ? "连接异常" : "服务运行中"}</strong>
                <small>
                  Go {status.version} ·{" "}
                  {status.port || new URL(location.href).port}
                </small>
              </div>
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={async () => {
                try {
                  await api("logout", "POST", {});
                  setAuth(false);
                  setDialog(undefined);
                } catch (e) {
                  setError((e as Error).message);
                }
              }}
            >
              <LogOut size={14} />
              退出管理
            </Button>
          </div>
        </aside>
        <main>
          <header className="page-header">
            <div className="header-title">
              <Button
                variant="ghost"
                size="icon"
                className="mobile-menu"
                aria-label="打开导航"
                onClick={() => setSidebar(true)}
              >
                <Menu size={20} />
              </Button>
              <div>
                <h1>{current[1]}</h1>
                <p>{current[3]}</p>
              </div>
            </div>
            <div className="actions">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => navigate("settings")}
              >
                <span className="status-dot" />
                {status.desktop ? "本机服务" : "服务在线"}
              </Button>
              <Button
                variant="ghost"
                size="icon"
                aria-label="刷新状态"
                disabled={refreshing}
                onClick={async () => {
                  setRefreshing(true);
                  try {
                    await reload();
                    setMessage("状态已刷新");
                  } catch (e) {
                    setError((e as Error).message);
                  } finally {
                    setRefreshing(false);
                  }
                }}
              >
                <RefreshCw size={17} className={refreshing ? "spin" : ""} />
              </Button>
            </div>
          </header>
          {(error || status.config_error) && (
            <ErrorNote>{error || status.config_error}</ErrorNote>
          )}
          {message && (
            <div className="notice" role="status">
              <ShieldCheck size={17} />
              <span>{message}</span>
              <Button
                variant="ghost"
                size="icon"
                aria-label="关闭提示"
                onClick={() => setMessage("")}
              >
                <X size={15} />
              </Button>
            </div>
          )}
          <div className="page-content" key={page}>
            {views[page] || views.overview}
          </div>
          <footer className="page-footer">
            PacaRoute · 你的模型，你的路由。
          </footer>
        </main>
      </div>
      {dialog}
    </StateContext.Provider>
  );
}
function Login({
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
        <img src="/mark.svg" alt="极光电子羊驼" />
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
