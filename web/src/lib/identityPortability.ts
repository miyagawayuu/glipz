import { api } from "./api";

export type IdentityBundle = {
  v: number;
  portable_id: string;
  account_public_key: string;
  account_private_key_encrypted: string;
  handle: string;
  display_name: string;
  bio: string;
  also_known_as?: string[];
  exported_at: string;
};

export async function exportIdentityBundle(): Promise<IdentityBundle> {
  return api<IdentityBundle>("/api/v1/me/identity/export");
}

export async function importIdentityBundle(bundle: IdentityBundle): Promise<void> {
  await api<{ ok: true }>("/api/v1/me/identity/import", {
    method: "PUT",
    json: bundle,
  });
}

export async function declareIdentityMove(movedToAcct: string): Promise<void> {
  await api<{ ok: true }>("/api/v1/me/identity/move", {
    method: "POST",
    json: { moved_to_acct: movedToAcct },
  });
}
