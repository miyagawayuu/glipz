import type { MeResp } from "../composables/useSecuritySettings";
import type { FanclubLinkStatus, FanclubProviderID } from "./registry";

type FanclubsMap = Record<string, FanclubLinkStatus>;

function isFanclubLinkStatus(v: unknown): v is FanclubLinkStatus {
  if (!v || typeof v !== "object") return false;
  const x = v as Record<string, unknown>;
  return typeof x.member_linked === "boolean" && typeof x.creator_linked === "boolean";
}

function asFanclubsMap(v: unknown): FanclubsMap | null {
  if (!v || typeof v !== "object") return null;
  const out: FanclubsMap = {};
  for (const [k, val] of Object.entries(v as Record<string, unknown>)) {
    if (isFanclubLinkStatus(val)) out[k] = val;
  }
  return out;
}

/**
 * Normalize fanclub link status from `/me`.
 *
 * Future: backend may return `me.fanclubs[providerId]`.
 * Future: backend may return `me.fanclubs[providerId]`.
 */
export function getFanclubLinkStatus(me: MeResp | null, providerId: FanclubProviderID): FanclubLinkStatus | null {
  if (!me) return null;

  // Future-compatible shape (if/when backend adds it).
  const maybeFanclubs = asFanclubsMap((me as unknown as { fanclubs?: unknown }).fanclubs);
  const fromFanclubs = maybeFanclubs?.[providerId];
  if (fromFanclubs) return fromFanclubs;

  return { member_linked: false, creator_linked: false };
}

