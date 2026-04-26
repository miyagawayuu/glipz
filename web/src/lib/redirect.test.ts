// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { safeHttpURL, safeMediaURL, safeRelativeRoute } from "./redirect";

describe("safeRelativeRoute", () => {
  it("allows normal in-app paths", () => {
    expect(safeRelativeRoute("/feed?tab=home", "/fallback")).toBe("/feed?tab=home");
  });

  it("rejects external and ambiguous paths", () => {
    expect(safeRelativeRoute("//evil.example/path", "/fallback")).toBe("/fallback");
    expect(safeRelativeRoute("https://evil.example", "/fallback")).toBe("/fallback");
    expect(safeRelativeRoute("/\\evil", "/fallback")).toBe("/fallback");
    expect(safeRelativeRoute("/foo:bar", "/fallback")).toBe("/fallback");
  });
});

describe("safeHttpURL", () => {
  it("allows http and https URLs", () => {
    expect(safeHttpURL("https://example.com/a")).toBe("https://example.com/a");
    expect(safeHttpURL("http://example.com/a")).toBe("http://example.com/a");
  });

  it("rejects non-http URLs", () => {
    expect(safeHttpURL("javascript:alert(1)")).toBe("");
    expect(safeHttpURL("data:text/html,hi")).toBe("");
  });
});

describe("safeMediaURL", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("allows media URLs on the same origin and configured API origin", () => {
    vi.stubEnv("VITE_API_URL", "https://api.example.com");
    expect(safeMediaURL("/api/v1/media/image.jpg")).toBe(`${window.location.origin}/api/v1/media/image.jpg`);
    expect(safeMediaURL("https://api.example.com/api/v1/media/image.jpg")).toBe(
      "https://api.example.com/api/v1/media/image.jpg",
    );
  });

  it("allows configured non-root media prefixes", () => {
    vi.stubEnv("VITE_ALLOWED_MEDIA_BASE_URLS", "https://cdn.example.com/media/");
    expect(safeMediaURL("https://cdn.example.com/media/image.jpg")).toBe("https://cdn.example.com/media/image.jpg");
  });

  it("rejects arbitrary external and non-http URLs", () => {
    vi.stubEnv("VITE_ALLOWED_MEDIA_BASE_URLS", "https://cdn.example.com/");
    expect(safeMediaURL("https://evil.example/media/image.jpg")).toBe("");
    expect(safeMediaURL("https://cdn.example.com/anything.jpg")).toBe("");
    expect(safeMediaURL("javascript:alert(1)")).toBe("");
    expect(safeMediaURL("data:text/html,hi")).toBe("");
  });
});
