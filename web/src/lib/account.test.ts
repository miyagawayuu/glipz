import { describe, expect, it } from "vitest";
import { accountDeletionInputReady } from "./account";

describe("accountDeletionInputReady", () => {
  it("requires a password and exact DELETE confirmation", () => {
    expect(accountDeletionInputReady("", "DELETE")).toBe(false);
    expect(accountDeletionInputReady("password", "delete")).toBe(false);
    expect(accountDeletionInputReady("password", "DELETE")).toBe(true);
  });
});
