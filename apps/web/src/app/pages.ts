import {
  LayoutDashboard,
  Users,
  Server,
  Layers,
  ChartNoAxesCombined,
  Network,
  Settings2,
  TriangleAlert,
  type LucideIcon,
} from "lucide-react";
import { type ComponentType } from "react";
import { CodexIcon } from "../components/brand-icon";
import { Accounts } from "../features/accounts/accounts-page";
import { Clients } from "../features/clients/clients-page";
import { Errors } from "../features/errors/errors-page";
import { Overview } from "../features/overview/overview-page";
import { Pools } from "../features/pools/pools-page";
import { Proxies } from "../features/proxies/proxies-page";
import { Settings } from "../features/settings/settings-page";
import { Sources } from "../features/sources/sources-page";
import { UsagePage } from "../features/usage/usage-page";
// Navigation table: `id` is the #/<id> URL, in sidebar order.
export type Page = {
  id: string;
  label: string;
  icon: LucideIcon | typeof CodexIcon;
  description: string;
  view: ComponentType;
};
export const pages: Page[] = [
  {
    id: "overview",
    label: "导览",
    icon: LayoutDashboard,
    description: "你的模型网络，一目了然。",
    view: Overview,
  },
  {
    id: "accounts",
    label: "账号",
    icon: Users,
    description: "连接身份，管理授权。",
    view: Accounts,
  },
  {
    id: "sources",
    label: "来源",
    icon: Server,
    description: "组合账号、模型和网络。",
    view: Sources,
  },
  {
    id: "pools",
    label: "来源池",
    icon: Layers,
    description: "按比例分配会话，按需切换来源。",
    view: Pools,
  },
  {
    id: "proxies",
    label: "代理",
    icon: Network,
    description: "独立配置网络，按来源复用。",
    view: Proxies,
  },
  {
    id: "clients",
    label: "客户端",
    icon: CodexIcon,
    description: "管理调用方，为 Agent 分配来源并生成接入脚本。",
    view: Clients,
  },
  {
    id: "usage",
    label: "用量",
    icon: ChartNoAxesCombined,
    description: "从整体到来源，查看每一次调用。",
    view: UsagePage,
  },
  {
    id: "settings",
    label: "服务配置",
    icon: Settings2,
    description: "管理 PacaRoute 的运行方式。",
    view: Settings,
  },
  {
    id: "errors",
    label: "错误",
    icon: TriangleAlert,
    description: "定位失败请求与来源问题。",
    view: Errors,
  },
];
// Page id for the current hash; the legacy #/routes page is now #/clients.
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
  return pages.some((p) => p.id === id) ? id : "overview";
};
