import { api } from "./api";
import { mapFeedItem } from "./feedStream";
import type { TimelinePost } from "../types/timeline";

function federationIncomingId(id: string): string {
  return id.startsWith("federated:") ? id.slice("federated:".length) : id;
}

function federationActionPath(it: TimelinePost, suffix: "like" | "poll/vote" | "unlock" | "repost" | "bookmark" | "reactions"): string {
  if (!it.is_federated) {
    if (suffix === "like") return `/api/v1/posts/${encodeURIComponent(it.id)}/like`;
    if (suffix === "poll/vote") return `/api/v1/posts/${encodeURIComponent(it.id)}/poll/vote`;
    if (suffix === "repost") return `/api/v1/posts/${encodeURIComponent(it.id)}/repost`;
    if (suffix === "bookmark") return `/api/v1/posts/${encodeURIComponent(it.id)}/bookmark`;
    if (suffix === "reactions") return `/api/v1/posts/${encodeURIComponent(it.id)}/reactions`;
    return `/api/v1/posts/${encodeURIComponent(it.id)}/unlock`;
  }
  const params = new URLSearchParams();
  if (it.remote_object_url) {
    params.set("object_url", it.remote_object_url);
  }
  const q = params.toString();
  const base = `/api/v1/federation/posts/${encodeURIComponent(federationIncomingId(it.id))}/${suffix}`;
  return q ? `${base}?${q}` : base;
}

export async function toggleTimelineLike(token: string, it: TimelinePost): Promise<{ liked: boolean; like_count: number }> {
  const path = federationActionPath(it, "like");
  return api<{ liked: boolean; like_count: number }>(path, {
    method: "POST",
    token,
  });
}

export async function addTimelineReaction(token: string, it: TimelinePost, emoji: string): Promise<TimelinePost> {
  const path = federationActionPath(it, "reactions");
  const res = await api<{ item: Record<string, unknown> }>(path, {
    method: "POST",
    token,
    json: { emoji },
  });
  return mapFeedItem(res.item as Parameters<typeof mapFeedItem>[0]);
}

export async function removeTimelineReaction(token: string, it: TimelinePost, emoji: string): Promise<TimelinePost> {
  const encodedEmoji = encodeURIComponent(emoji);
  const path = `${federationActionPath(it, "reactions")}/${encodedEmoji}`;
  const res = await api<{ item: Record<string, unknown> }>(path, {
    method: "DELETE",
    token,
  });
  return mapFeedItem(res.item as Parameters<typeof mapFeedItem>[0]);
}

export async function toggleTimelineBookmark(token: string, it: TimelinePost): Promise<{ bookmarked: boolean }> {
  const path = federationActionPath(it, "bookmark");
  return api<{ bookmarked: boolean }>(path, {
    method: "POST",
    token,
  });
}

export async function voteTimelinePoll(token: string, it: TimelinePost, optionId: string): Promise<TimelinePost> {
  const path = federationActionPath(it, "poll/vote");
  const res = await api<{ item: Record<string, unknown> }>(path, {
    method: "POST",
    token,
    json: { option_id: optionId },
  });
  return mapFeedItem(res.item as Parameters<typeof mapFeedItem>[0]);
}

export async function unlockTimelinePost(token: string, it: TimelinePost, password: string): Promise<Partial<TimelinePost>> {
  const path = federationActionPath(it, "unlock");
  const res = await api<Record<string, unknown>>(path, {
    method: "POST",
    token,
    json: { password },
  });
  const mapped = mapFeedItem({
    id: it.id,
    user_email: it.user_email,
    user_handle: it.user_handle,
    user_display_name: it.user_display_name,
    user_avatar_url: it.user_avatar_url,
    caption: String(res.caption ?? it.caption ?? ""),
    media_type: String(res.media_type ?? it.media_type ?? "none"),
    media_urls: Array.isArray(res.media_urls) ? (res.media_urls as string[]) : [],
    is_nsfw: Boolean(res.is_nsfw),
    has_view_password: Boolean(res.has_view_password),
    view_password_scope: typeof res.view_password_scope === "number" ? res.view_password_scope : 0,
    view_password_text_ranges: Array.isArray(res.view_password_text_ranges) ? res.view_password_text_ranges : [],
    content_locked: Boolean(res.content_locked),
    text_locked: Boolean(res.text_locked),
    media_locked: Boolean(res.media_locked),
    reactions: it.reactions,
    reply_count: it.reply_count,
    like_count: it.like_count,
    repost_count: it.repost_count,
    liked_by_me: it.liked_by_me,
    reposted_by_me: it.reposted_by_me,
    poll: it.poll,
    reply_to_post_id: it.reply_to_post_id,
    feed_entry_id: it.feed_entry_id,
    is_federated: it.is_federated,
    federated_boost: it.federated_boost,
    remote_object_url: it.remote_object_url,
    remote_actor_url: it.remote_actor_url,
    repost: it.repost,
    visible_at: it.visible_at,
    created_at: it.created_at,
  } as Parameters<typeof mapFeedItem>[0]);
  return {
    caption: mapped.caption,
    media_type: mapped.media_type,
    media_urls: mapped.media_urls,
    is_nsfw: mapped.is_nsfw,
    has_view_password: mapped.has_view_password,
    view_password_scope: mapped.view_password_scope,
    view_password_text_ranges: mapped.view_password_text_ranges,
    content_locked: mapped.content_locked,
    text_locked: mapped.text_locked,
    media_locked: mapped.media_locked,
  };
}

export async function toggleTimelineRepost(
  token: string,
  it: TimelinePost,
  comment?: string | null,
): Promise<{ reposted: boolean; repost_count: number }> {
  const path = federationActionPath(it, "repost");
  const trimmed = typeof comment === "string" ? comment.trim() : "";
  const json = trimmed.length > 0 ? { comment: trimmed } : {};
  return api<{ reposted: boolean; repost_count: number }>(path, {
    method: "POST",
    token,
    json,
  });
}
