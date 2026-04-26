import DOMPurify from "dompurify";

export function enforceSafeLinkAttrs(html: string): string {
  if (typeof document === "undefined") return html;
  const root = document.createElement("div");
  root.innerHTML = html;
  for (const a of Array.from(root.querySelectorAll("a"))) {
    const href = a.getAttribute("href") || "";
    if (href && !/^(https?:|mailto:)/i.test(href)) {
      a.removeAttribute("href");
    }
    if (a.getAttribute("target") === "_blank") {
      a.setAttribute("rel", "noopener noreferrer");
    }
  }
  return root.innerHTML;
}

export function sanitizeInlineHtml(html: string): string {
  const sanitized = DOMPurify.sanitize(html || "", {
    ALLOWED_TAGS: ["a", "br", "code", "em", "strong"],
    ALLOWED_ATTR: ["href", "rel", "target"],
    FORBID_TAGS: ["script", "style", "svg", "math"],
    FORBID_ATTR: ["onerror", "onload", "onclick", "style"],
  });
  return enforceSafeLinkAttrs(sanitized);
}

export function sanitizeRemoteProfileSummary(html: string): string {
  const sanitized = DOMPurify.sanitize(html || "", {
    ALLOWED_TAGS: ["p", "br", "a", "span", "strong", "em", "b", "i", "ul", "ol", "li", "code"],
    ALLOWED_ATTR: ["href", "target", "rel"],
    FORBID_TAGS: ["script", "style", "svg", "math", "iframe", "object", "embed"],
    FORBID_ATTR: ["onerror", "onload", "onclick", "style"],
  });
  return enforceSafeLinkAttrs(sanitized);
}
