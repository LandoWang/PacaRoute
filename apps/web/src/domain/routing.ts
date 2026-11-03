import {
  type Agent,
  type Member,
  type Pool,
  type Route,
  type Client,
  type Config,
} from "../api/types";
import { sources } from "./config";
export const memberModel = (p: Pool, m: Member) =>
  p.multi_model ? m.alias || m.model : p.model;
export const poolModels = (p: Pool) => [
  ...new Set(p.members.map((m) => memberModel(p, m))),
];
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
  return pool && agent !== "pacaroute" && pool.agent !== agent
    ? { ...route, agent, source_id: sources(c)[0]?.id || "", model: "" }
    : { ...route, agent };
}
export const share = (weight: number, members: Member[]) =>
  members.reduce((n, m) => n + m.weight, 0) > 0
    ? Math.round((weight / members.reduce((n, m) => n + m.weight, 0)) * 1000) /
      10
    : 0;
export const agentNames: Record<Agent, string> = {
  pacaroute: "PacaRoute",
  codex: "Codex",
  "claude-code": "Claude Code",
};
export const enabledAgents = (c: Client): Agent[] =>
  c.enabled_agents ?? ["codex", "claude-code"];
