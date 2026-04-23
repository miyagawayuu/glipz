import DOMPurify from "dompurify";
import { marked } from "marked";

marked.setOptions({ gfm: true, breaks: true });

/** Converts note-body Markdown into sanitized display HTML. */
export function renderNoteMarkdown(md: string): string {
  const raw = marked.parse(md || "", { async: false });
  const html = typeof raw === "string" ? raw : String(raw);
  return DOMPurify.sanitize(html, {
    ADD_TAGS: ["video", "source"],
    ADD_ATTR: ["controls", "playsinline", "src", "type", "class", "alt", "loading", "href", "target", "rel", "width", "height"],
  });
}
