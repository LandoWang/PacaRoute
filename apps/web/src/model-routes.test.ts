import { describe, it, expect } from "vitest";
import {
  readModelSelection,
  selectedModelRoutes,
  connectionModelRoutes,
  type ModelSelection,
} from "./model-routes";
import type { Route } from "./api";

const selection: ModelSelection = {
  source: "copilot",
  models: ["claude-sonnet-5.5", "claude-opus-5.5", "claude-sonnet-5"],
  defaultModel: "claude-opus-5.5",
  fixed: false,
};
describe("Client × Agent model selection", () => {
  it("routes each selected model exactly and never silently substitutes the default", () => {
    const rules = selectedModelRoutes("claude-code", "copilot", selection, []);
    const resolve = (model: string) =>
      rules.find((r) => r.match_model === model || r.match_model === "*")
        ?.model;
    expect(resolve("claude-sonnet-5-5")).toBe("claude-sonnet-5.5");
    expect(resolve("claude-opus-5-5")).toBe("claude-opus-5.5");
    expect(resolve("claude-sonnet-5")).toBe("claude-sonnet-5");
    expect(resolve("claude-haiku-4.5")).toBeUndefined();
    expect(rules.some((r) => r.match_model === "*")).toBe(false);
  });
  it("persists the connection default first without duplicate alias choices on reload", () => {
    const rules = selectedModelRoutes("claude-code", "copilot", selection, []);
    const reloaded = readModelSelection(
      JSON.parse(JSON.stringify(rules)),
      "copilot",
    )!;
    expect(reloaded.defaultModel).toBe("claude-opus-5.5");
    expect(new Set(reloaded.models)).toEqual(new Set(selection.models));
    expect(
      connectionModelRoutes(rules, "copilot").map((r) => r.match_model),
    ).toEqual(["claude-opus-5.5", "claude-sonnet-5.5", "claude-sonnet-5"]);
    expect(
      selectedModelRoutes("claude-code", "copilot", reloaded, rules),
    ).toEqual(rules);
  });
  it("does not collapse advanced source mappings, custom aliases, or fallbacks", () => {
    const rules = selectedModelRoutes("claude-code", "copilot", selection, []);
    for (const extra of [
      { match_model: "*", model: "claude-opus-5.5", source_id: "copilot" },
      { match_model: "fast", model: "claude-opus-5.5", source_id: "copilot" },
      { match_model: "gpt-6-astra", model: "gpt-6-astra", source_id: "azure" },
    ]) {
      const advanced: Route[] = [
        ...rules,
        { id: "custom", agent: "claude-code", ...extra },
      ];
      expect(readModelSelection(advanced, "copilot")).toBeNull();
      expect(connectionModelRoutes(advanced, "copilot")).toEqual(advanced);
    }
  });
  it("keeps a legacy fixed model explicit and allows converting it to exact rules", () => {
    const fixed = selectedModelRoutes(
      "claude-code",
      "copilot",
      { ...selection, fixed: true },
      [],
    );
    expect(fixed).toHaveLength(1);
    expect(fixed[0].match_model).toBe("*");
    expect(readModelSelection(fixed, "copilot")?.fixed).toBe(true);
    expect(
      selectedModelRoutes("claude-code", "copilot", selection, fixed).every(
        (r) => r.match_model !== "*",
      ),
    ).toBe(true);
  });
  it("does not apply Copilot aliases to Azure deployment names or other Agents", () => {
    expect(
      selectedModelRoutes("codex", "copilot", selection, []).map(
        (r) => r.match_model,
      ),
    ).not.toContain("claude-opus-5-5");
    expect(
      selectedModelRoutes(
        "claude-code",
        "azure-openai",
        { ...selection, source: "azure" },
        [],
      ).map((r) => r.match_model),
    ).not.toContain("claude-opus-5-5");
    expect(() =>
      selectedModelRoutes("codex", "", { ...selection, source: "pool:a" }, []),
    ).toThrow();
    expect(() =>
      selectedModelRoutes("codex", "", { ...selection, models: [] }, []),
    ).toThrow();
  });
});
