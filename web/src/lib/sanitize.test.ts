// @vitest-environment jsdom

import { describe, expect, it } from "vitest";
import { renderLegalMarkdown } from "./legalDocs";
import { renderNoteMarkdown } from "./noteRender";
import { sanitizeInlineHtml, sanitizeRemoteProfileSummary } from "./sanitizeHtml";

const maliciousMarkdown = `
[bad](javascript:alert(1))
<img src=x onerror="alert(1)">
<svg><script>alert(1)</script></svg>
<video controls onload="alert(1)"><source src="javascript:alert(1)" type="video/mp4"></video>
`;

function expectSanitized(html: string) {
  expect(html).not.toContain("javascript:");
  expect(html).not.toContain("onerror");
  expect(html).not.toContain("onload");
  expect(html).not.toContain("<script");
  expect(html).not.toContain("<svg");
}

describe("markdown sanitizers", () => {
  it("sanitizes note markdown", () => {
    expectSanitized(renderNoteMarkdown(maliciousMarkdown));
  });

  it("sanitizes legal markdown", () => {
    const html = renderLegalMarkdown(`${maliciousMarkdown}\n\n# Title\n\n| A | B |\n| - | - |\n| 1 | 2 |`);
    expect(html).toContain("<h1>Title</h1>");
    expect(html).toContain("<table>");
    expect(html).not.toContain("<img");
    expect(html).not.toContain("<video");
    expectSanitized(html);
  });

  it("sanitizes inline federation guideline html", () => {
    const html = sanitizeInlineHtml("<code>ok</code><img src=x onerror='alert(1)'><script>alert(1)</script>");
    expect(html).toContain("<code>ok</code>");
    expectSanitized(html);
  });

  it("sanitizes remote profile summaries used with v-html", () => {
    const html = sanitizeRemoteProfileSummary(`
      <p onclick="alert(1)">hello <strong>remote</strong></p>
      <a href="javascript:alert(1)" target="_blank">bad</a>
      <a href="https://example.com" target="_blank">ok</a>
      <iframe src="https://example.com"></iframe>
      <svg><script>alert(1)</script></svg>
    `);
    expect(html).toContain("<strong>remote</strong>");
    expect(html).toContain('href="https://example.com"');
    expect(html).toContain('rel="noopener noreferrer"');
    expect(html).not.toContain("onclick");
    expect(html).not.toContain("javascript:");
    expect(html).not.toContain("<iframe");
    expect(html).not.toContain("<svg");
    expect(html).not.toContain("<script");
  });
});
