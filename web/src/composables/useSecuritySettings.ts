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

export type MeResp = {
  id: string;
  email: string;
  totp_enabled: boolean;
  dm_invite_auto_accept?: boolean;
};

export function useSecuritySettings() {
  const { t } = useI18n();
  const router = useRouter();
  const route = useRoute();
  const me = ref<MeResp | null>(null);
  const secret = ref("");
  const uri = ref("");
  const qrDataUrl = ref("");
  const code = ref("");
  const mfaPassword = ref("");
  const err = ref("");
  const msg = ref("");
  /** Save message dedicated to the DM settings card so it stays separate from 2FA and other messages. */
  const dmSaveMsg = ref("");
  const loading = ref(false);
  const webPushConfig = ref<WebPushConfig | null>(null);
  const webPushBusy = ref(false);
  const webPushPermission = ref<NotificationPermission | "unsupported">("unsupported");
  const webPushBrowserSubscribed = ref(false);
  const dmInviteAutoAccept = ref(false);
  const dmInviteAutoDirty = computed(() => dmInviteAutoAccept.value !== !!me.value?.dm_invite_auto_accept);

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
    dmInviteAutoAccept.value = !!u.dm_invite_auto_accept;
    dmSaveMsg.value = "";
    await refreshWebPushState();
  }

  onMounted(async () => {
    await refresh();
  });

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
        json: { password: mfaPassword.value },
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
        json: { code: code.value, password: mfaPassword.value },
      });
      code.value = "";
      mfaPassword.value = "";
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

  async function saveDMSettings() {
    err.value = "";
    msg.value = "";
    dmSaveMsg.value = "";
    const token = getAccessToken();
    if (!token) return;
    loading.value = true;
    try {
      await api("/api/v1/me/dm-settings", {
        method: "PATCH",
        token,
        json: {
          dm_invite_auto_accept: dmInviteAutoAccept.value,
        },
      });
      dmSaveMsg.value = t("views.settings.directMessages.savedToast");
      await refresh();
      bumpMeHub();
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : t("views.settings.directMessages.saveFailed");
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
    mfaPassword,
    err,
    msg,
    dmSaveMsg,
    loading,
    webPushConfig,
    webPushBusy,
    webPushPermission,
    webPushBrowserSubscribed,
    dmInviteAutoAccept,
    dmInviteAutoDirty,
    webPushStatusLabel,
    refresh,
    setup,
    enable,
    saveDMSettings,
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
