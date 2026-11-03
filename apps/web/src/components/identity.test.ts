import { describe, expect, it } from "vitest";
import { identifier } from "./identity";

describe("internal ID input pattern", () => {
  it("compiles as browsers do (v flag) and matches the server identifier rule", () => {
    const pattern = new RegExp(`^(?:${identifier.pattern})$`, "v");
    const server = /^[a-z][a-z0-9-]{0,62}$/;
    for (const id of [
      "pool-a",
      "a",
      "source-1",
      "a" + "b".repeat(62),
      "a" + "b".repeat(63),
      "Pool",
      "1pool",
      "-pool",
      "pool_a",
      "",
    ])
      expect(pattern.test(id)).toBe(server.test(id));
  });
});
