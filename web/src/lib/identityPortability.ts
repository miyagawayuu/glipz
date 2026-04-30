import { api } from "./api";

export type IdentityBundle = {
  v: number;
  portable_id: string;
  account_public_key: string;
  account_private_key_encrypted?: string;
  private_key?: {
    kdf: string;
    salt: string;
    nonce: string;
    ciphertext: string;
  };
  handle: string;
  display_name: string;
  bio: string;
  also_known_as?: string[];
  created_for_origin?: string;
  exported_at: string;
};

export type IdentityTransferSession = {
  id: string;
  portable_id: string;
  allowed_target_origin: string;
  include_private: boolean;
  include_gated: boolean;
  expires_at: string;
  used_at?: string;
  revoked_at?: string;
  attempt_count: number;
  created_at: string;
};

export type IdentityTransferImportJob = {
  id: string;
  source_origin: string;
  target_origin: string;
  source_session_id: string;
  status: "pending" | "running" | "completed" | "failed" | "cancelled";
  total_posts: number;
  imported_posts: number;
  failed_posts: number;
  total_items: number;
  imported_items: number;
  stats?: {
    profile?: IdentityTransferCategoryStats;
    posts?: IdentityTransferCategoryStats;
    following?: IdentityTransferCategoryStats;
    followers?: IdentityTransferCategoryStats;
    bookmarks?: IdentityTransferCategoryStats;
  };
  next_cursor: string;
  attempt_count: number;
  next_attempt_at: string;
  last_error: string;
  include_private: boolean;
  include_gated: boolean;
  created_at: string;
  updated_at: string;
};

export type IdentityTransferCategoryStats = {
  total: number;
  imported: number;
  skipped: number;
  failed: number;
};

export type IdentityMigrationFile = {
  v: 1;
  kind: "glipz_identity_migration";
  source_origin: string;
  target_origin: string;
  created_at: string;
  expires_at: string;
  session_id: string;
  token: string;
  include_private: boolean;
  include_gated: boolean;
  bundle: IdentityBundle;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasTextField(record: Record<string, unknown>, key: string): boolean {
  return typeof record[key] === "string" && record[key].trim().length > 0;
}

export function isValidSecureIdentityBundle(value: unknown): value is IdentityBundle {
  if (!isRecord(value)) return false;
  if (value.v !== 2) return false;
  if (!hasTextField(value, "portable_id")) return false;
  if (!hasTextField(value, "account_public_key")) return false;
  if (!hasTextField(value, "handle")) return false;
  if (!hasTextField(value, "exported_at")) return false;
  if (!isRecord(value.private_key)) return false;
  return hasTextField(value.private_key, "kdf")
    && hasTextField(value.private_key, "salt")
    && hasTextField(value.private_key, "nonce")
    && hasTextField(value.private_key, "ciphertext");
}

export function isValidIdentityMigrationFile(value: unknown): value is IdentityMigrationFile {
  if (!isRecord(value)) return false;
  if (value.v !== 1 || value.kind !== "glipz_identity_migration") return false;
  if (!hasTextField(value, "source_origin")) return false;
  if (!hasTextField(value, "target_origin")) return false;
  if (!hasTextField(value, "created_at")) return false;
  if (!hasTextField(value, "expires_at")) return false;
  if (!hasTextField(value, "session_id")) return false;
  if (!hasTextField(value, "token")) return false;
  if (typeof value.include_private !== "boolean" || typeof value.include_gated !== "boolean") return false;
  return isValidSecureIdentityBundle(value.bundle);
}

export async function exportSecureIdentityBundle(passphrase: string, targetOrigin: string): Promise<IdentityBundle> {
  return api<IdentityBundle>("/api/v1/me/identity/export-secure", {
    method: "POST",
    json: { passphrase, target_origin: targetOrigin },
  });
}

export async function importSecureIdentityBundle(bundle: IdentityBundle, passphrase: string): Promise<void> {
  await api<{ ok: true }>("/api/v1/me/identity/import-secure", {
    method: "PUT",
    json: { bundle, passphrase },
  });
}

export async function createIdentityTransferSession(input: {
  target_origin: string;
  include_private: boolean;
  include_gated: boolean;
  expires_in_hours?: number;
}): Promise<{ session: IdentityTransferSession; token: string }> {
  return api<{ session: IdentityTransferSession; token: string }>("/api/v1/me/identity/transfer-sessions", {
    method: "POST",
    json: input,
  });
}

export async function createIdentityImportJob(input: {
  source_origin: string;
  target_origin?: string;
  source_session_id: string;
  token: string;
  include_private: boolean;
  include_gated: boolean;
}): Promise<IdentityTransferImportJob> {
  return api<IdentityTransferImportJob>("/api/v1/me/identity/import-jobs", {
    method: "POST",
    json: input,
  });
}

export async function getIdentityImportJob(jobID: string): Promise<IdentityTransferImportJob> {
  return api<IdentityTransferImportJob>(`/api/v1/me/identity/import-jobs/${encodeURIComponent(jobID)}`);
}

export async function retryIdentityImportJob(jobID: string): Promise<void> {
  await api<{ ok: true }>(`/api/v1/me/identity/import-jobs/${encodeURIComponent(jobID)}/retry`, { method: "POST" });
}

export async function cancelIdentityImportJob(jobID: string): Promise<void> {
  await api<{ ok: true }>(`/api/v1/me/identity/import-jobs/${encodeURIComponent(jobID)}`, { method: "DELETE" });
}

export async function declareIdentityMove(movedToAcct: string): Promise<void> {
  await api<{ ok: true }>("/api/v1/me/identity/move", {
    method: "POST",
    json: { moved_to_acct: movedToAcct },
  });
}
