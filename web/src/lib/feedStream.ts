import { api, apiBase, apiPublicGet } from "./api";
import type { TimelinePoll, TimelinePost, TimelineReaction } from "../types/timeline";

export type FeedPubPayload =
  | { v: number; kind: "post_created"; post_id: string; author_id: string }
  | { v: number; kind: "post_updated"; post_id: string; author_id?: string }
  | { v: number; kind: "post_deleted"; post_id: string; author_id?: string }
  | { v: number; kind: "federated_post_upsert"; incoming_id: string }
  | { v: number; kind: "federated_post_deleted"; incoming_id: string };

/** Consumes complete SSE blocks and returns the unfinished remainder still buffered. */
function consumeSseBuffer(buf: string, onDataLine: (line: string) => void): string {
  for (;;) {
    const sep = buf.indexOf("\n\n");
    if (sep < 0) return buf;
    const block = buf.slice(0, sep);
    buf = buf.slice(sep + 2);
    for (const line of block.split("\n")) {
      if (line.startsWith("data:")) {
        onDataLine(line.slice(5).trimStart());
      }
    }
  }
}

function mapPoll(p: unknown): TimelinePoll | undefined {
  if (!p || typeof p !== "object") return undefined;
  const o = p as Record<string, unknown>;
  const ends = typeof o.ends_at === "string" ? o.ends_at : "";
  if (!ends) return undefined;
  const optsRaw = Array.isArray(o.options) ? o.options : [];
  const options = optsRaw
    .map((row) => {
      if (!row || typeof row !== "object") return null;
      const r = row as Record<string, unknown>;
      const id = typeof r.id === "string" ? r.id : "";
      const label = typeof r.label === "string" ? r.label : "";
      const votes = typeof r.votes === "number" ? r.votes : Number(r.votes) || 0;
      if (!id) return null;
      return { id, label, votes };
    })
    .filter((x): x is { id: string; label: string; votes: number } => x != null);
  return {
    ends_at: ends,
    closed: Boolean(o.closed),
    options,
    my_option_id: typeof o.my_option_id === "string" && o.my_option_id ? o.my_option_id : undefined,
    total_votes: typeof o.total_votes === "number" ? o.total_votes : Number(o.total_votes) || 0,
  };
}

function mapReactions(input: unknown): TimelineReaction[] {
  if (!Array.isArray(input)) return [];
  return input
    .map((row) => {
      if (!row || typeof row !== "object") return null;
      const r = row as Record<string, unknown>;
      const emoji = typeof r.emoji === "string" ? r.emoji.trim() : "";
      if (!emoji) return null;
      return {
        emoji,
        count: typeof r.count === "number" ? r.count : Number(r.count) || 0,
        reacted_by_me: Boolean(r.reacted_by_me),
      };
    })
    .filter((row): row is TimelineReaction => row != null);
}

function mapFeedItem(x: {
  id: string;
  user_email: string;
  user_handle?: string;
  user_display_name?: string;
  user_badges?: string[];
  user_avatar_url?: string;
  caption: string;
  media_type: string;
  media_urls: string[];
  visibility?: string;
  is_nsfw?: boolean;
  has_view_password?: boolean;
  has_membership_lock?: boolean;
  membership_provider?: string;
  membership_creator_id?: string;
  membership_tier_id?: string;
  has_payment_lock?: boolean;
  payment_provider?: string;
  payment_creator_id?: string;
  payment_plan_id?: string;
  view_password_scope?: number;
  view_password_text_ranges?: { start?: number; end?: number }[];
  content_locked?: boolean;
  text_locked?: boolean;
  media_locked?: boolean;
  created_at?: string;
  visible_at?: string;
  poll?: unknown;
  reactions?: unknown;
  reply_count?: number;
  like_count?: number;
  repost_count?: number;
  liked_by_me?: boolean;
  reposted_by_me?: boolean;
  bookmarked_by_me?: boolean;
  reply_to_post_id?: string;
  reply_to_object_url?: string;
  feed_entry_id?: string;
  repost?: {
    user_id: string;
    user_email: string;
    user_handle: string;
    user_display_name?: string;
    user_badges?: string[];
    user_avatar_url?: string;
    reposted_at: string;
    comment?: string;
  };
  is_federated?: boolean;
  federated_boost?: boolean;
  remote_object_url?: string;
  remote_actor_url?: string;
}): TimelinePost {
  const poll = mapPoll(x.poll);
  const reactions = mapReactions(x.reactions);
  return {
    id: x.id,
    user_email: x.user_email,
    user_handle: x.user_handle ?? "",
    user_display_name: x.user_display_name ?? "",
    user_badges: Array.isArray(x.user_badges) ? x.user_badges.map((badge) => String(badge)) : [],
    user_avatar_url: x.user_avatar_url ?? "",
    caption: x.caption ?? "",
    media_type: x.media_type,
    media_urls: x.media_urls ?? [],
    visibility:
      x.visibility === "logged_in" || x.visibility === "followers" || x.visibility === "private"
        ? x.visibility
        : "public",
    is_nsfw: Boolean(x.is_nsfw),
    has_view_password: Boolean(x.has_view_password),
    has_membership_lock: Boolean(x.has_membership_lock),
    membership_provider: typeof x.membership_provider === "string" ? x.membership_provider : undefined,
    membership_creator_id: typeof x.membership_creator_id === "string" ? x.membership_creator_id : undefined,
    membership_tier_id: typeof x.membership_tier_id === "string" ? x.membership_tier_id : undefined,
    has_payment_lock: Boolean(x.has_payment_lock),
    payment_provider: typeof x.payment_provider === "string" ? x.payment_provider : undefined,
    payment_creator_id: typeof x.payment_creator_id === "string" ? x.payment_creator_id : undefined,
    payment_plan_id: typeof x.payment_plan_id === "string" ? x.payment_plan_id : undefined,
    view_password_scope: typeof x.view_password_scope === "number" ? x.view_password_scope : 0,
    view_password_text_ranges: Array.isArray(x.view_password_text_ranges)
      ? x.view_password_text_ranges
          .map((rg) => {
            const start = typeof rg?.start === "number" ? rg.start : Number(rg?.start);
            const end = typeof rg?.end === "number" ? rg.end : Number(rg?.end);
            if (!Number.isFinite(start) || !Number.isFinite(end)) return null;
            return { start, end };
          })
          .filter((rg): rg is { start: number; end: number } => rg != null)
      : [],
    content_locked: Boolean(x.content_locked),
    text_locked: Boolean(x.text_locked),
    media_locked: Boolean(x.media_locked),
    created_at: x.created_at,
    visible_at: x.visible_at ?? "",
    poll: poll ?? undefined,
    reactions,
    reply_count: x.reply_count ?? 0,
    like_count: x.like_count ?? 0,
    repost_count: x.repost_count ?? 0,
    liked_by_me: Boolean(x.liked_by_me),
    reposted_by_me: Boolean(x.reposted_by_me),
    bookmarked_by_me: Boolean(x.bookmarked_by_me),
    reply_to_post_id: typeof x.reply_to_post_id === "string" && x.reply_to_post_id ? x.reply_to_post_id : undefined,
    reply_to_object_url: typeof x.reply_to_object_url === "string" && x.reply_to_object_url ? x.reply_to_object_url : undefined,
    feed_entry_id:
      typeof x.feed_entry_id === "string" && x.feed_entry_id.trim()
        ? x.feed_entry_id.trim()
        : typeof x.id === "string"
          ? x.id
          : undefined,
    is_federated: Boolean(x.is_federated),
    federated_boost: Boolean(x.federated_boost),
    remote_object_url: typeof x.remote_object_url === "string" ? x.remote_object_url : undefined,
    remote_actor_url: typeof x.remote_actor_url === "string" ? x.remote_actor_url : undefined,
    repost: (() => {
      if (!x.repost || typeof x.repost !== "object") return undefined;
      const r = x.repost as Record<string, unknown>;
      const repostedAt = typeof r.reposted_at === "string" ? r.reposted_at.trim() : "";
      const userHandle = typeof r.user_handle === "string" ? r.user_handle.trim() : "";
      if (!repostedAt || !userHandle) return undefined;
      const commentRaw = typeof r.comment === "string" ? r.comment.trim() : "";
      return {
        user_id: String(r.user_id ?? ""),
        user_email: String(r.user_email ?? ""),
        user_handle: userHandle,
        user_display_name: typeof r.user_display_name === "string" ? (r.user_display_name as string) : undefined,
        user_badges: Array.isArray(r.user_badges) ? r.user_badges.map((badge) => String(badge)) : [],
        user_avatar_url: typeof r.user_avatar_url === "string" ? (r.user_avatar_url as string) : undefined,
        reposted_at: repostedAt,
        ...(commentRaw ? { comment: commentRaw } : {}),
      };
    })(),
  };
}

/**
 * Reads the Redis-backed SSE stream over fetch and forwards `event: feed` JSON payloads.
 * @returns Abort function used to disconnect the stream.
 */
export function connectFeedStream(opts: {
  scope: "all" | "following";
  token?: string | null;
  public?: boolean;
  onPayload: (p: FeedPubPayload) => void;
  onError?: (e: unknown) => void;
}): () => void {
  const ac = new AbortController();
  const q = opts.scope === "following" ? "following" : "all";
  const url = opts.public
    ? `${apiBase()}/api/v1/public/posts/feed/stream`
    : `${apiBase()}/api/v1/posts/feed/stream?scope=${encodeURIComponent(q)}`;

  void (async () => {
    try {
      const res = await fetch(url, {
        method: "GET",
        headers: {
          Accept: "text/event-stream",
          ...(opts.token ? { Authorization: `Bearer ${opts.token}` } : {}),
        },
        signal: ac.signal,
      });
      if (!res.ok) {
        opts.onError?.(new Error(String(res.status)));
        return;
      }
      const body = res.body;
      if (!body) {
        opts.onError?.(new Error("no body"));
        return;
      }
      const reader = body.getReader();
      const dec = new TextDecoder();
      let buf = "";
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        buf = consumeSseBuffer(buf, (line) => {
          if (!line || line.startsWith(":")) return;
          try {
            const p = JSON.parse(line) as FeedPubPayload;
            if (p && typeof p === "object" && "kind" in p) opts.onPayload(p);
          } catch {
            /* ignore */
          }
        });
      }
    } catch (e: unknown) {
      if ((e as Error)?.name === "AbortError") return;
      opts.onError?.(e);
    }
  })();

  return () => ac.abort();
}

export async function fetchPublicFeedItems(): Promise<TimelinePost[]> {
  try {
    const res = await apiPublicGet<{ items: Parameters<typeof mapFeedItem>[0][] }>("/api/v1/public/posts/feed");
    return (res.items ?? []).map((x) => mapFeedItem(x));
  } catch {
    return [];
  }
}

/** Fetches a single post using the feed projection. Omitting token keeps the request anonymous. */
export async function fetchFeedItem(postId: string, token?: string | null): Promise<TimelinePost | null> {
  try {
    const res = await api<{ item: Parameters<typeof mapFeedItem>[0] }>(
      `/api/v1/posts/${encodeURIComponent(postId)}/feed-item`,
      {
        method: "GET",
        ...(token ? { token } : {}),
      },
    );
    return mapFeedItem(res.item);
  } catch {
    return null;
  }
}

/** Fetches one inbound federated post for the public Glipz detail page. */
export async function fetchPublicFederatedIncoming(incomingId: string): Promise<TimelinePost | null> {
  try {
    const res = await apiPublicGet<{ item: Parameters<typeof mapFeedItem>[0] }>(
      `/api/v1/public/federation/incoming/${encodeURIComponent(incomingId)}`,
    );
    return mapFeedItem(res.item);
  } catch {
    return null;
  }
}

export async function fetchFederatedIncomingFeedItem(incomingId: string, token: string): Promise<TimelinePost | null> {
  try {
    const res = await api<{ item: Parameters<typeof mapFeedItem>[0] }>(
      `/api/v1/federation/posts/${encodeURIComponent(incomingId)}/feed-item`,
      {
        method: "GET",
        token,
      },
    );
    return mapFeedItem(res.item);
  } catch {
    return null;
  }
}

/** Fetches flat reply rows belonging to a root-post thread. Omitting token keeps the request anonymous. */
export async function fetchPostThreadReplies(rootPostId: string, token?: string | null): Promise<TimelinePost[]> {
  try {
    const res = await api<{ items: Parameters<typeof mapFeedItem>[0][] }>(
      `/api/v1/posts/${encodeURIComponent(rootPostId)}/thread`,
      { method: "GET", ...(token ? { token } : {}) },
    );
    return (res.items ?? []).map((x) => mapFeedItem(x));
  } catch {
    return [];
  }
}

export async function fetchFederatedThreadReplies(incomingId: string, token?: string | null): Promise<TimelinePost[]> {
  try {
    const path = token
      ? `/api/v1/federation/posts/${encodeURIComponent(incomingId)}/thread`
      : `/api/v1/public/federation/incoming/${encodeURIComponent(incomingId)}/thread`;
    const res = await api<{ items: Parameters<typeof mapFeedItem>[0][] }>(
      path,
      { method: "GET", ...(token ? { token } : {}) },
    );
    return (res.items ?? []).map((x) => mapFeedItem(x));
  } catch {
    return [];
  }
}

export { mapFeedItem };
