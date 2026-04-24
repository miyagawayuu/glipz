import type { InjectionKey } from "vue";
import { computed, inject, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { api } from "../lib/api";
import { getAccessToken } from "../auth";
import { bumpMeHub } from "../meHub";
import {
  currentNotificationPermission,
  disableWebPush,
  enableWebPush,
  fetchWebPushConfig,
  getCurrentPushSubscription,
  isWebPushSupported,
  syncExistingPushSubscription,
  type WebPushConfig,
} from "../lib/webPush";
import { listDMThreads, type DMThread } from "../lib/dm";

export type MeResp = {
  id: string;
  email: string;
  totp_enabled: boolean;
  dm_call_timeout_seconds?: number;
  dm_call_enabled?: boolean;
  dm_call_scope?: "none" | "all" | "followers" | "specific_users";
  dm_call_allowed_user_ids?: string[];
  dm_invite_auto_accept?: boolean;
  fanclubs?: Record<string, { member_linked: boolean; creator_linked: boolean }>;
};

export function useSecuritySettings() {
  const { t, te } = useI18n();
  const router = useRouter();
  const route = useRoute();
  const me = ref<MeResp | null>(null);
  const secret = ref("");
  const uri = ref("");
  const qrDataUrl = ref("");
  const code = ref("");
  const err = ref("");
  const msg = ref("");
  /** Save message dedicated to the DM settings card so it stays separate from 2FA and other messages. */
  const dmSaveMsg = ref("");
  const loading = ref(false);
  const webPushConfig = ref<WebPushConfig | null>(null);
  const webPushBusy = ref(false);
  const webPushPermission = ref<NotificationPermission | "unsupported">("unsupported");
  const webPushBrowserSubscribed = ref(false);
  const dmCallTimeoutSeconds = ref("30");
  const dmCallTimeoutOptions = computed(() =>
    (["15", "30", "45", "60", "90", "120"] as const).map((value) => ({
      value,
      label: t(`views.settings.security.dmCallTimeoutSeconds.${value}`),
    })),
  );
  const dmCallTimeoutDirty = computed(
    () => dmCallTimeoutSeconds.value !== String(me.value?.dm_call_timeout_seconds ?? 30),
  );
  const dmCallEnabled = ref(false);
  const dmCallScope = ref<"none" | "all" | "followers" | "specific_users">("none");
  const dmCallAllowedUserIDs = ref<string[]>([]);
  const selectableThreads = ref<DMThread[]>([]);
  const dmInviteAutoAccept = ref(false);
  const dmCallPolicyDirty = computed(
    () =>
      dmCallEnabled.value !== !!me.value?.dm_call_enabled
      || dmCallScope.value !== (me.value?.dm_call_scope ?? "none")
      || JSON.stringify([...dmCallAllowedUserIDs.value].sort()) !== JSON.stringify([...(me.value?.dm_call_allowed_user_ids ?? [])].sort()),
  );
  const dmInviteAutoDirty = computed(() => dmInviteAutoAccept.value !== !!me.value?.dm_invite_auto_accept);
  const fanclubLinking = ref(false);

  watch(
    () => uri.value,
    async (u) => {
      if (!u) {
        qrDataUrl.value = "";
        return;
      }
      try {
        const QR = (await import("qrcode")).default;
        qrDataUrl.value = await QR.toDataURL(u, {
          width: 240,
          margin: 2,
          errorCorrectionLevel: "M",
          color: { dark: "#171717", light: "#ffffff" },
        });
      } catch {
        qrDataUrl.value = "";
      }
    },
  );

  function resolveFanclubReturnQuery(q: Record<string, unknown>): string | null {
    // Future shape (planned): ?fanclub=<provider>&result=<code>
    if (typeof q.fanclub === "string" && typeof q.result === "string") {
      const provider = q.fanclub;
      const result = q.result;
      const key = `views.settings.security.${provider}Return.${result}`;
      if (te(key)) return t(key);
    }
    return null;
  }

  async function refresh() {
    const token = getAccessToken();
    if (!token) {
      await router.replace("/login");
      return;
    }
    const u = await api<MeResp>("/api/v1/me", {
      method: "GET",
      token,
    });
    me.value = u;
    dmCallTimeoutSeconds.value = String(u.dm_call_timeout_seconds ?? 30);
    dmCallEnabled.value = !!u.dm_call_enabled;
    dmCallScope.value = (u.dm_call_scope ?? "none") as typeof dmCallScope.value;
    dmCallAllowedUserIDs.value = [...(u.dm_call_allowed_user_ids ?? [])];
    dmInviteAutoAccept.value = !!u.dm_invite_auto_accept;
    dmSaveMsg.value = "";
    selectableThreads.value = await listDMThreads().catch(() => []);
    await refreshWebPushState();
  }

  onMounted(async () => {
    const qm = resolveFanclubReturnQuery(route.query as Record<string, unknown>);
    if (qm) {
      msg.value = qm;
      await router.replace({ path: "/settings", query: {} });
    }
    await refresh();
  });

  async function connectPatreonMember() {
    err.value = "";
    msg.value = "";
    const token = getAccessToken();
    if (!token) return;
    fanclubLinking.value = true;
    try {
      const res = await api<{ authorize_url: string }>("/api/v1/fanclub/patreon/member/authorize-url", {
        method: "GET",
        token,
      });
      if (res.authorize_url) window.location.href = res.authorize_url;
      else err.value = t("views.settings.security.fanclubLinkFailed");
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : t("views.settings.security.fanclubLinkFailed");
    } finally {
      fanclubLinking.value = false;
    }
  }

  async function connectPatreonCreator() {
    err.value = "";
    msg.value = "";
    const token = getAccessToken();
    if (!token) return;
    fanclubLinking.value = true;
    try {
      const res = await api<{ authorize_url: string }>("/api/v1/fanclub/patreon/creator/authorize-url", {
        method: "GET",
        token,
      });
      if (res.authorize_url) window.location.href = res.authorize_url;
      else err.value = t("views.settings.security.fanclubLinkFailed");
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : t("views.settings.security.fanclubLinkFailed");
    } finally {
      fanclubLinking.value = false;
    }
  }

  async function disconnectMember() {
    err.value = "";
    msg.value = "";
    const token = getAccessToken();
    if (!token) return;
    fanclubLinking.value = true;
    try {
      await api("/api/v1/fanclub/patreon/member/disconnect", { method: "POST", token });
      msg.value = t("views.settings.security.fanclubDisconnected");
      await refresh();
      bumpMeHub();
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : t("views.settings.security.fanclubDisconnectFailed");
    } finally {
      fanclubLinking.value = false;
    }
  }

  async function disconnectCreator() {
    err.value = "";
    msg.value = "";
    const token = getAccessToken();
    if (!token) return;
    fanclubLinking.value = true;
    try {
      await api("/api/v1/fanclub/patreon/creator/disconnect", { method: "POST", token });
      msg.value = t("views.settings.security.fanclubDisconnected");
      await refresh();
      bumpMeHub();
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : t("views.settings.security.fanclubDisconnectFailed");
    } finally {
      fanclubLinking.value = false;
    }
  }

  async function setup() {
    err.value = "";
    msg.value = "";
    const token = getAccessToken();
    if (!token) return;
    loading.value = true;
    try {
      const res = await api<{ secret: string; uri: string }>("/api/v1/auth/mfa/setup", {
        method: "POST",
        token,
      });
      secret.value = res.secret;
      uri.value = res.uri;
      msg.value = t("views.settings.security.mfa.setupReady");
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : t("views.settings.security.mfa.setupFailed");
    } finally {
      loading.value = false;
    }
  }

  async function enable() {
    err.value = "";
    msg.value = "";
    const token = getAccessToken();
    if (!token) return;
    loading.value = true;
    try {
      await api("/api/v1/auth/mfa/enable", {
        method: "POST",
        token,
        json: { code: code.value },
      });
      code.value = "";
      secret.value = "";
      uri.value = "";
      msg.value = t("views.settings.security.mfa.enabledToast");
      await refresh();
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : t("views.settings.security.mfa.enableFailed");
    } finally {
      loading.value = false;
    }
  }

  async function saveDMCallSettings() {
    err.value = "";
    msg.value = "";
    dmSaveMsg.value = "";
    const token = getAccessToken();
    if (!token) return;
    const seconds = Number.parseInt(dmCallTimeoutSeconds.value, 10);
    if (!Number.isFinite(seconds) || seconds < 5 || seconds > 300) {
      err.value = t("views.settings.security.dmCalls.timeoutInvalid");
      return;
    }
    loading.value = true;
    try {
      await api("/api/v1/me/dm-settings", {
        method: "PATCH",
        token,
        json: {
          call_timeout_seconds: seconds,
          call_enabled: dmCallEnabled.value,
          call_scope: dmCallEnabled.value ? dmCallScope.value : "none",
          allowed_user_ids: dmCallEnabled.value && dmCallScope.value === "specific_users" ? dmCallAllowedUserIDs.value : [],
          dm_invite_auto_accept: dmInviteAutoAccept.value,
        },
      });
      dmSaveMsg.value = t("views.settings.directMessages.savedToast");
      await refresh();
      bumpMeHub();
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : t("views.settings.security.dmCalls.saveFailed");
    } finally {
      loading.value = false;
    }
  }

  async function refreshWebPushState() {
    webPushPermission.value = currentNotificationPermission();
    if (!isWebPushSupported()) {
      webPushConfig.value = { available: false, subscription_count: 0 };
      webPushBrowserSubscribed.value = false;
      return;
    }
    const [config, subscription] = await Promise.all([
      fetchWebPushConfig().catch(() => ({ available: false, subscription_count: 0 } as WebPushConfig)),
      getCurrentPushSubscription().catch(() => null),
    ]);
    webPushConfig.value = config;
    webPushBrowserSubscribed.value = !!subscription;
    if (config.available && subscription) {
      await syncExistingPushSubscription().catch(() => undefined);
      const refreshed = await fetchWebPushConfig().catch(() => config);
      webPushConfig.value = refreshed;
    }
  }

  const webPushStatusLabel = computed(() => {
    if (!isWebPushSupported()) return t("views.settings.security.webPush.statusUnsupported");
    if (!webPushConfig.value?.available) return t("views.settings.security.webPush.statusServerOff");
    if (webPushPermission.value === "denied") return t("views.settings.security.webPush.statusDenied");
    if (webPushBrowserSubscribed.value) return t("views.settings.security.webPush.statusSubscribed");
    if (webPushPermission.value === "granted") return t("views.settings.security.webPush.statusGrantedNotSubscribed");
    return t("views.settings.security.webPush.statusCanEnable");
  });

  async function enableWebPushNotifications() {
    err.value = "";
    msg.value = "";
    if (!webPushConfig.value?.available) {
      err.value = t("views.settings.security.webPush.errServerOff");
      return;
    }
    if (!webPushConfig.value.vapid_public_key) {
      err.value = t("views.settings.security.webPush.errNoVapid");
      return;
    }
    webPushBusy.value = true;
    try {
      await enableWebPush(webPushConfig.value.vapid_public_key);
      msg.value = t("views.settings.security.webPush.enabledToast");
      await refreshWebPushState();
    } catch (e: unknown) {
      const message = e instanceof Error ? e.message : t("views.settings.security.webPush.enableFailed");
      err.value =
        message === "notification_permission_denied"
          ? t("views.settings.security.webPush.errPermissionDenied")
          : message === "web_push_unsupported"
            ? t("views.settings.security.webPush.errUnsupported")
            : message === "service_worker_registration_failed"
              ? t("views.settings.security.webPush.errSwFailed")
              : message;
    } finally {
      webPushBusy.value = false;
    }
  }

  async function disableWebPushNotifications() {
    err.value = "";
    msg.value = "";
    webPushBusy.value = true;
    try {
      await disableWebPush();
      msg.value = t("views.settings.security.webPush.disabledToast");
      await refreshWebPushState();
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : t("views.settings.security.webPush.disableFailed");
    } finally {
      webPushBusy.value = false;
    }
  }

  return {
    me,
    secret,
    uri,
    qrDataUrl,
    code,
    err,
    msg,
    dmSaveMsg,
    loading,
    webPushConfig,
    webPushBusy,
    webPushPermission,
    webPushBrowserSubscribed,
    dmCallTimeoutSeconds,
    dmCallTimeoutOptions,
    dmCallTimeoutDirty,
    dmCallEnabled,
    dmCallScope,
    dmCallAllowedUserIDs,
    selectableThreads,
    dmCallPolicyDirty,
    dmInviteAutoAccept,
    dmInviteAutoDirty,
    webPushStatusLabel,
    refresh,
    setup,
    enable,
    saveDMCallSettings,
    connectPatreonMember,
    connectPatreonCreator,
    disconnectMember,
    disconnectCreator,
    fanclubLinking,
    enableWebPushNotifications,
    disableWebPushNotifications,
  };
}

export type SecuritySettingsContext = ReturnType<typeof useSecuritySettings>;

export const securitySettingsKey: InjectionKey<SecuritySettingsContext> = Symbol("securitySettings");

export function useSecuritySettingsContext(): SecuritySettingsContext {
  const ctx = inject(securitySettingsKey);
  if (!ctx) {
    throw new Error("useSecuritySettingsContext: use under SettingsView with provide(securitySettingsKey, …)");
  }
  return ctx;
}
