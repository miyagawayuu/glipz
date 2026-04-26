/** Empty means same-origin, for example :5173, and requests reach the backend through Vite's /api proxy. */
import { translate } from "../i18n";
import { COOKIE_AUTH_TOKEN } from "../auth";
import { isNativeApp } from "./runtime";

function normalizeApiOrigin(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) return "";
  const withScheme = /^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(trimmed) ? trimmed : `https://${trimmed}`;
  const u = new URL(withScheme);
  if (u.protocol !== "https:" || !u.hostname || u.username || u.password) return "";
  return u.origin.replace(/\/+$/, "");
}

export function readStoredApiBase(): string {
  return "";
}

export function writeStoredApiBase(next: string): string {
  void next;
  return "";
}

export function clearStoredApiBase(): void {
  /* API origin is fixed at build time. */
}

/** Instance domain for display purposes, avoiding Capacitor's localhost origin. */
export function displayInstanceDomain(): string {
  if (typeof window === "undefined") return "";

  const base = apiBase();
  if (base) {
    try {
      return new URL(base).host;
    } catch {
      /* ignore */
    }
  }

  const host = window.location.host || "";
  const hostname = window.location.hostname || "";
  if (!host) return "";
  if (hostname === "localhost" || hostname === "127.0.0.1" || hostname === "::1") return "";
  return host;
}

/** Suppress early warnings about the API base URL. */
function warnIfCrossOriginApiBase(base: string): void {
  void base;
}

export function apiBase(): string {
  if (isNativeApp()) {
    const nativeURL = import.meta.env.VITE_NATIVE_API_URL;
    if (typeof nativeURL === "string" && nativeURL.trim() !== "") {
      const b = normalizeApiOrigin(nativeURL);
      if (b) {
        warnIfCrossOriginApiBase(b);
        return b;
      }
    }
  }
  const v = import.meta.env.VITE_API_URL;
  if (typeof v === "string" && v.trim() !== "") {
    const b = v.replace(/\/+$/, "");
    warnIfCrossOriginApiBase(b);
    return b;
  }
  return "";
}

export type ApiError = { error: string };

const CSRF_COOKIE = "glipz_csrf";

function csrfToken(): string {
  if (typeof document === "undefined") return "";
  const prefix = `${CSRF_COOKIE}=`;
  for (const part of document.cookie.split(";")) {
    const trimmed = part.trim();
    if (trimmed.startsWith(prefix)) return decodeURIComponent(trimmed.slice(prefix.length));
  }
  return "";
}

function isMutatingMethod(method: string | undefined): boolean {
  const m = (method || "GET").toUpperCase();
  return m !== "GET" && m !== "HEAD" && m !== "OPTIONS" && m !== "TRACE";
}

export function applyBrowserAuth(init: RequestInit = {}): RequestInit {
  const headers = new Headers(init.headers);
  if (isMutatingMethod(init.method)) {
    const csrf = csrfToken();
    if (csrf) headers.set("X-CSRF-Token", csrf);
  }
  return { ...init, credentials: "include", headers };
}

export async function api<T>(
  path: string,
  init?: RequestInit & { token?: string; json?: unknown },
): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.json !== undefined) {
    headers.set("Content-Type", "application/json");
  }
  if (init?.token) {
    if (init.token !== COOKIE_AUTH_TOKEN) headers.set("Authorization", `Bearer ${init.token}`);
  }
  const requestInit = applyBrowserAuth({
    ...init,
    headers,
    body: init?.json !== undefined ? JSON.stringify(init.json) : init?.body,
  });
  const res = await fetch(`${apiBase()}${path}`, requestInit);
  const data = (await res.json().catch(() => ({}))) as T & ApiError;
  if (!res.ok) {
    const err = new Error((data as ApiError).error || res.statusText);
    throw err;
  }
  return data as T;
}

/** Unauthenticated GET helper for public resources such as federated profiles. */
export async function apiPublicGet<T>(path: string): Promise<T> {
  const res = await fetch(`${apiBase()}${path}`, {
    method: "GET",
    credentials: "include",
    headers: { Accept: "application/json" },
  });
  const data = (await res.json().catch(() => ({}))) as T & ApiError;
  if (!res.ok) {
    throw new Error((data as ApiError).error || res.statusText);
  }
  return data as T;
}

export async function presignedPut(
  url: string,
  file: File,
  contentType: string,
): Promise<void> {
  const res = await fetch(url, {
    method: "PUT",
    headers: { "Content-Type": contentType },
    body: file,
  });
  if (!res.ok) {
    throw new Error(translate("errors.uploadFailed"));
  }
}

/** Stores media through the backend when direct presigned PUT uploads are not usable. */
export type MediaUploadResponse = { object_key: string; public_url: string };

export async function uploadMediaFile(token: string, file: File): Promise<MediaUploadResponse> {
  const fd = new FormData();
  fd.append("file", file, file.name);
  const res = await fetch(`${apiBase()}/api/v1/media/upload`, {
    method: "POST",
    credentials: "include",
    headers: applyBrowserAuth({ method: "POST" }).headers,
    body: fd,
  });
  const data = (await res.json().catch(() => ({}))) as MediaUploadResponse & ApiError;
  if (!res.ok) {
    throw new Error((data as ApiError).error || res.statusText);
  }
  return data as MediaUploadResponse;
}
