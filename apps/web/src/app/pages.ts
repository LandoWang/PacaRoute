import {
  LayoutDashboard,
  Users,
  Server,
  Layers,
  ChartNoAxesCombined,
  Network,
  Settings2,
  TriangleAlert,
} from "lucide-react";
import { CodexIcon } from "../components/brand-icon";
export const pages = [
  ["overview", "导览", LayoutDashboard, "你的模型网络，一目了然。"],
  ["accounts", "账号", Users, "连接身份，管理授权。"],
  ["sources", "来源", Server, "组合账号、模型和网络。"],
  ["pools", "来源池", Layers, "按比例分配会话，按需切换来源。"],
  ["proxies", "代理", Network, "独立配置网络，按来源复用。"],
  ["clients", "客户端", CodexIcon, "管理调用方，为 Agent 分配来源并生成接入脚本。"],
  ["usage", "用量", ChartNoAxesCombined, "从整体到来源，查看每一次调用。"],
  ["settings", "服务配置", Settings2, "管理 PacaRoute 的运行方式。"],
  ["errors", "错误", TriangleAlert, "定位失败请求与来源问题。"],
] as const;
export const initialPage = () => {
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
