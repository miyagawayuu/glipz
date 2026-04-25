import { api } from "./api";

export async function requestGumroadEntitlement(token: string, postId: string, licenseKey: string): Promise<string> {
  const res = await api<{ entitlement_jwt: string }>("/api/v1/fanclub/gumroad/entitlement", {
    method: "POST",
    token,
    json: { post_id: postId, license_key: licenseKey },
  });
  return res.entitlement_jwt;
}
