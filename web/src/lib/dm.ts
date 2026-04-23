import { getAccessToken } from "../auth";
import { api, apiBase } from "./api";
import {
  samePublicJwk,
  type DMEncryptedPrivateKeyBackup,
  type DMLocalIdentity,
  type DMSealedBox,
} from "./dmCrypto";

export type DMIdentityResponse = {
  configured: boolean;
  algorithm?: string;
  public_jwk?: JsonWebKey;
  encrypted_private_jwk?: DMEncryptedPrivateKeyBackup;
};

export type DMThread = {
  id: string;
  peer_id: string;
  peer_handle: string;
  peer_display_name: string;
  peer_badges?: string[];
  peer_avatar_url: string | null;
  peer_algorithm: string;
  peer_public_jwk: JsonWebKey | null;
  skyway_room_name: string;
  unread_count: number;
  can_call?: boolean;
  last_message_at?: string | null;
  created_at: string;
  updated_at: string;
};

export type DMAttachment = {
  object_key: string;
  public_url: string;
  file_name: string;
  content_type: string;
  size_bytes: number;
  encrypted_bytes: number;
  file_iv: string;
  sender_key_box: DMSealedBox;
  recipient_key_box: DMSealedBox;
};

export type DMMessage = {
  id: string;
  sent_by_me: boolean;
  sender_handle: string;
  sender_display_name: string;
  sender_badges?: string[];
  sender_avatar_url: string | null;
  ciphertext: DMSealedBox;
  attachments: DMAttachment[];
  created_at: string;
};

export type DMCallToken = {
  token: string;
  member_name: string;
  room_name: string;
};

export type DMCallEvent = {
  id: string;
  event_type: "invite" | "cancel" | "end" | "missed";
  call_mode: "audio" | "video";
  sent_by_me: boolean;
  actor_handle: string;
  actor_display_name: string;
  actor_badges?: string[];
  actor_avatar_url: string | null;
  created_at: string;
};

export async function inviteDMCall(threadId: string, mode: "audio" | "video"): Promise<void> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  await api(`/api/v1/dm/threads/${encodeURIComponent(threadId)}/call-invite`, {
    method: "POST",
    token,
    json: { mode },
  });
}

async function postDMCallEvent(threadId: string, path: string, mode: "audio" | "video"): Promise<void> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  await api(`/api/v1/dm/threads/${encodeURIComponent(threadId)}/${path}`, {
    method: "POST",
    token,
    json: { mode },
  });
}

export async function cancelDMCall(threadId: string, mode: "audio" | "video"): Promise<void> {
  return postDMCallEvent(threadId, "call-cancel", mode);
}

export async function endDMCall(threadId: string, mode: "audio" | "video"): Promise<void> {
  return postDMCallEvent(threadId, "call-end", mode);
}

export async function markDMCallMissed(threadId: string, mode: "audio" | "video"): Promise<void> {
  return postDMCallEvent(threadId, "call-missed", mode);
}

export async function listDMCallHistory(threadId: string): Promise<DMCallEvent[]> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  const res = await api<{ items: DMCallEvent[] }>(`/api/v1/dm/threads/${encodeURIComponent(threadId)}/call-history`, {
    method: "GET",
    token,
  });
  return res.items ?? [];
}

export type DMUploadResponse = {
  object_key: string;
  public_url: string;
  content_type: string;
  size_bytes: number;
  file_name: string;
};

export async function fetchDMIdentity(): Promise<DMIdentityResponse> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  return api<DMIdentityResponse>("/api/v1/dm/identity", { method: "GET", token });
}

export async function saveDMIdentity(
  identity: DMLocalIdentity,
  encryptedPrivateJwk: DMEncryptedPrivateKeyBackup,
): Promise<void> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  await api("/api/v1/dm/identity", {
    method: "PUT",
    token,
    json: {
      algorithm: identity.algorithm,
      public_jwk: identity.publicJwk,
      encrypted_private_jwk: encryptedPrivateJwk,
    },
  });
}

export function assertDMIdentityMatchesServer(identity: DMLocalIdentity, remote: DMIdentityResponse) {
  if (!remote.public_jwk || !samePublicJwk(identity.publicJwk, remote.public_jwk)) {
    throw new Error("identity_mismatch");
  }
}

export async function listDMThreads(): Promise<DMThread[]> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  const res = await api<{ items: DMThread[] }>("/api/v1/dm/threads", { method: "GET", token });
  return res.items ?? [];
}

export async function createDMThread(peerHandle: string): Promise<DMThread> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  const res = await api<{ thread: DMThread }>("/api/v1/dm/threads", {
    method: "POST",
    token,
    json: { peer_handle: peerHandle },
  });
  return res.thread;
}

/** Prompts the peer to join DM via notification when they have not set up DM keys yet. */
export async function inviteDMPeer(peerHandle: string): Promise<"invited" | "invited_auto" | "peer_ready"> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  const res = await api<{ status: string }>("/api/v1/dm/invite-peer", {
    method: "POST",
    token,
    json: { peer_handle: peerHandle },
  });
  if (res.status === "peer_ready") return "peer_ready";
  if (res.status === "invited_auto") return "invited_auto";
  return "invited";
}

export async function fetchDMThread(threadId: string): Promise<DMThread> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  const res = await api<{ thread: DMThread }>(`/api/v1/dm/threads/${encodeURIComponent(threadId)}`, {
    method: "GET",
    token,
  });
  return res.thread;
}

export async function listDMMessages(threadId: string, before?: string): Promise<DMMessage[]> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  const suffix = before ? `?before=${encodeURIComponent(before)}` : "";
  const res = await api<{ items: DMMessage[] }>(`/api/v1/dm/threads/${encodeURIComponent(threadId)}/messages${suffix}`, {
    method: "GET",
    token,
  });
  return res.items ?? [];
}

export async function sendDMMessage(
  threadId: string,
  input: { sender_payload: DMSealedBox; recipient_payload: DMSealedBox; attachments: DMAttachment[] },
): Promise<DMMessage> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  const res = await api<{ message: DMMessage }>(`/api/v1/dm/threads/${encodeURIComponent(threadId)}/messages`, {
    method: "POST",
    token,
    json: input,
  });
  return res.message;
}

export async function markDMThreadRead(threadId: string): Promise<void> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  await api(`/api/v1/dm/threads/${encodeURIComponent(threadId)}/read`, {
    method: "POST",
    token,
  });
}

export async function fetchDMUnreadCount(): Promise<number> {
  const token = getAccessToken();
  if (!token) return 0;
  const res = await api<{ count: number }>("/api/v1/dm/unread-count", { method: "GET", token });
  return Number(res.count) || 0;
}

export async function issueDMCallToken(threadId: string): Promise<DMCallToken> {
  const token = getAccessToken();
  if (!token) throw new Error("unauthorized");
  return api<DMCallToken>(`/api/v1/dm/threads/${encodeURIComponent(threadId)}/call-token`, {
    method: "POST",
    token,
  });
}

export async function uploadDMFile(token: string, file: File): Promise<DMUploadResponse> {
  const fd = new FormData();
  fd.append("file", file, file.name);
  const res = await fetch(`${apiBase()}/api/v1/dm/upload`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: fd,
  });
  const data = (await res.json().catch(() => ({}))) as DMUploadResponse & { error?: string };
  if (!res.ok) {
    throw new Error(data.error || res.statusText);
  }
  return data as DMUploadResponse;
}
