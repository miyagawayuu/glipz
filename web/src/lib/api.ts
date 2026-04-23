/** Empty means same-origin, for example :5173, and requests reach the backend through Vite's /api proxy. */
import { translate } from "../i18n";
import { isNativeApp } from "./runtime";

const API_BASE_STORAGE_KEY = "glipz_api_base";

function normalizeApiDomainInput(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) return "";

  // Accept inputs like:
  // - example.com
  // - example.com:8443
  // - https://example.com (we'll strip scheme)
  // - example.com/path (we'll drop path/query/hash)
  const withScheme = /^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(trimmed) ? trimmed : `https://${trimmed}`;
  const u = new URL(withScheme);
  if (!u.hostname) return "";

  const host = u.port ? `${u.hostname}:${u.port}` : u.hostname;
  return host;
}

export function readStoredApiBase(): string {
  if (typeof window === "undefined") return "";
  if (!isNativeApp()) return "";
  try {
    const v = window.localStorage.getItem(API_BASE_STORAGE_KEY);
    if (typeof v !== "string") return "";
    return normalizeApiDomainInput(v);
  } catch {
    return "";
  }
}

export function writeStoredApiBase(next: string): string {
  const normalized = normalizeApiDomainInput(next);
  if (typeof window === "undefined") return normalized;
  if (!isNativeApp()) return normalized;
  try {
    if (!normalized) {
      window.localStorage.removeItem(API_BASE_STORAGE_KEY);
      return "";
    }
    window.localStorage.setItem(API_BASE_STORAGE_KEY, normalized);
    return normalized;
  } catch {
    return normalized;
  }
}

export function clearStoredApiBase(): void {
  if (typeof window === "undefined") return;
  if (!isNativeApp()) return;
  try {
    window.localStorage.removeItem(API_BASE_STORAGE_KEY);
  } catch {
    /* ignore */
  }
}

/** Instance domain for display purposes, avoiding Capacitor's localhost origin. */
export function displayInstanceDomain(): string {
  if (typeof window === "undefined") return "";

  const apiDomain = readStoredApiBase();
  if (apiDomain) return apiDomain;

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
  const stored = readStoredApiBase();
  if (stored) {
    const b = `https://${stored}`;
    warnIfCrossOriginApiBase(b);
    return b;
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

export async function api<T>(
  path: string,
  init?: RequestInit & { token?: string; json?: unknown },
): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.json !== undefined) {
    headers.set("Content-Type", "application/json");
  }
  if (init?.token) {
    headers.set("Authorization", `Bearer ${init.token}`);
  }
  const res = await fetch(`${apiBase()}${path}`, {
    ...init,
    headers,
    body: init?.json !== undefined ? JSON.stringify(init.json) : init?.body,
  });
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
    headers: { Authorization: `Bearer ${token}` },
    body: fd,
  });
  const data = (await res.json().catch(() => ({}))) as MediaUploadResponse & ApiError;
  if (!res.ok) {
    throw new Error((data as ApiError).error || res.statusText);
  }
  return data as MediaUploadResponse;
}
