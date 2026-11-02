import { Server } from "lucide-react";
import github from "../assets/brands/github.svg";
import copilot from "../assets/brands/githubcopilot.svg";
import foundry from "../assets/brands/foundry.svg";
import azure from "../assets/brands/azure-icon.svg";
import openai from "../assets/brands/openai-icon.svg";
import anthropic from "../assets/brands/anthropic.svg";

export function BrandIcon({
  kind,
  account = false,
  size = 24,
}: {
  kind: string;
  account?: boolean;
  size?: number;
}) {
  const brand =
    kind === "github"
      ? account
        ? { src: github, label: "GitHub", mono: true }
        : { src: copilot, label: "GitHub Copilot", mono: true }
      : kind === "copilot"
        ? { src: copilot, label: "GitHub Copilot", mono: true }
        : kind === "azure" || kind === "azure-openai"
          ? account
            ? { src: azure, label: "Azure", mono: false }
            : { src: foundry, label: "Azure Foundry", mono: false }
          : kind === "openai"
            ? { src: openai, label: "OpenAI", mono: true }
            : kind === "anthropic"
              ? { src: anthropic, label: "Anthropic", mono: true }
              : undefined;
  return brand ? (
    <img
      src={brand.src}
      alt={brand.label}
      width={size}
      height={size}
      className={"brand-icon" + (brand.mono ? " brand-monochrome" : "")}
    />
  ) : (
    <Server size={size} aria-hidden="true" />
  );
}
