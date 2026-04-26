import DOMPurify from "dompurify";
import { marked } from "marked";
import { enforceSafeLinkAttrs } from "./sanitizeHtml";

marked.setOptions({ gfm: true, breaks: true });

/** Converts note-body Markdown into sanitized display HTML. */
export function renderNoteMarkdown(md: string): string {
  const raw = marked.parse(md || "", { async: false });
  const html = typeof raw === "string" ? raw : String(raw);
  const sanitized = DOMPurify.sanitize(html, {
    ADD_TAGS: ["video", "source"],
    ADD_ATTR: ["controls", "playsinline", "src", "type", "class", "alt", "loading", "href", "target", "rel", "width", "height"],
    FORBID_TAGS: ["script", "style", "svg", "math"],
    FORBID_ATTR: ["onerror", "onload", "onclick", "style"],
  });
  return enforceSafeLinkAttrs(sanitized);
}
