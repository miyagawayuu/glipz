import { api } from "./api";
import { mapFeedItem } from "./feedStream";
import type { TimelinePost } from "../types/timeline";

export type CommunityMemberStatus = "pending" | "approved" | "rejected";
export type CommunityMemberRole = "owner" | "member";

export type Community = {
  id: string;
  name: string;
  description: string;
  details: string;
  member_previews?: CommunityMemberPreview[];
  icon_url?: string | null;
  header_url?: string | null;
  creator_user_id: string;
  created_at: string;
  updated_at: string;
  approved_member_count: number;
  viewer_status?: CommunityMemberStatus;
  viewer_role?: CommunityMemberRole;
  pending_request_count?: number;
  can_manage?: boolean;
};

export type CommunityMemberPreview = {
  user_id: string;
  handle: string;
  display_name: string;
  avatar_url?: string | null;
};

export type CommunityJoinRequest = {
  user_id: string;
  handle: string;
  display_name: string;
  avatar_url?: string | null;
  created_at: string;
};

export type CommunityMediaTile = {
  post_id: string;
  media_type: string;
  preview_url: string;
  locked: boolean;
};

export async function listCommunities(q = ""): Promise<Community[]> {
  const params = new URLSearchParams();
  const query = q.trim();
  if (query) params.set("q", query);
  const path = `/api/v1/communities${params.toString() ? `?${params.toString()}` : ""}`;
  const res = await api<{ items: Community[] }>(path, { method: "GET" });
  return res.items ?? [];
}

export async function createCommunity(input: {
  name: string;
  description: string;
  details?: string;
  icon_object_key?: string;
  header_object_key?: string;
}): Promise<Community> {
  const res = await api<{ community: Community }>("/api/v1/communities", {
    method: "POST",
    json: input,
  });
  return res.community;
}

export async function getCommunity(id: string): Promise<Community> {
  const res = await api<{ community: Community }>(`/api/v1/communities/${encodeURIComponent(id)}`, { method: "GET" });
  return res.community;
}

export async function getCommunityPosts(id: string): Promise<{ community: Community; items: TimelinePost[] }> {
  const res = await api<{ community: Community; items: Record<string, unknown>[] }>(
    `/api/v1/communities/${encodeURIComponent(id)}/posts`,
    { method: "GET" },
  );
  return {
    community: res.community,
    items: (res.items ?? []).map((x) => mapFeedItem(x as Parameters<typeof mapFeedItem>[0])),
  };
}

export async function getCommunityPostMediaTiles(id: string): Promise<CommunityMediaTile[]> {
  const res = await api<{ tiles: CommunityMediaTile[] }>(
    `/api/v1/communities/${encodeURIComponent(id)}/post-media-tiles`,
    { method: "GET" },
  );
  return res.tiles ?? [];
}

export async function requestCommunityJoin(id: string): Promise<Community> {
  const res = await api<{ community: Community }>(`/api/v1/communities/${encodeURIComponent(id)}/join-requests`, {
    method: "POST",
  });
  return res.community;
}

export async function listCommunityJoinRequests(id: string): Promise<CommunityJoinRequest[]> {
  const res = await api<{ items: CommunityJoinRequest[] }>(
    `/api/v1/communities/${encodeURIComponent(id)}/join-requests`,
    { method: "GET" },
  );
  return res.items ?? [];
}

export async function reviewCommunityJoinRequest(id: string, userId: string, approve: boolean): Promise<void> {
  await api(`/api/v1/communities/${encodeURIComponent(id)}/members/${encodeURIComponent(userId)}/${approve ? "approve" : "reject"}`, {
    method: "POST",
  });
}

export async function updateCommunity(
  id: string,
  input: { name: string; description: string; details?: string; icon_object_key?: string; header_object_key?: string },
): Promise<Community> {
  const res = await api<{ community: Community }>(`/api/v1/communities/${encodeURIComponent(id)}`, {
    method: "PATCH",
    json: input,
  });
  return res.community;
}

export async function deleteCommunity(id: string): Promise<void> {
  await api(`/api/v1/communities/${encodeURIComponent(id)}`, { method: "DELETE" });
}
