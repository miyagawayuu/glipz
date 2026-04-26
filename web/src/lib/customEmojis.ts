import emojiKeywords from "emojilib";
import { shallowRef } from "vue";
import twemoji from "twemoji";
import { api, apiPublicGet } from "./api";
import { safeHttpURL } from "./redirect";
import type { CustomEmoji } from "../types/customEmoji";
import { unicodeEmojiByGroup } from "../data/unicodeEmojiByGroup";

const shortcodePattern = /^:([a-z0-9_]+(?:@[a-z0-9._-]+)?):$/i;
const TWEMOJI_BASE_URL = "https://cdn.jsdelivr.net/gh/twitter/twemoji@14.0.2/assets/svg";

export type UnicodeEmojiPickerItem = {
  emoji: string;
  name: string;
  slug: string;
  skinToneSupport: boolean;
};

export type UnicodeEmojiPickerCategory = {
  slug: string;
  name: string;
  emojis: UnicodeEmojiPickerItem[];
};

const customEmojiMapRef = shallowRef<Record<string, CustomEmoji>>({});
const remoteEmojiImageUrlRef = shallowRef<Record<string, string>>({});
let loadPromise: Promise<Record<string, CustomEmoji>> | null = null;
let remoteResolvePromise: Promise<void> | null = null;

const standardShortcodeMap = (() => {
  const map = new Map<string, string>();
  for (const [emoji, keywords] of Object.entries(emojiKeywords)) {
    for (const keyword of keywords) {
      const normalized = String(keyword).trim().toLowerCase();
      if (!normalized || normalized.includes(" ")) continue;
      if (!/^[a-z0-9_+\-]+$/.test(normalized)) continue;
      if (!map.has(normalized)) map.set(normalized, emoji);
    }
  }
  map.set("heart", "\u2764\ufe0f");
  map.set("+1", "\ud83d\udc4d");
  map.set("thumbsup", "\ud83d\udc4d");
  map.set("smile", "\ud83d\ude04");
  map.set("tada", "\ud83c\udf89");
  return map;
})();

export function customEmojiMap() {
  return customEmojiMapRef;
}

export async function ensureCustomEmojiCatalog(token?: string | null): Promise<Record<string, CustomEmoji>> {
  if (Object.keys(customEmojiMapRef.value).length > 0) return customEmojiMapRef.value;
  if (!loadPromise) {
    loadPromise = (async () => {
      const res = token
        ? await api<{ items?: CustomEmoji[] }>("/api/v1/custom-emojis", { method: "GET", token })
        : await apiPublicGet<{ items?: CustomEmoji[] }>("/api/v1/custom-emojis");
      const next: Record<string, CustomEmoji> = {};
      for (const item of res.items ?? []) {
        const imageURL = safeHttpURL(item?.image_url);
        if (!item?.shortcode || !imageURL) continue;
        next[item.shortcode] = { ...item, image_url: imageURL };
      }
      customEmojiMapRef.value = next;
      return next;
    })().finally(() => {
      loadPromise = null;
    });
  }
  return loadPromise;
}

export async function refreshCustomEmojiCatalog(token?: string | null): Promise<Record<string, CustomEmoji>> {
  customEmojiMapRef.value = {};
  loadPromise = null;
  return ensureCustomEmojiCatalog(token);
}

export function normalizeEmojiToken(raw: string): string {
  return raw.trim();
}

export function isShortcodeToken(raw: string): boolean {
  return shortcodePattern.test(normalizeEmojiToken(raw));
}

export function shortcodeBody(raw: string): string | null {
  const m = normalizeEmojiToken(raw).match(shortcodePattern);
  return m?.[1]?.toLowerCase() ?? null;
}

function twemojiUrlForUnicode(raw: string): string | undefined {
  const token = normalizeEmojiToken(raw);
  if (!token) return undefined;
  let codepoint = "";
  twemoji.parse(token, {
    callback: (icon) => {
      codepoint = icon;
      return false;
    },
  });
  return codepoint ? `${TWEMOJI_BASE_URL}/${codepoint}.svg` : undefined;
}

export function resolveEmojiToken(raw: string): { kind: "unicode" | "custom" | "text"; text: string; imageUrl?: string } {
  const token = normalizeEmojiToken(raw);
  if (!token) return { kind: "text", text: "" };
  const custom = customEmojiMapRef.value[token];
  if (custom) {
    return { kind: "custom", text: token, imageUrl: custom.image_url };
  }
  const remoteUrl = remoteEmojiImageUrlRef.value[token];
  if (remoteUrl) {
    return { kind: "custom", text: token, imageUrl: remoteUrl };
  }
  const shortcode = shortcodeBody(token);
  if (shortcode) {
    const unicode = standardShortcodeMap.get(shortcode);
    if (unicode) return { kind: "unicode", text: unicode, imageUrl: twemojiUrlForUnicode(unicode) };
    return { kind: "text", text: token };
  }
  const imageUrl = twemojiUrlForUnicode(token);
  if (imageUrl) return { kind: "unicode", text: token, imageUrl };
  return { kind: "text", text: token };
}

export async function ensureRemoteCustomEmojiResolved(token: string): Promise<void> {
  const normalized = normalizeEmojiToken(token);
  if (!normalized) return;
  if (customEmojiMapRef.value[normalized]) return;
  if (remoteEmojiImageUrlRef.value[normalized]) return;
  if (!isShortcodeToken(normalized)) return;
  const body = shortcodeBody(normalized);
  if (!body || !body.includes("@")) return; // only remote-scoped shortcodes

  if (!remoteResolvePromise) {
    remoteResolvePromise = Promise.resolve().finally(() => {
      remoteResolvePromise = null;
    });
  }
  await remoteResolvePromise;

  try {
    const res = await apiPublicGet<{ image_url?: string }>(`/api/v1/public/federation/custom-emoji?shortcode=${encodeURIComponent(normalized)}`);
    const url = safeHttpURL(res?.image_url);
    if (!url) return;
    remoteEmojiImageUrlRef.value = { ...remoteEmojiImageUrlRef.value, [normalized]: url };
  } catch {
    // best-effort
  }
}

export function unicodeReactionPickerCategories(): UnicodeEmojiPickerCategory[] {
  return unicodeEmojiByGroup.map((group) => ({
    slug: group.slug,
    name: group.name,
    emojis: group.emojis.map((emoji) => ({
      emoji: emoji.emoji,
      name: emoji.name,
      slug: emoji.slug,
      skinToneSupport: emoji.skin_tone_support,
    })),
  }));
}

export function pickerCustomEmojisForHandle(handle: string | null | undefined): CustomEmoji[] {
  const normalized = (handle ?? "").trim().toLowerCase();
  return Object.values(customEmojiMapRef.value)
    .filter((item) => item.scope === "site" || (normalized && item.owner_handle?.toLowerCase() === normalized))
    .sort((a, b) => a.shortcode.localeCompare(b.shortcode));
}
