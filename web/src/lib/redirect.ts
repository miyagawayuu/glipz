import { apiBase } from "./api";

export function safeRelativeRoute(raw: unknown, fallback = "/feed"): string {
  if (typeof raw !== "string") return fallback;
  const trimmed = raw.trim();
  if (!trimmed || !trimmed.startsWith("/") || trimmed.startsWith("//") || trimmed.includes("\\") || trimmed.includes(":")) {
    return fallback;
  }
  try {
    if (typeof window !== "undefined") {
      const target = new URL(trimmed, window.location.origin);
      if (target.origin !== window.location.origin) return fallback;
    }
    return trimmed;
  } catch {
    return fallback;
  }
}

export function safeHttpURL(raw: unknown): string {
  if (typeof raw !== "string") return "";
  try {
    const base = typeof window !== "undefined" ? window.location.origin : "http://localhost";
    const target = new URL(raw.trim(), base);
    if (target.protocol !== "https:" && target.protocol !== "http:") return "";
    return target.href;
  } catch {
    return "";
  }
}

export function safeMediaURL(raw: unknown): string {
  const href = safeHttpURL(raw);
  if (!href) return "";
  if (typeof window === "undefined") return "";
  try {
    const target = new URL(href);
    const allowedBaseURLs = [
      `${window.location.origin}/api/v1/media/`,
      apiBase() ? `${new URL(apiBase(), window.location.origin).origin}/api/v1/media/` : "",
      ...String(import.meta.env.VITE_ALLOWED_MEDIA_BASE_URLS || "")
        .split(",")
        .map((baseURL) => baseURL.trim())
        .filter(Boolean),
    ];
    for (const baseURL of allowedBaseURLs) {
      if (!baseURL) continue;
      const base = new URL(baseURL, window.location.origin);
      const basePath = base.pathname.endsWith("/") ? base.pathname : `${base.pathname}/`;
      if (basePath === "/") continue;
      if (target.origin === base.origin && target.pathname.startsWith(basePath)) return target.href;
    }
  } catch {
    return "";
  }
  return "";
}

export function redirectToAllowedExternalURL(raw: string, allowedOrigins: string[]): boolean {
  try {
    const target = new URL(raw);
    if (target.protocol !== "https:") return false;
    const allowed = new Set(
      allowedOrigins
        .map((origin) => {
          try {
            return new URL(origin).origin;
          } catch {
            return "";
          }
        })
        .filter(Boolean),
    );
    if (!allowed.has(target.origin)) return false;
    window.location.href = target.href;
    return true;
  } catch {
    return false;
  }
}
