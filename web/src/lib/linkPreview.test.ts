// @vitest-environment jsdom

import { describe, expect, it } from "vitest";
import { normalizeLinkPreview } from "./linkPreview";

describe("normalizeLinkPreview", () => {
  it("keeps http URLs and removes unsafe image URLs", () => {
    expect(
      normalizeLinkPreview({
        url: "https://example.com/post",
        title: "Example",
        image_url: "javascript:alert(1)",
      }),
    ).toEqual({
      url: "https://example.com/post",
      title: "Example",
      image_url: undefined,
    });
  });

  it("rejects previews with unsafe target URLs", () => {
    expect(
      normalizeLinkPreview({
        url: "javascript:alert(1)",
        title: "Bad",
        image_url: "https://example.com/image.png",
      }),
    ).toBeNull();
  });
});
