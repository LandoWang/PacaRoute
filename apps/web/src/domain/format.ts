export const format = (n: number | null | undefined) =>
  n == null
    ? "—"
    : new Intl.NumberFormat("zh-CN", {
        notation: n >= 100000 ? "compact" : "standard",
        maximumFractionDigits: 1,
      }).format(n);
