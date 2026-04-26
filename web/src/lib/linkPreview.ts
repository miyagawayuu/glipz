import { apiPublicGet } from "./api";
import { safeHttpURL } from "./redirect";

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
    .then((preview) => normalizeLinkPreview(preview))
    .catch(() => null);
  previewCache.set(normalized, promise);
  return promise;
}

export function normalizeLinkPreview(preview: LinkPreview): LinkPreview | null {
  const url = safeHttpURL(preview?.url);
  if (!url) return null;
  return {
    ...preview,
    url,
    image_url: preview.image_url ? safeHttpURL(preview.image_url) || undefined : undefined,
  };
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
