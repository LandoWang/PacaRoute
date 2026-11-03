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
