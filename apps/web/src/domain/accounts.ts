import { type Account, type Source, type Model } from "../api/types";
export const accountProvider = (kind: string) =>
  ({
    pacaroute: "PacaRoute",
    github: "GitHub",
    openai: "OpenAI",
    anthropic: "Anthropic",
    azure: "Azure",
    "api-key": "兼容 API Key",
  })[kind] || kind;
export const deviceAccount = (kind: string) =>
  kind === "github" || kind === "azure";
export const compatibleAccount = (source: string, kind: string) =>
  source === "pacaroute"
    ? kind === "pacaroute"
    : source === "copilot"
      ? kind === "github"
      : source === "azure-openai"
        ? kind === "azure"
        : kind === source || kind === "api-key";
export const resourceKind = (account: string) =>
  account === "pacaroute"
    ? "pacaroute"
    : account === "github"
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
export const sharedAzure = (a: Account) =>
  a.kind === "azure" &&
  (a.id === "azure-local" || a.login_mode === "azure-cli");
export const peerCapabilities = (models: Model[]): Source["agent_models"] => ({
  codex: models.filter((m) => m.codex).map((m) => m.id),
  "claude-code": models.filter((m) => m.claude).map((m) => m.id),
});
