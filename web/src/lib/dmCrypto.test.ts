// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { isAllowedDMAttachmentURL } from "./dmCrypto";

describe("isAllowedDMAttachmentURL", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("rejects extra base URLs that only allow an origin root", () => {
    vi.stubEnv("VITE_ALLOWED_DM_ATTACHMENT_BASE_URLS", "https://media.example.com/");
    expect(isAllowedDMAttachmentURL("https://media.example.com/anything.bin")).toBe(false);
  });

  it("allows extra base URLs with a non-root path prefix", () => {
    vi.stubEnv("VITE_ALLOWED_DM_ATTACHMENT_BASE_URLS", "https://media.example.com/dm/");
    expect(isAllowedDMAttachmentURL("https://media.example.com/dm/thread/file.bin")).toBe(true);
    expect(isAllowedDMAttachmentURL("https://media.example.com/public/file.bin")).toBe(false);
  });
});
