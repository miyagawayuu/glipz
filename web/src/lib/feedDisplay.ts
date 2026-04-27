import type { TimelinePost } from "../types/timeline";
import { formatDateTime, formatRelativeTime as formatRelativeTimeLocalized, formatScheduledAt } from "../i18n";
import { displayInstanceDomain } from "./api";

/** Builds a 2x2 media grid, padding the bottom-right slot when there are exactly three images. */
export function gridSlots(urls: string[]): (string | null)[] {
  if (!urls.length) return [];
  if (urls.length === 3) return [...urls, null];
  return urls.slice(0, 4);
}

/** Maps a grid slot position back to the index inside `media_urls`, even when URLs are duplicated. */
export function mediaIndexFromGridSlot(urls: string[], slotGi: number): number {
  const slots = gridSlots(urls);
  let idx = 0;
  for (let i = 0; i < slotGi; i++) {
    if (slots[i] != null) idx++;
  }
  return idx;
}

export function displayName(email: string): string {
  const local = email.split("@")[0] ?? email;
  return local || email;
}

export function fullHandleAt(handle: string): string {
  const h = handle.trim().replace(/^@/, "");
  const domain = displayInstanceDomain();
  if (!h) return domain ? `@user@${domain}` : "@user";
  if (h.includes("@")) return `@${h}`;
  return domain ? `@${h}@${domain}` : `@${h}`;
}

/** Prefers `user_display_name` from the feed API and falls back to the email-derived name. */
export function timelineDisplayName(it: Pick<TimelinePost, "user_email"> & { user_display_name?: string }): string {
  const n = it.user_display_name?.trim();
  if (n) return n;
  return displayName(it.user_email);
}

export function handleFromEmail(email: string): string {
  const local = (email.split("@")[0] ?? "user").replace(/[^a-zA-Z0-9_]/g, "");
  return fullHandleAt(local || "user");
}

/** Display-ready @handle, preferring the API-provided user_handle. */
export function handleAt(it: Pick<TimelinePost, "user_handle"> & Partial<Pick<TimelinePost, "user_email">>): string {
  const h = it.user_handle?.trim();
	if (!h) return handleFromEmail(it.user_email ?? "");
  return fullHandleAt(h);
}

export function profilePath(it: Pick<TimelinePost, "user_handle" | "user_email">): string {
  const h = it.user_handle?.trim();
  if (h) return `/@${h}`;
  return "/feed";
}

const FEDERATED_POST_ID_PREFIX = "federated:";

/** Resolves a federated author into a Glipz remote-profile route such as `/@user@host` or `?actor=`. */
export function federatedRemoteProfilePath(
  it: Pick<TimelinePost, "user_handle" | "remote_actor_url">,
): string | null {
  const h = (it.user_handle ?? "").trim();
  if (h.includes("@")) {
    return `/@${h.replace(/^@/, "")}`;
  }
  const url = (it.remote_actor_url ?? "").trim();
  if (url) return `/remote/profile?actor=${encodeURIComponent(url)}`;
  return null;
}

/** Route path for the post detail page. */
export function postDetailPath(postId: string): string {
  if (postId.startsWith(FEDERATED_POST_ID_PREFIX)) {
    const rest = postId.slice(FEDERATED_POST_ID_PREFIX.length).trim();
    if (rest) return `/posts/federated/${rest}`;
  }
  return `/posts/${encodeURIComponent(postId)}`;
}

export function avatarInitials(email: string): string {
  const local = email.split("@")[0] ?? "";
  const cleaned = local.replace(/[^a-zA-Z0-9]/g, "");
  if (cleaned.length >= 2) return cleaned.slice(0, 2).toUpperCase();
  if (local.length >= 2) return local.slice(0, 2).toUpperCase();
  return (local[0] ?? "?").toUpperCase();
}

export function formatRelativeTime(iso: string): string {
  return formatRelativeTimeLocalized(iso);
}

/** Canonical publish timestamp used for display. visible_at remains the source of truth even for immediate posts. */
export function postPublishedAtISO(it: Pick<TimelinePost, "visible_at" | "created_at">): string {
  const v = it.visible_at?.trim();
  if (v) return v;
  return it.created_at?.trim() ?? "";
}

/** Timeline time label. Future scheduled posts use an absolute scheduled-at label. */
export function formatPostTime(it: Pick<TimelinePost, "visible_at" | "created_at">): string {
  const iso = postPublishedAtISO(it);
  if (!iso) return "";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "";
  const diffSec = Math.floor((Date.now() - t) / 1000);
  if (diffSec < 0) {
    return formatScheduledAt(iso);
  }
  return formatRelativeTime(iso);
}

export function formatAbsoluteDateTime(
  iso: string,
  options?: Intl.DateTimeFormatOptions,
): string {
  return formatDateTime(iso, options ?? { dateStyle: "short", timeStyle: "short" });
}

export function formatActionCount(n: number): string {
  if (n <= 0) return "";
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${(n / 1000).toFixed(1).replace(/\.0$/, "")}K`;
  return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
}
