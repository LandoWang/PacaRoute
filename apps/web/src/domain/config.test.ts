import { describe, expect, it } from "vitest";
import { upsertByID } from "./config";

describe("configuration list updates", () => {
  const items = [
    { id: "a", name: "A" },
    { id: "b", name: "B" },
  ];
  it("refuses to add an item whose ID is already used", () => {
    expect(() => upsertByID(items, { id: "a", name: "New" })).toThrow(
      "内部 ID",
    );
  });
  it("adds new items and replaces only the item being edited", () => {
    expect(upsertByID(items, { id: "c", name: "C" })).toEqual([
      ...items,
      { id: "c", name: "C" },
    ]);
    expect(upsertByID(items, { id: "a", name: "A2" }, "a")).toEqual([
      { id: "b", name: "B" },
      { id: "a", name: "A2" },
    ]);
  });
});
