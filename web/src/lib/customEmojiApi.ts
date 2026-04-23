import { api, uploadMediaFile } from "./api";
import type { CustomEmoji } from "../types/customEmoji";

type EmojiListResponse = { items?: CustomEmoji[] };
type EmojiItemResponse = { item?: CustomEmoji };

export async function listMyCustomEmojis(token: string): Promise<CustomEmoji[]> {
  const res = await api<EmojiListResponse>("/api/v1/me/custom-emojis", { method: "GET", token });
  return res.items ?? [];
}

export async function createMyCustomEmoji(
  token: string,
  input: { shortcode_name: string; file: File; is_enabled?: boolean },
): Promise<CustomEmoji> {
  const upload = await uploadMediaFile(token, input.file);
  const res = await api<EmojiItemResponse>("/api/v1/me/custom-emojis", {
    method: "POST",
    token,
    json: {
      shortcode_name: input.shortcode_name,
      object_key: upload.object_key,
      is_enabled: input.is_enabled ?? true,
    },
  });
  if (!res.item) throw new Error("invalid_response");
  return res.item;
}

export async function patchMyCustomEmoji(
  token: string,
  emojiId: string,
  input: { shortcode_name: string; is_enabled: boolean; file?: File | null },
): Promise<CustomEmoji> {
  let objectKey: string | undefined;
  if (input.file) {
    const upload = await uploadMediaFile(token, input.file);
    objectKey = upload.object_key;
  }
  const res = await api<EmojiItemResponse>(`/api/v1/me/custom-emojis/${encodeURIComponent(emojiId)}`, {
    method: "PATCH",
    token,
    json: {
      shortcode_name: input.shortcode_name,
      object_key: objectKey,
      is_enabled: input.is_enabled,
    },
  });
  if (!res.item) throw new Error("invalid_response");
  return res.item;
}

export async function deleteMyCustomEmoji(token: string, emojiId: string): Promise<void> {
  await api(`/api/v1/me/custom-emojis/${encodeURIComponent(emojiId)}`, { method: "DELETE", token });
}

export async function listAdminSiteCustomEmojis(token: string): Promise<CustomEmoji[]> {
  const res = await api<EmojiListResponse>("/api/v1/admin/custom-emojis/site", { method: "GET", token });
  return res.items ?? [];
}

export async function createAdminSiteCustomEmoji(
  token: string,
  input: { shortcode_name: string; file: File; is_enabled?: boolean },
): Promise<CustomEmoji> {
  const upload = await uploadMediaFile(token, input.file);
  const res = await api<EmojiItemResponse>("/api/v1/admin/custom-emojis/site", {
    method: "POST",
    token,
    json: {
      shortcode_name: input.shortcode_name,
      object_key: upload.object_key,
      is_enabled: input.is_enabled ?? true,
    },
  });
  if (!res.item) throw new Error("invalid_response");
  return res.item;
}

export async function patchAdminSiteCustomEmoji(
  token: string,
  emojiId: string,
  input: { shortcode_name: string; is_enabled: boolean; file?: File | null },
): Promise<CustomEmoji> {
  let objectKey: string | undefined;
  if (input.file) {
    const upload = await uploadMediaFile(token, input.file);
    objectKey = upload.object_key;
  }
  const res = await api<EmojiItemResponse>(`/api/v1/admin/custom-emojis/site/${encodeURIComponent(emojiId)}`, {
    method: "PATCH",
    token,
    json: {
      shortcode_name: input.shortcode_name,
      object_key: objectKey,
      is_enabled: input.is_enabled,
    },
  });
  if (!res.item) throw new Error("invalid_response");
  return res.item;
}

export async function deleteAdminSiteCustomEmoji(token: string, emojiId: string): Promise<void> {
  await api(`/api/v1/admin/custom-emojis/site/${encodeURIComponent(emojiId)}`, { method: "DELETE", token });
}
