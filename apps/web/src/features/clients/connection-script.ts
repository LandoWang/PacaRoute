import { type Agent, type Client } from "../../api/types";
import { enabledAgents } from "../../domain/routing";
export type ScriptPlatform = "shell" | "powershell";
const shellQuote = (v: string) => "'" + v.replaceAll("'", "'\"'\"'") + "'";
// PowerShell also ends single-quoted strings at U+2018..U+201B; double each one.
const psQuote = (v: string) =>
  "'" + v.replace(/['\u2018\u2019\u201a\u201b]/g, "$&$&") + "'";
export function connectionScript(
  client: Client,
  agent: Agent,
  address: string,
  model: string,
  platform: ScriptPlatform,
  apiKey = "",
): string {
  if (agent === "pacaroute")
    throw Error(
      "PacaRoute 使用接入二维码或账号地址与密钥，不需要 Agent 配置脚本。",
    );
  if (client.disabled || !enabledAgents(client).includes(agent))
    throw Error("这个客户端尚未启用此 Agent。");
  // The ID is embedded in script comments; do not rely on server validation.
  if (!/^[a-z][a-z0-9-]{0,62}$/.test(client.id))
    throw Error("客户端 ID 无效。");
  // new URL() strips tab/CR/LF, but the raw address is embedded in the script.
  if (/[\x00-\x1f\x7f]/.test(address))
    throw Error("请输入服务的 HTTP(S) 地址，不要包含密钥或查询参数。");
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
    throw Error("本机 default 客户端只能连接本机地址。");
  if (!model || !/^[A-Za-z0-9][A-Za-z0-9._:/-]{0,159}$/.test(model))
    throw Error("先为这个客户端的 Agent 配置模型。");
  if (
    !client.local &&
    (!apiKey || apiKey.length > 16384 || /[\r\n\x00]/.test(apiKey))
  )
    throw Error("请先保存这个客户端的 Key，再生成接入脚本。");
  // Windows PowerShell 5.1 reads a BOM-less .ps1 as Windows-1252, where UTF-8
  // continuation bytes 0x91-0x94 become quote characters. Keep PowerShell
  // scripts pure ASCII: embed the normalized (punycode, percent-encoded) URL
  // and refuse non-ASCII keys.
  if (platform === "powershell" && /[^\x20-\x7e]/.test(apiKey))
    throw Error("客户端 Key 含有非 ASCII 字符，无法生成 PowerShell 脚本。");
  const target = platform === "powershell" ? url.href : address;
  const q = platform === "shell" ? shellQuote : psQuote;
  const command = `connect --agent ${q(agent)} --url ${q(target.replace(/\/$/, ""))} --model ${q(model)}${client.local ? " --local" : ""} --apply`;
  if (platform === "shell")
    return `#!/usr/bin/env bash
set +x
set -euo pipefail
# Requires PacaRoute CLI 0.8+ on PATH. The CLI backs up the existing config.
# Client: ${client.id}; Agent: ${agent}. Run on the caller's computer.
paca_bin="$(command -v pacaroute || true)"
if [[ -z "$paca_bin" && -x /Applications/PacaRoute.app/Contents/MacOS/pacaroute ]]; then paca_bin=/Applications/PacaRoute.app/Contents/MacOS/pacaroute; fi
[[ -n "$paca_bin" ]] || { echo 'Install PacaRoute CLI 0.8+ or PacaRoute.app in /Applications.' >&2; exit 1; }
${
  client.local
    ? `"$paca_bin" ${command}`
    : `# This script includes this Client's API key. Share only with its caller.
pacaroute_key=${shellQuote(apiKey)}
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
    : `# This script includes this Client's API key. Share only with its caller.
Set-PSDebug -Off
$pacaKey = ${psQuote(apiKey)}
try {
  $pacaKey | & $pacaBinary ${command}
  if ($LASTEXITCODE -ne 0) { throw 'PacaRoute configuration failed.' }
} finally {
  Remove-Variable pacaKey -ErrorAction SilentlyContinue
}`
}
`;
}
