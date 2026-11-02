export type Agent = "codex" | "claude-code";
export type Account = {
  id: string;
  name: string;
  kind: string;
  oauth_client_id?: string;
  login_mode?: string;
};
export type Source = {
  id: string;
  name: string;
  kind: string;
  account_id: string;
  endpoint: string;
  auth_mode: string;
  subscription_id?: string;
  resource_id?: string;
  deployments: string[];
};
export type Proxy = {
  id: string;
  name: string;
  mode: string;
  address?: string;
};
export type Member = {
  id: string;
  source_id: string;
  model: string;
  weight: number;
};
export type Pool = {
  id: string;
  name: string;
  agent: Agent;
  model: string;
  failover: boolean;
  members: Member[];
};
export type Route = {
  id: string;
  agent: Agent;
  match_model: string;
  source_id: string;
  model: string;
};
export type Client = {
  local?: boolean;
  legacy_direct?: boolean;
  enabled_agents?: Agent[];
  id: string;
  name: string;
  disabled: boolean;
  key_hash?: string;
  routes: Route[];
};
export type Config = {
  revision: string;
  accounts: Account[];
  sources: Source[];
  pools: Pool[];
  clients: Client[];
  proxies: {
    profiles: Proxy[];
    bindings: Record<string, string>;
    account_proxy_id: string;
  };
  source_state: { paused: string[] };
};
export type Status = {
  version: string;
  uptime_seconds: number;
  config_error?: string;
  local_access: boolean;
  local_management: boolean;
  desktop: boolean;
  port: number;
  startup: boolean;
  api_version: number;
  gateway_url: string;
};
export type Totals = {
  requests: number;
  input_tokens: number;
  output_tokens: number;
  cached_tokens: number;
  failures: number;
  missing_usage: number;
};
export type Usage = {
  total: Totals;
  sources: Record<string, Totals>;
  source_models: Record<string, Record<string, Totals>>;
  timeline: Record<string, Totals>;
};
export type Activity = {
  id: string;
  timestamp: string;
  source: string;
  model: string;
  client: string;
  client_id: string;
  status: number;
  input_tokens: number | null;
  output_tokens: number | null;
  duration_ms: number;
  error?: string;
};
export type Model = {
  id: string;
  name: string;
  vendor: string;
  codex: boolean;
  claude: boolean;
};
export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const response = await fetch("/api/" + path, {
    method,
    signal,
    credentials: "same-origin",
    headers:
      body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const value = await response
    .json()
    .catch(() => ({ error: { message: "服务返回了无效响应" } }));
  if (!response.ok) {
    if (response.status === 401)
      window.dispatchEvent(new Event("pacaroute:unauthorized"));
    throw new APIError(response.status, value.error?.message || "请求失败");
  }
  return value;
}
export const builtinAccounts: Account[] = [
  { id: "copilot", name: "GitHub（默认）", kind: "github" },
  { id: "azure-local", name: "本机 Azure CLI", kind: "azure" },
];
export const builtinSource: Source = {
  id: "copilot",
  name: "GitHub Copilot",
  kind: "copilot",
  account_id: "copilot",
  endpoint: "",
  auth_mode: "",
  deployments: [],
};
export const accounts = (c: Config) => [...builtinAccounts, ...c.accounts];
export const sources = (c: Config) => [builtinSource, ...c.sources];
export const sourceName = (c: Config, id: string) =>
  id.startsWith("pool:")
    ? c.pools.find((p) => "pool:" + p.id === id)?.name || id
    : sources(c).find((s) => s.id === id)?.name || id;
export function routeTarget(route: Route, target: string, c: Config): Route {
  const pool = c.pools.find((p) => "pool:" + p.id === target);
  return {
    ...route,
    source_id: target,
    model:
      pool?.model ||
      sources(c).find((s) => s.id === target)?.deployments?.[0] ||
      "",
  };
}
export function routeAgent(route: Route, agent: Agent, c: Config): Route {
  const pool = c.pools.find((p) => "pool:" + p.id === route.source_id);
  return pool && pool.agent !== agent
    ? { ...route, agent, source_id: "copilot", model: "" }
    : { ...route, agent };
}
export const share = (weight: number, members: Member[]) =>
  members.reduce((n, m) => n + m.weight, 0) > 0
    ? Math.round((weight / members.reduce((n, m) => n + m.weight, 0)) * 100)
    : 0;
export const format = (n: number | null | undefined) =>
  n == null
    ? "—"
    : new Intl.NumberFormat("zh-CN", {
        notation: n >= 100000 ? "compact" : "standard",
        maximumFractionDigits: 1,
      }).format(n);

export const accountProvider = (kind: string) =>
  ({
    github: "GitHub",
    openai: "OpenAI",
    anthropic: "Anthropic",
    azure: "Azure",
    "api-key": "兼容 API Key",
  })[kind] || kind;
export const deviceAccount = (kind: string) =>
  kind === "github" || kind === "azure";
export const compatibleAccount = (source: string, kind: string) =>
  source === "copilot"
    ? kind === "github"
    : source === "azure-openai"
      ? kind === "azure"
      : kind === source || kind === "api-key";
export const resourceKind = (account: string) =>
  account === "github"
    ? "copilot"
    : account === "azure"
      ? "azure-openai"
      : account === "anthropic"
        ? "anthropic"
        : "openai";
export const defaultEndpoint = (kind: string) =>
  kind === "openai"
    ? "https://api.openai.com/v1"
    : kind === "anthropic"
      ? "https://api.anthropic.com"
      : "";
export type ModelDiscovery = {
  models: Model[];
  resource_id?: string;
  subscription_id?: string;
};

export const generatedID = (prefix: string) =>
  `${prefix}-${crypto.randomUUID().slice(0, 8)}`;
export function suggestedName(
  label: string,
  items: { name: string }[],
): string {
  const names = new Set(items.map((item) => item.name));
  if (!names.has(label)) return label;
  let suffix = 2;
  while (names.has(`${label} ${suffix}`)) suffix++;
  return `${label} ${suffix}`;
}

export const agentNames: Record<Agent, string> = {
  codex: "Codex",
  "claude-code": "Claude Code",
};
export const enabledAgents = (c: Client): Agent[] =>
  c.enabled_agents ?? ["codex", "claude-code"];
export const sharedAzure = (a: Account) =>
  a.kind === "azure" &&
  (a.id === "azure-local" || a.login_mode === "azure-cli");
