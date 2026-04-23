import { apiPublicGet } from "./api";

export type LinkPreview = {
  url: string;
  title: string;
  description?: string;
  image_url?: string;
  site_name?: string;
};

const previewCache = new Map<string, Promise<LinkPreview | null>>();

export function fetchLinkPreview(url: string): Promise<LinkPreview | null> {
  const normalized = normalizePreviewUrl(url);
  if (!normalized) return Promise.resolve(null);
  const cached = previewCache.get(normalized);
  if (cached) return cached;
  const promise = apiPublicGet<LinkPreview>(`/api/v1/link-preview?url=${encodeURIComponent(normalized)}`)
    .then((preview) => preview)
    .catch(() => null);
  previewCache.set(normalized, promise);
  return promise;
}

function normalizePreviewUrl(raw: string): string | null {
  try {
    const url = new URL(raw);
    if (url.protocol !== "http:" && url.protocol !== "https:") {
      return null;
    }
    return url.toString();
  } catch {
    return null;
  }
}
