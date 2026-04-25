import { api } from "./api";

export type PayPalPlanRow = {
  id: string;
  plan_id: string;
  label?: string;
  active?: boolean;
};

export async function listPayPalPlans(token: string): Promise<PayPalPlanRow[]> {
  const res = await api<{ plans: PayPalPlanRow[] }>("/api/v1/payment/paypal/plans", { method: "GET", token });
  return Array.isArray(res.plans) ? res.plans : [];
}

export async function upsertPayPalPlan(
  token: string,
  input: { plan_id: string; label?: string; active?: boolean },
): Promise<void> {
  await api("/api/v1/payment/paypal/plans", { method: "POST", token, json: input });
}

export async function createPayPalSubscription(token: string, postID: string): Promise<{ approval_url: string }> {
  const res = await api<{ approval_url: string; subscription_id: string }>("/api/v1/payment/paypal/subscription/create", {
    method: "POST",
    token,
    json: { post_id: postID },
  });
  return { approval_url: res.approval_url };
}

export async function requestPayPalEntitlement(token: string, postID: string): Promise<string> {
  const res = await api<{ entitlement_jwt: string }>("/api/v1/payment/paypal/entitlement", {
    method: "POST",
    token,
    json: { post_id: postID },
  });
  return res.entitlement_jwt;
}

