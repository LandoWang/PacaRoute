import { type Agent, type Route } from "../../api/types";
import { generatedID } from "../../domain/config";
// Exact aliases observed in real Claude Code requests. Do not infer aliases
// or protocol compatibility from model-family names.
const copilotClaudeAliases: Record<string, string[]> = {
  "claude-opus-5.5": ["claude-opus-5-5"],
  "claude-sonnet-5.5": ["claude-sonnet-5-5"],
};
function aliases(agent: Agent, kind: string, model: string) {
  return agent === "claude-code" && kind === "copilot"
    ? copilotClaudeAliases[model] || []
    : [];
}
export type ModelSelection = {
  source: string;
  models: string[];
  defaultModel: string;
  fixed: boolean;
};

export function readModelSelection(
  routes: Route[],
  kind: string,
): ModelSelection | null {
  if (!routes.length)
    return { source: "", models: [], defaultModel: "", fixed: false };
  const first = routes[0];
  if (routes.length === 1 && first.match_model === "*")
    return {
      source: first.source_id,
      models: [first.model],
      defaultModel: first.model,
      fixed: true,
    };
  const canonical = routes.filter((r) => r.match_model === r.model);
  const models = canonical.map((r) => r.model);
  if (
    !models.length ||
    new Set(models).size !== models.length ||
    routes.some(
      (r) =>
        r.source_id !== first.source_id ||
        r.agent !== first.agent ||
        !models.includes(r.model) ||
        (r.match_model !== r.model &&
          !aliases(r.agent, kind, r.model).includes(r.match_model)),
    )
  )
    return null;
  // The first canonical rule is the connection-script default, not a fallback.
  return {
    source: first.source_id,
    models,
    defaultModel: models[0],
    fixed: false,
  };
}

export function selectedModelRoutes(
  agent: Agent,
  kind: string,
  selection: ModelSelection,
  existing: Route[],
): Route[] {
  const { source, defaultModel, fixed } = selection;
  const models = [...new Set(selection.models)];
  if (!source || !models.length || !models.includes(defaultModel))
    throw new Error("请选择模型和默认接入模型");
  function rule(model: string, match_model: string): Route {
    const old = existing.find(
      (r) =>
        r.agent === agent &&
        r.source_id === source &&
        r.match_model === match_model &&
        r.model === model,
    );
    return {
      id: old?.id || generatedID("route"),
      agent,
      source_id: source,
      model,
      match_model,
    };
  }
  if (fixed) return [rule(defaultModel, "*")];
  return [defaultModel, ...models.filter((m) => m !== defaultModel)].flatMap(
    (model) =>
      [model, ...aliases(agent, kind, model)].map((match) =>
        rule(model, match),
      ),
  );
}

export function connectionModelRoutes(routes: Route[], kind: string): Route[] {
  const selection = readModelSelection(routes, kind);
  return selection && !selection.fixed
    ? routes.filter((r) => r.match_model === r.model)
    : routes;
}
