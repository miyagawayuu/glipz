import { api } from "./api";

export type FederatedNotePayload = {
  id: string;
  object_iri: string;
  actor_iri: string;
  actor_acct: string;
  actor_name: string;
  actor_icon_url?: string;
  actor_profile_url?: string;
  title: string;
  body_md: string;
  body_premium_md: string;
  premium_locked: boolean;
  visibility: string;
  published_at: string;
  updated_at: string;
  has_premium: boolean;
  paywall_provider: string;
  patreon_campaign_id: string;
  patreon_required_reward_tier_id: string;
  unlock_url: string;
};

export async function unlockFederatedNote(token: string, incomingId: string, password: string): Promise<FederatedNotePayload> {
  const res = await api<{ item: FederatedNotePayload }>(`/api/v1/federation/notes/${encodeURIComponent(incomingId)}/unlock`, {
    method: "POST",
    token,
    json: { password },
  });
  return res.item;
}

