import { APP_NAME } from "./appInfo";

const DEFAULT_PUBLIC_ORIGIN = "https://glipz.io";
const DEFAULT_IMAGE_PATH = "/icon.svg";

type SeoMetaOptions = {
  title?: string;
  description: string;
  canonicalPath?: string;
  noindex?: boolean;
};

function getPublicOrigin(): string {
  if (typeof window === "undefined") return DEFAULT_PUBLIC_ORIGIN;
  return window.location.origin;
}

function toAbsoluteUrl(pathOrUrl: string): string {
  try {
    return new URL(pathOrUrl, getPublicOrigin()).toString();
  } catch {
    return new URL("/", DEFAULT_PUBLIC_ORIGIN).toString();
  }
}

function getCurrentCanonicalUrl(canonicalPath?: string): string {
  if (canonicalPath) return toAbsoluteUrl(canonicalPath);
  if (typeof window === "undefined") return toAbsoluteUrl("/");
  return toAbsoluteUrl(window.location.pathname);
}

function upsertMeta(selector: string, attributes: Record<string, string>) {
  if (typeof document === "undefined") return;
  let element = document.head.querySelector<HTMLMetaElement>(selector);
  if (!element) {
    element = document.createElement("meta");
    document.head.appendChild(element);
  }
  for (const [name, value] of Object.entries(attributes)) {
    element.setAttribute(name, value);
  }
}

function upsertCanonical(href: string) {
  if (typeof document === "undefined") return;
  let element = document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]');
  if (!element) {
    element = document.createElement("link");
    element.rel = "canonical";
    document.head.appendChild(element);
  }
  element.href = href;
}

export function applySeoMeta({ title, description, canonicalPath, noindex }: SeoMetaOptions) {
  const resolvedTitle = title?.trim() || APP_NAME;
  const canonicalUrl = getCurrentCanonicalUrl(canonicalPath);
  const imageUrl = toAbsoluteUrl(DEFAULT_IMAGE_PATH);
  const robots = noindex ? "noindex,nofollow" : "index,follow";

  upsertMeta('meta[name="description"]', { name: "description", content: description });
  upsertMeta('meta[name="robots"]', { name: "robots", content: robots });
  upsertCanonical(canonicalUrl);

  upsertMeta('meta[property="og:site_name"]', { property: "og:site_name", content: APP_NAME });
  upsertMeta('meta[property="og:type"]', { property: "og:type", content: "website" });
  upsertMeta('meta[property="og:title"]', { property: "og:title", content: resolvedTitle });
  upsertMeta('meta[property="og:description"]', { property: "og:description", content: description });
  upsertMeta('meta[property="og:url"]', { property: "og:url", content: canonicalUrl });
  upsertMeta('meta[property="og:image"]', { property: "og:image", content: imageUrl });

  upsertMeta('meta[name="twitter:card"]', { name: "twitter:card", content: "summary" });
  upsertMeta('meta[name="twitter:title"]', { name: "twitter:title", content: resolvedTitle });
  upsertMeta('meta[name="twitter:description"]', { name: "twitter:description", content: description });
  upsertMeta('meta[name="twitter:image"]', { name: "twitter:image", content: imageUrl });
}
