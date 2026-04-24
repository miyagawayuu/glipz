import { api } from "../lib/api";
import { getAccessToken } from "../auth";

export type FanclubLinkActions = {
  connectMember?: () => Promise<void>;
  connectCreator?: () => Promise<void>;
  disconnectMember?: () => Promise<void>;
  disconnectCreator?: () => Promise<void>;
};

type UseFanclubLinksDeps = {
  providerId: string;
  setErr: (v: string) => void;
  setMsg: (v: string) => void;
  setLoading: (v: boolean) => void;
  confirm: (text: string) => boolean;
  t: (key: string) => string;
  refreshMe: () => Promise<void>;
};

export function useFanclubLinks(deps: UseFanclubLinksDeps): FanclubLinkActions {
  const { providerId, setErr, setMsg, setLoading, confirm, t, refreshMe } = deps;

  // Provider-specific routing stays intact for backwards compatibility.
  if (providerId !== "patreon") {
    return {};
  }

  async function connectMember() {
    setErr("");
    setMsg("");
    const token = getAccessToken();
    if (!token) return;
    setLoading(true);
    try {
      const res = await api<{ authorize_url: string }>("/api/v1/patreon/member/authorize-url", {
        method: "GET",
        token,
      });
      window.location.href = res.authorize_url;
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : t("views.settings.security.patreon.startFailed"));
    } finally {
      setLoading(false);
    }
  }

  async function connectCreator() {
    setErr("");
    setMsg("");
    const token = getAccessToken();
    if (!token) return;
    setLoading(true);
    try {
      const res = await api<{ authorize_url: string }>("/api/v1/patreon/creator/authorize-url", {
        method: "GET",
        token,
      });
      window.location.href = res.authorize_url;
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : t("views.settings.security.patreon.startFailed"));
    } finally {
      setLoading(false);
    }
  }

  async function disconnectMember() {
    if (!confirm(t("views.settings.security.patreon.confirmDisconnectMember"))) return;
    const token = getAccessToken();
    if (!token) return;
    setLoading(true);
    setErr("");
    try {
      await api("/api/v1/patreon/member/disconnect", { method: "POST", token });
      setMsg(t("views.settings.security.patreon.disconnectedMember"));
      await refreshMe();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : t("views.settings.security.patreon.disconnectFailed"));
    } finally {
      setLoading(false);
    }
  }

  async function disconnectCreator() {
    if (!confirm(t("views.settings.security.patreon.confirmDisconnectCreator"))) return;
    const token = getAccessToken();
    if (!token) return;
    setLoading(true);
    setErr("");
    try {
      await api("/api/v1/patreon/creator/disconnect", { method: "POST", token });
      setMsg(t("views.settings.security.patreon.disconnectedCreator"));
      await refreshMe();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : t("views.settings.security.patreon.disconnectFailed"));
    } finally {
      setLoading(false);
    }
  }

  return { connectMember, connectCreator, disconnectMember, disconnectCreator };
}

