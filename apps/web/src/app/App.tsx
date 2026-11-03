import {
  RefreshCw,
  LogOut,
  Menu,
  X,
  ShieldCheck,
} from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { api, APIError } from "../api/transport";
import { type Config, type Status } from "../api/types";
import pacaRouteMark from "../assets/pacaroute-mark.png";
import { Button, ErrorNote } from "../components/ui";
import { RoutingAlert } from "../features/pools/routing-health";
import { saveConfig } from "./config-save";
import { startPolling } from "./polling";
import { Login } from "./login";
import { initialPage, pages } from "./pages";
import { StateContext } from "./state";
import { errorMessage } from "../domain/errors";
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
    return startPolling(
      (signal) =>
        api<Status>("status", "GET", undefined, signal)
          .then((value) => {
            setStatus(value);
            setError("");
          })
          .catch((e) => {
            if (!signal.aborted && !(e instanceof APIError && e.status === 401))
              setError("服务连接中断，请从托盘检查服务状态。");
          }),
      5000,
      false,
    );
  }, [auth]);
  function navigate(id: string) {
    location.hash = "/" + id;
    setPage(id);
    setSidebar(false);
    setMessage("");
  }
  async function save(next: Config) {
    const c = await saveConfig(next, setConfig);
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
  const current = pages.find((p) => p.id === page) || pages[0];
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
            <img src={pacaRouteMark} alt="PacaRoute 羊驼" />
            <div>
              <strong>PacaRoute</strong>
              <small>连接模型与 Agent</small>
            </div>
          </a>
          <span className="nav-caption">工作空间</span>
          <nav>
            {pages.map(({ id, label, icon: Icon }) => (
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
              <span
                className={
                  error || status.service?.serving === false
                    ? "status-dot warning"
                    : "status-dot"
                }
              />
              <div>
                <strong>
                  {error
                    ? "连接异常"
                    : status.service?.serving === false
                      ? "模型服务已关闭"
                      : "服务运行中"}
                </strong>
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
                  setError(errorMessage(e));
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
                <h1>{current.label}</h1>
                <p>{current.description}</p>
              </div>
            </div>
            <div className="actions">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => navigate("settings")}
              >
                <span className="status-dot" />
                {status.service?.serving === false
                  ? "模型服务已关闭"
                  : status.desktop
                    ? "本机服务"
                    : "服务在线"}
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
                    setError(errorMessage(e));
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
          <RoutingAlert />
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
            <current.view />
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
