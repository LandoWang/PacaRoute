// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { initialPage, pages } from "./pages";

afterEach(() => history.replaceState(null, "", "/"));

describe("page navigation", () => {
  it("keeps the existing #/<id> URLs in sidebar order", () => {
    expect(pages.map((p) => p.id)).toEqual([
      "overview",
      "accounts",
      "sources",
      "pools",
      "proxies",
      "clients",
      "usage",
      "settings",
      "errors",
    ]);
  });
  it("resolves the hash, ignores its query and redirects legacy #/routes", () => {
    history.replaceState(null, "", "/#/usage?ticket=x");
    expect(initialPage()).toBe("usage");
    history.replaceState(null, "", "/#/routes?client=alice");
    expect(initialPage()).toBe("clients");
    expect(location.hash).toBe("#/clients?client=alice");
    history.replaceState(null, "", "/#/unknown");
    expect(initialPage()).toBe("overview");
  });
});
