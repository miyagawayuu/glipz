import { getAccessToken } from "../auth";
import { api, apiBase } from "./api";

export type PatreonStatusResponse = {
  patreon: { available: boolean; connected?: boolean };
};

export type PatreonCampaignRow = {
  id: string;
  title: string;
  tiers: { id: string; name: string }[];
};

/**
 * Start Patreon OAuth with the current access token. A normal anchor navigation cannot send
 * `Authorization: Bearer`, so the client fetches JSON with the token and then navigates to Patreon.
 */
export async function startPatreonOAuth(returnToPath: string): Promise<void> {
  const b = (apiBase() || "").replace(/\/+$/, "");
  const path = (returnToPath || "/settings").trim() || "/settings";
  const q = new URLSearchParams({ return_to: path });
  const url = `${b}/api/v1/fanclub/patreon/authorize?${q.toString()}`;
  const token = getAccessToken();
  if (!token) {
    throw new Error("unauthorized");
  }
  const res = await fetch(url, {
    method: "GET",
    headers: {
      Authorization: `Bearer ${token}`,
      Accept: "application/json",
    },
  });
  const data = (await res.json().catch(() => ({}))) as { error?: string; redirect?: string };
  if (res.status === 401) {
    throw new Error("unauthorized");
  }
  if (!res.ok) {
    throw new Error(data.error || "patreon_authorize_failed");
  }
  const loc = typeof data.redirect === "string" ? data.redirect.trim() : "";
  if (loc) {
    window.location.href = loc;
    return;
  }
  throw new Error("patreon_authorize_no_redirect");
}

export async function fetchPatreonStatus(token: string): Promise<PatreonStatusResponse> {
  return api<PatreonStatusResponse>("/api/v1/fanclub/patreon/status", { method: "GET", token });
}

export async function disconnectPatreon(token: string): Promise<void> {
  await api("/api/v1/fanclub/patreon/connection", { method: "DELETE", token });
}

export async function fetchPatreonCampaigns(token: string): Promise<PatreonCampaignRow[]> {
  const res = await api<{ campaigns: PatreonCampaignRow[] }>("/api/v1/fanclub/patreon/campaigns", {
    method: "GET",
    token,
  });
  return Array.isArray(res.campaigns) ? res.campaigns : [];
}

export async function requestPatreonEntitlement(token: string, postId: string): Promise<string> {
  const res = await api<{ entitlement_jwt: string }>("/api/v1/fanclub/patreon/entitlement", {
    method: "POST",
    token,
    json: { post_id: postId },
  });
  return res.entitlement_jwt;
}
