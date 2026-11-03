export type PeerConnection = {
  version: 1;
  endpoint: string;
  client_key: string;
  name?: string;
};

const prefix = "pacaroute://connect#";
export function normalizePeerEndpoint(input: string): string {
  let url: URL;
  try {
    url = new URL(input.trim());
  } catch {
    throw Error("请输入完整的 HTTP / HTTPS 服务地址");
  }
  if (
    !["http:", "https:"].includes(url.protocol) ||
    !url.hostname ||
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    input.length > 1024
  )
    throw Error("服务地址不能包含账号密码、查询参数或片段");
  return url.href.replace(/\/+$/, "");
}

function validate(value: unknown): PeerConnection {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw Error("不是 PacaRoute 接入信息");
  const v = value as Record<string, unknown>;
  if (
    v.version !== 1 ||
    Object.keys(v).some(
      (k) => !["version", "endpoint", "client_key", "name"].includes(k),
    )
  )
    throw Error("不支持的 PacaRoute 接入格式或版本");
  if (
    typeof v.endpoint !== "string" ||
    typeof v.client_key !== "string" ||
    !/^[\x21-\x7e]{8,512}$/.test(v.client_key)
  )
    throw Error("接入信息缺少有效服务地址或客户端密钥");
  if (
    v.name !== undefined &&
    (typeof v.name !== "string" || v.name.length > 120)
  )
    throw Error("接入名称过长或无效");
  return {
    version: 1,
    endpoint: normalizePeerEndpoint(v.endpoint),
    client_key: v.client_key,
    ...(v.name ? { name: v.name as string } : {}),
  };
}

// This string is exchanged as data. Never navigate to it or persist it in URLs.
export function encodePeerConnection(value: PeerConnection): string {
  const bytes = new TextEncoder().encode(JSON.stringify(validate(value)));
  const encoded =
    prefix +
    btoa(String.fromCharCode(...bytes))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "");
  if (encoded.length > 2300) throw Error("接入信息过长，请缩短地址或名称");
  return encoded;
}

export function parsePeerConnection(input: string): PeerConnection {
  const text = input.trim();
  if (text.length > 4096) throw Error("接入信息过长");
  let json = text;
  if (text.startsWith(prefix)) {
    const encoded = text.slice(prefix.length);
    if (!/^[A-Za-z0-9_-]+$/.test(encoded)) throw Error("二维码接入信息无效");
    try {
      json = new TextDecoder("utf-8", { fatal: true }).decode(
        Uint8Array.from(
          atob(encoded.replace(/-/g, "+").replace(/_/g, "/")),
          (c) => c.charCodeAt(0),
        ),
      );
    } catch {
      throw Error("二维码接入信息无效");
    }
  }
  let value: unknown;
  try {
    value = JSON.parse(json);
  } catch {
    throw Error("请粘贴 PacaRoute 生成的接入信息，或选择它的二维码图片");
  }
  return validate(value);
}
