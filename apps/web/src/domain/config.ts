import { type Config } from "../api/types";
export const accounts = (c: Config) => c.accounts;
export const sources = (c: Config) => c.sources;
export const sourceName = (c: Config, id: string) =>
  id.startsWith("pool:")
    ? c.pools.find((p) => "pool:" + p.id === id)?.name || id
    : sources(c).find((s) => s.id === id)?.name || id;
export const generatedID = (prefix: string) =>
  `${prefix}-${crypto.randomUUID().slice(0, 8)}`;
export function suggestedName(
  label: string,
  items: { name: string }[],
): string {
  const names = new Set(items.map((item) => item.name));
  if (!names.has(label)) return label;
  let suffix = 2;
  while (names.has(`${label} ${suffix}`)) suffix++;
  return `${label} ${suffix}`;
}
// Adds `value`, or replaces the item being edited (`originalID`). A new item
// never replaces an existing one that happens to share its ID.
export function upsertByID<T extends { id: string }>(
  items: T[],
  value: T,
  originalID?: string,
): T[] {
  if (originalID === undefined && items.some((item) => item.id === value.id))
    throw Error("内部 ID 已被使用，请在高级设置中修改");
  return [
    ...items.filter((item) => item.id !== (originalID ?? value.id)),
    value,
  ];
}
