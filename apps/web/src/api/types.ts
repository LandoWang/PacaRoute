export type ModelPrice = {
  input_usd: string;
  cached_usd: string;
  output_usd: string;
  cache_write_usd?: string;
};
export type ClientQuota = {
  window_seconds: number;
  requests?: number;
  input_tokens?: number;
  cost_usd?: string;
};
// Native Agent protocol (pool members, source capabilities, local Agent configs).
export type ProtocolAgent = "codex" | "claude-code";
// What a Client may be granted: a native protocol or downstream PacaRoute.
export type Grant = ProtocolAgent | "pacaroute";
// Literal sets accepted by Config.Validate in internal/pacaroute/config.go.
export type AccountKind =
  | "github"
  | "azure"
  | "openai"
  | "anthropic"
  | "api-key"
  | "pacaroute";
export type LoginMode = "device" | "oauth-app" | "azure-cli";
export type SourceKind =
  | "copilot"
  | "azure-openai"
  | "openai"
  | "anthropic"
  | "pacaroute";
export type SourceAuthMode = "api-key" | "azure-cli";
export type ProxyMode = "custom" | "direct" | "system";
export type Account = {
  id: string;
  name: string;
  kind: AccountKind;
  endpoint?: string;
  oauth_client_id?: string;
  login_mode?: LoginMode | "";
};
export type Source = {
  prices?: Record<string, ModelPrice>;
  id: string;
  name: string;
  kind: SourceKind;
  account_id?: string;
  endpoint: string;
  auth_mode: SourceAuthMode;
  subscription_id?: string;
  resource_id?: string;
  deployments: string[];
  agent_models?: Partial<Record<ProtocolAgent, string[]>>;
};
export type Proxy = {
  id: string;
  name: string;
  mode: ProxyMode;
  address?: string;
};
export type Member = {
  alias?: string;
  id: string;
  source_id: string;
  model: string;
  weight: number;
};
export type Pool = {
  multi_model?: boolean;
  id: string;
  name: string;
  agent: ProtocolAgent;
  model: string;
  failover: boolean;
  members: Member[];
};
export type Route = {
  id: string;
  agent: Grant;
  match_model: string;
  source_id: string;
  model: string;
};
export type Client = {
  quota?: ClientQuota;
  local?: boolean;
  legacy_direct?: boolean;
  // Omitted or null (legacy) means the default Codex + Claude Code grants;
  // an empty list grants nothing. Keep the distinction when editing.
  enabled_agents?: Grant[] | null;
  id: string;
  name: string;
  disabled: boolean;
  key_hash?: string;
  routes: Route[];
};
export type Config = {
  revision?: string;
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
  routing?: RoutingHealth;
  version: string;
  uptime_seconds: number;
  config_error?: string;
  local_access: boolean;
  local_management: boolean;
  desktop: boolean;
  service_control?: boolean;
  service?: {
    serving: boolean;
    port?: number;
    admin_port?: number;
    draining?: number;
    error?: string;
  };
  port: number;
  startup: boolean;
  api_version: number;
  gateway_url: string;
};
export type Cooldown = {
  kind?: "error" | "throttle";
  source_id: string;
  model: string;
  error: string;
  failed_at: string;
  until: string;
};
export type RoutingHealth = {
  observed_at: string;
  cooldowns: Cooldown[];
  pools: {
    id: string;
    available: number;
    total: number;
    models?: { model: string; available: number; total: number }[];
  }[];
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
export type ModelDiscovery = {
  endpoint?: string;
  models: Model[];
  resource_id?: string;
  subscription_id?: string;
};
