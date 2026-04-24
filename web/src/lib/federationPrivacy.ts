import { api } from "./api";

export type FederationRelationship = { blocked: boolean; muted: boolean };

export function normalizeFederationAcct(acct: string): string {
  return acct.trim().toLowerCase();
}

export async function getFederationRelationship(token: string, targetAcct: string): Promise<FederationRelationship> {
  return api<FederationRelationship>(
    `/api/v1/me/federation/relationship?target_acct=${encodeURIComponent(normalizeFederationAcct(targetAcct))}`,
    { method: "GET", token },
  );
}

export async function blockFederationUser(token: string, targetAcct: string): Promise<void> {
  await api("/api/v1/me/federation/blocks", {
    method: "POST",
    token,
    json: { target_acct: normalizeFederationAcct(targetAcct) },
  });
}

export async function unblockFederationUser(token: string, targetAcct: string): Promise<void> {
  await api(`/api/v1/me/federation/blocks?target_acct=${encodeURIComponent(normalizeFederationAcct(targetAcct))}`, {
    method: "DELETE",
    token,
  });
}

export async function muteFederationUser(token: string, targetAcct: string): Promise<void> {
  await api("/api/v1/me/federation/mutes", {
    method: "POST",
    token,
    json: { target_acct: normalizeFederationAcct(targetAcct) },
  });
}

export async function unmuteFederationUser(token: string, targetAcct: string): Promise<void> {
  await api(`/api/v1/me/federation/mutes?target_acct=${encodeURIComponent(normalizeFederationAcct(targetAcct))}`, {
    method: "DELETE",
    token,
  });
}
