import { describe, expect, it } from "vitest";
import { format } from "./format";

describe("number formatting", () => {
  it("keeps standard and compact zh-CN output and the missing marker", () => {
    for (const n of [0, 7, 1234.56, 99999, 100000, 1234567, 98765432.1]) {
      const reference = new Intl.NumberFormat("zh-CN", {
        notation: n >= 100000 ? "compact" : "standard",
        maximumFractionDigits: 1,
      }).format(n);
      expect(format(n)).toBe(reference);
    }
    expect(format(null)).toBe("—");
    expect(format(undefined)).toBe("—");
  });
});
