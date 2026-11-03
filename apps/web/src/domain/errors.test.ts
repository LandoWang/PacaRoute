import { describe, expect, it } from "vitest";
import { APIError } from "../api/transport";
import { errorMessage } from "./errors";

describe("error messages", () => {
  it("shows the message of API errors and thrown Errors, and stringifies other values", () => {
    expect(errorMessage(new APIError(409, "configuration changed"))).toBe(
      "configuration changed",
    );
    expect(errorMessage(Error("请选择备份"))).toBe("请选择备份");
    expect(errorMessage("plain")).toBe("plain");
  });
});
