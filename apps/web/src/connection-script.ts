import { type Agent, type Client, enabledAgents } from "./api";
export type ScriptPlatform = "shell" | "powershell";
const shellQuote = (v: string) => "'" + v.replaceAll("'", "'\"'\"'") + "'";
const psQuote = (v: string) => "'" + v.replaceAll("'", "''") + "'";
export function connectionScript(
  client: Client,
  agent: Agent,
  address: string,
  model: string,
  platform: ScriptPlatform,
): string {
  if (client.disabled || !enabledAgents(client).includes(agent))
    throw Error("这个 Client 尚未启用此 Agent。");
  const url = new URL(address);
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash
  )
    throw Error("请输入服务的 HTTP(S) 地址，不要包含密钥或查询参数。");
  if (
    client.local &&
    !["localhost", "127.0.0.1", "[::1]"].includes(url.hostname)
  )
    throw Error("本机 default Client 只能连接本机地址。");
  if (!model || !/^[A-Za-z0-9][A-Za-z0-9._:/-]{0,159}$/.test(model))
    throw Error("先在路由页为这个 Agent 配置模型。");
  const q = platform === "shell" ? shellQuote : psQuote;
  const command = `connect --agent ${q(agent)} --url ${q(address.replace(/\/$/, ""))} --model ${q(model)}${client.local ? " --local" : ""} --apply`;
  if (platform === "shell")
    return `#!/usr/bin/env bash
set -euo pipefail
# Requires PacaRoute CLI 0.8+ on PATH. The CLI backs up the existing config.
# Client: ${client.id}; Agent: ${agent}. Run on the caller's computer.
paca_bin="$(command -v pacaroute || true)"
if [[ -z "$paca_bin" && -x /Applications/PacaRoute.app/Contents/MacOS/pacaroute ]]; then paca_bin=/Applications/PacaRoute.app/Contents/MacOS/pacaroute; fi
[[ -n "$paca_bin" ]] || { echo 'Install PacaRoute CLI 0.8+ or PacaRoute.app in /Applications.' >&2; exit 1; }
${
  client.local
    ? `"$paca_bin" ${command}`
    : `read -r -s -p 'Client API key: ' pacaroute_key
printf '\\n'
trap 'unset pacaroute_key' EXIT
printf '%s' "$pacaroute_key" | "$paca_bin" ${command}`
}
`;
  return `# Requires PacaRoute CLI 0.8+ on PATH. Run on the caller's computer.
# Client: ${client.id}; Agent: ${agent}. Existing config is backed up.
$ErrorActionPreference = 'Stop'
$pacaBinary = (Get-Command pacaroute-cli -ErrorAction SilentlyContinue).Source
if (-not $pacaBinary) { $pacaBinary = Join-Path $PSScriptRoot 'pacaroute-cli.exe' }
if (-not (Test-Path $pacaBinary)) { throw 'Put this script beside pacaroute-cli.exe from PacaRoute 0.8+, or add the CLI to PATH.' }
${
  client.local
    ? `& $pacaBinary ${command}\nif ($LASTEXITCODE -ne 0) { throw 'PacaRoute configuration failed.' }`
    : `$pacaSecret = Read-Host 'Client API key' -AsSecureString
$pacaPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($pacaSecret)
try {
  [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pacaPointer) | & $pacaBinary ${command}
  if ($LASTEXITCODE -ne 0) { throw 'PacaRoute configuration failed.' }
} finally {
  [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pacaPointer)
  $pacaSecret.Dispose()
}`
}
`;
}
