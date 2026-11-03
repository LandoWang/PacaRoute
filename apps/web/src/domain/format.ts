// Constructing Intl.NumberFormat is costly; usage tables format every cell.
const formatters = {
  compact: new Intl.NumberFormat("zh-CN", {
    notation: "compact",
    maximumFractionDigits: 1,
  }),
  standard: new Intl.NumberFormat("zh-CN", {
    notation: "standard",
    maximumFractionDigits: 1,
  }),
};
export const format = (n: number | null | undefined) =>
  n == null ? "—" : formatters[n >= 100000 ? "compact" : "standard"].format(n);
