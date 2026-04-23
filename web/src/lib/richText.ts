export type RichTextSegment =
  | { type: "text"; value: string }
  | { type: "link"; value: string; href: string }
  | { type: "hashtag"; value: string; tag: string }
  | { type: "emoji_shortcode"; value: string };

export type RichTextLine = {
  key: string;
  segments: RichTextSegment[];
};

const URL_RE = /https?:\/\/[^\s<]+/g;
const TRAILING_PUNCTUATION_RE = /[),.!?:;'"\]\u3001\u3002]+$/;

export function parseRichText(text: string): RichTextLine[] {
  return text.split(/\r?\n/).map((line, lineIndex) => ({
    key: `line-${lineIndex}`,
    segments: parseLine(line),
  }));
}

export function extractPreviewUrls(text: string): string[] {
  const seen = new Set<string>();
  const urls: string[] = [];
  for (const line of parseRichText(text)) {
    for (const segment of line.segments) {
      if (segment.type !== "link") continue;
      if (seen.has(segment.href)) continue;
      seen.add(segment.href);
      urls.push(segment.href);
    }
  }
  return urls;
}

function parseLine(line: string): RichTextSegment[] {
  const segments: RichTextSegment[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  URL_RE.lastIndex = 0;
  while ((match = URL_RE.exec(line)) !== null) {
    const raw = match[0];
    const normalized = normalizeUrlToken(raw);
    if (match.index > lastIndex) {
      segments.push(...parseTextSegment(line.slice(lastIndex, match.index)));
    }
    if (normalized.url) {
      segments.push({ type: "link", value: normalized.url, href: normalized.url });
    } else {
      segments.push(...parseTextSegment(raw));
    }
    if (normalized.trailing) {
      segments.push(...parseTextSegment(normalized.trailing));
    }
    lastIndex = match.index + raw.length;
  }
  if (lastIndex < line.length) {
    segments.push(...parseTextSegment(line.slice(lastIndex)));
  }
  if (!segments.length) {
    segments.push({ type: "text", value: line });
  }
  return segments;
}

function parseTextSegment(text: string): RichTextSegment[] {
  if (!text) return [];
  const chars = Array.from(text);
  const out: RichTextSegment[] = [];
  let cursor = 0;
  let textStart = 0;
  while (cursor < chars.length) {
    if (isShortcodeStart(chars, cursor)) {
      if (cursor > textStart) {
        out.push({ type: "text", value: chars.slice(textStart, cursor).join("") });
      }
      const end = shortcodeEndIndex(chars, cursor);
      out.push({ type: "emoji_shortcode", value: chars.slice(cursor, end).join("") });
      cursor = end;
      textStart = end;
      continue;
    }
    if (!isHashtagStart(chars, cursor)) {
      cursor++;
      continue;
    }
    if (cursor > textStart) {
      out.push({ type: "text", value: chars.slice(textStart, cursor).join("") });
    }
    let end = cursor + 1;
    while (end < chars.length && isHashtagChar(chars[end])) end++;
    const value = chars.slice(cursor, end).join("");
    const tag = chars
      .slice(cursor + 1, end)
      .join("")
      .toLowerCase();
    out.push({ type: "hashtag", value, tag });
    cursor = end;
    textStart = end;
  }
  if (textStart < chars.length) {
    out.push({ type: "text", value: chars.slice(textStart).join("") });
  }
  return out.length ? out : [{ type: "text", value: text }];
}

function isShortcodeStart(chars: string[], index: number): boolean {
  if (chars[index] !== ":") return false;
  const end = shortcodeEndIndex(chars, index);
  return end > index + 2;
}

function shortcodeEndIndex(chars: string[], index: number): number {
  let end = index + 1;
  while (end < chars.length && chars[end] !== ":") {
    end++;
  }
  if (end >= chars.length || chars[end] !== ":") return -1;
  const body = chars.slice(index + 1, end).join("");
  if (!/^[a-z0-9_]+(?:@[a-z0-9._-]+)?$/i.test(body)) return -1;
  return end + 1;
}

function isHashtagStart(chars: string[], index: number): boolean {
  if (chars[index] !== "#") return false;
  const next = chars[index + 1];
  if (!next || !isHashtagChar(next)) return false;
  const prev = chars[index - 1];
  if (!prev) return true;
  return !isHashtagChar(prev) && prev !== "/" && prev !== "#";
}

function isHashtagChar(ch: string): boolean {
  return /[\p{L}\p{N}\p{M}_]/u.test(ch);
}

function normalizeUrlToken(raw: string): { url: string; trailing: string } {
  const trailing = raw.match(TRAILING_PUNCTUATION_RE)?.[0] ?? "";
  const candidate = trailing ? raw.slice(0, raw.length - trailing.length) : raw;
  try {
    const url = new URL(candidate);
    if (url.protocol !== "http:" && url.protocol !== "https:") {
      return { url: "", trailing: "" };
    }
    return { url: url.toString(), trailing };
  } catch {
    return { url: "", trailing: "" };
  }
}
