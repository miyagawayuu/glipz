import type { LocationQuery, LocationQueryRaw } from "vue-router";

export type ComposerReplyTarget = {
  id: string;
  user_handle: string;
  is_federated?: boolean;
  remote_object_url?: string;
};

export function isMobileViewport(): boolean {
  return typeof window !== "undefined" && window.matchMedia("(max-width: 1023.98px)").matches;
}

export function composeRoutePath(): string {
  return isMobileViewport() ? "/compose" : "/feed";
}

export function buildComposerReplyQuery(target: {
  id: string;
  user_handle?: string | null;
  is_federated?: boolean;
  remote_object_url?: string | null;
}): LocationQueryRaw {
  return {
    reply: target.id,
    rh: target.user_handle ?? "",
    ...(target.is_federated ? { rf: "1", rou: target.remote_object_url ?? "" } : {}),
  };
}

export function parseComposerReplyQuery(query: LocationQuery): ComposerReplyTarget | null {
  const reply = typeof query.reply === "string" ? query.reply : Array.isArray(query.reply) ? String(query.reply[0] ?? "") : "";
  if (!reply) return null;
  const userHandle = typeof query.rh === "string" ? query.rh : Array.isArray(query.rh) ? String(query.rh[0] ?? "") : "";
  const federatedFlag = typeof query.rf === "string" ? query.rf : Array.isArray(query.rf) ? String(query.rf[0] ?? "") : "";
  const remoteObjectUrl = typeof query.rou === "string" ? query.rou : Array.isArray(query.rou) ? String(query.rou[0] ?? "") : "";
  return {
    id: reply,
    user_handle: userHandle,
    is_federated: federatedFlag === "1",
    remote_object_url: remoteObjectUrl || undefined,
  };
}
