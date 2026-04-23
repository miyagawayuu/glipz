import { api } from "./api";

export type RepostToggleResponse = { reposted: boolean; repost_count: number };

/** Creates or removes a repost without post-body edits. Omit comment when removing a repost. */
export async function toggleRepost(postId: string, token: string, comment?: string | null): Promise<RepostToggleResponse> {
  const trimmed = typeof comment === "string" ? comment.trim() : "";
  const json = trimmed.length > 0 ? { comment: trimmed } : {};
  return api<RepostToggleResponse>(`/api/v1/posts/${encodeURIComponent(postId)}/repost`, {
    method: "POST",
    token,
    json,
  });
}
