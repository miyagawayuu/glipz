<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, provide, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import logoImg from "./assets/logo.png";
import Icon from "./components/Icon.vue";
import UserBadges from "./components/UserBadges.vue";
import { ACCESS, clearTokens, getAccessToken } from "./auth";
import { api, displayInstanceDomain } from "./lib/api";
import { displayName as displayNameFromEmail } from "./lib/feedDisplay";
import {
  applyTheme,
  persistThemePreference,
  readStoredThemePreference,
  systemThemeMediaQuery,
  type ThemePreference,
} from "./lib/theme";
import { connectNotifyStream, notifyToastMessage, type NotifyPayload } from "./lib/notifyStream";
import { connectDMStream, type DmStreamPayload } from "./lib/dmStream";
import { clearRememberedUnlockedIdentity } from "./lib/dmUnlockMemory";
import { meHubTick } from "./meHub";
import {
  incrementUnreadNotificationCount,
  pingNotificationReceived,
  refreshUnreadNotificationCount,
  unreadNotificationCount,
} from "./notificationHub";
import {
  incrementUnreadDMCount,
  pingDMReceived,
  refreshUnreadDMCount,
  unreadDMCount,
} from "./dmHub";
import { getOperatorAnnouncements } from "./data/operatorAnnouncements";
import { fetchPublicInstanceSettings, type OperatorAnnouncement } from "./lib/instanceSettings";
import { legalDocumentLink, type LegalDocumentKey, type LegalDocumentURLSettings } from "./lib/legalDocumentLinks";
import { safeHttpURL } from "./lib/redirect";

const route = useRoute();
const router = useRouter();
const { t } = useI18n();
const authed = ref(!!getAccessToken());
const me = ref<{
  id: string;
  email: string;
  handle: string;
  display_name: string;
  badges?: string[];
  avatar_url: string | null;
  is_site_admin?: boolean;
  fanclub_patreon_enabled?: boolean;
  fanclub_gumroad_enabled?: boolean;
  payment_paypal_enabled?: boolean;
} | null>(null);
provide("appMe", me);
/** Falls back to initials when loading an avatar URL fails. */
const avatarImgFailed = ref(false);
const profileMenuOpen = ref(false);
const profileMenuRoot = ref<HTMLElement | null>(null);
/** Shows the sidebar as a drawer below the lg breakpoint. */
const mobileNavOpen = ref(false);
const appHeaderEl = ref<HTMLElement | null>(null);
const appHeaderOffset = ref("56px");
const searchQuery = ref("");
const themePreference = ref<ThemePreference>(readStoredThemePreference());

function setThemePreferenceSilent(next: ThemePreference) {
  themePreference.value = next;
}
provide("setThemePreferenceSilent", setThemePreferenceSilent);

const operatorAnnouncements = ref<OperatorAnnouncement[]>(getOperatorAnnouncements());
const legalDocumentUrls = ref<LegalDocumentURLSettings>({});

async function loadOperatorAnnouncements() {
  try {
    const settings = await fetchPublicInstanceSettings();
    legalDocumentUrls.value = settings;
    operatorAnnouncements.value = settings.operator_announcements.length
      ? settings.operator_announcements
      : getOperatorAnnouncements();
  } catch {
    operatorAnnouncements.value = getOperatorAnnouncements();
  }
}

function legalLink(key: LegalDocumentKey): { href: string; external: boolean } {
  return legalDocumentLink(legalDocumentUrls.value, key);
}

function currentDomain(): string {
  return displayInstanceDomain();
}

function looksLikeRemoteAcct(q: string): boolean {
  const s = q.trim().replace(/^@/, "");
  if (!s.includes("@")) return false;
  const parts = s.split("@");
  return parts.length === 2 && parts[0].length > 0 && parts[1].length > 0;
}

function looksLikeActorURL(q: string): boolean {
  return /^https?:\/\//i.test(q.trim());
}

function navigateFromSearch() {
  const q = searchQuery.value.trim();
  if (!q) return;
  if (looksLikeActorURL(q)) {
    router.push({ path: "/remote/profile", query: { actor: q.trim() } });
    searchQuery.value = "";
    return;
  }
  if (looksLikeRemoteAcct(q)) {
    const acct = q.trim().replace(/^@/, "");
    const at = acct.indexOf("@");
    const user = acct.slice(0, at);
    const host = acct.slice(at + 1);
    router.push({ path: `/@${user}@${host}` });
    searchQuery.value = "";
    return;
  }
  if (authed.value) {
    router.push({ path: "/search", query: { q } });
    searchQuery.value = "";
  }
}

function onGlobalSearchEnter() {
  navigateFromSearch();
}

function syncSearchQueryFromRoute() {
  if (route.path !== "/search") {
    searchQuery.value = "";
    return;
  }
  const raw = route.query.q;
  if (typeof raw === "string") {
    searchQuery.value = raw;
    return;
  }
  if (Array.isArray(raw)) {
    searchQuery.value = String(raw[0] ?? "");
    return;
  }
  searchQuery.value = "";
}

function syncAppHeaderOffset() {
  if (typeof window === "undefined") return;
  const height = appHeaderEl.value?.getBoundingClientRect().height ?? 56;
  appHeaderOffset.value = `${Math.max(56, Math.round(height))}px`;
}

let disconnectNotifyStream: (() => void) | null = null;
let disconnectDMStream: (() => void) | null = null;
const notifyToastMessageText = ref("");
let notifyToastTimer: ReturnType<typeof setTimeout> | null = null;
let themeMediaQuery: MediaQueryList | null = null;

function syncTheme() {
  applyTheme(themePreference.value);
}

function onSystemThemeChange() {
  if (themePreference.value === "system") {
    syncTheme();
  }
}

const usesGuestSimpleLayout = computed(() => {
  if (route.meta.guestSimpleLayout === true) return true;
  if (route.path === "/login" || route.path === "/register") return true;
  if (route.path === "/about") return true;
  if (authed.value) return false;
  return (
    route.path === "/legal/terms"
    || route.path === "/legal/privacy"
    || route.path === "/legal/nsfw-guidelines"
    || route.path === "/legal/api-guidelines"
    || route.path === "/federation/guidelines"
  );
});
const hideRightAside = computed(() => route.meta.hideRightAside === true);
const wideMain = computed(() => route.meta.wideMain === true);
const mobileEdgeToEdge = computed(() => route.meta.mobileEdgeToEdge === true);
const hideMobileChrome = computed(() => route.meta.hideMobileChrome === true);
const isAdminShell = computed(() => route.meta.adminShell === true);
const useViewportScroll = computed(() => authed.value && !isAdminShell.value && !usesGuestSimpleLayout.value && !wideMain.value && !hideRightAside.value);
const appRootClass = computed(() => {
  if (isAdminShell.value) return "min-h-screen";
  const base = useViewportScroll.value ? "min-h-screen" : authed.value ? "h-[100dvh] max-h-[100dvh]" : "min-h-screen";
  /** Reserve the top safe area at the root level when the header is hidden. */
  const topSafe =
    usesGuestSimpleLayout.value
      ? "pt-[env(safe-area-inset-top,0px)]"
      : hideMobileChrome.value && authed.value
        ? "max-lg:pt-[env(safe-area-inset-top,0px)]"
        : "";
  return [base, topSafe].filter(Boolean).join(" ");
});
const headerContainerClass = computed(() => {
  if (isAdminShell.value) return "max-w-none";
  if (wideMain.value) return "max-w-none";
  if (!authed.value) return "max-w-[598px]";
  return "max-w-[min(100%,92rem)]";
});
const shellClass = computed(() => {
  if (isAdminShell.value) return "max-w-none flex-col";
  if (usesGuestSimpleLayout.value) return "max-w-none flex-col py-8";
  if (wideMain.value) {
    return authed.value
      ? "max-w-none flex-row flex-nowrap items-stretch justify-start gap-[20px] overflow-hidden"
      : "max-w-none flex-col py-8";
  }
  if (!authed.value) return "max-w-[598px] flex-col py-8";
  if (useViewportScroll.value) {
    return "max-w-[min(100%,92rem)] flex-row flex-nowrap items-stretch justify-center gap-[20px]";
  }
  return "max-w-[min(100%,92rem)] flex-row flex-nowrap items-stretch justify-center gap-[20px] overflow-hidden";
});
const mainFooterNavItems = computed(() => [
  { to: "/feed", label: t("app.nav.home"), icon: "home" as const },
  { to: "/search", label: t("app.nav.search"), icon: "search" as const },
  { to: "/notifications", label: t("app.nav.notifications"), icon: "bell" as const },
  { to: "/messages", label: t("app.nav.messages"), icon: "message" as const },
]);
const mobileFooterPaddingClass = computed(() =>
  isAdminShell.value || usesGuestSimpleLayout.value || hideMobileChrome.value ? "" : "max-lg:pb-[calc(4.5rem+env(safe-area-inset-bottom))]",
);
const mainClass = computed(() => {
  if (isAdminShell.value) return "min-h-screen w-full bg-white";
  if (usesGuestSimpleLayout.value) return "w-full";
  if (wideMain.value) {
    return authed.value
      ? `flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-y-auto bg-white ${mobileFooterPaddingClass.value} lg:border-l lg:border-neutral-200`.trim()
      : "w-full";
  }
  if (!authed.value) return "w-full";
  if (useViewportScroll.value) {
    return `flex min-h-full min-w-0 max-w-[598px] flex-[0_1_598px] flex-col self-stretch border-x border-neutral-200 bg-white ${mobileFooterPaddingClass.value}`.trim();
  }
  return `flex h-full min-h-0 min-w-0 max-w-[598px] flex-[0_1_598px] flex-col overflow-y-auto border-x border-neutral-200 bg-white ${mobileFooterPaddingClass.value}`.trim();
});
const shellPaddingClass = computed(() =>
  isAdminShell.value ? "px-0" : mobileEdgeToEdge.value ? "px-0 sm:px-[20px]" : "px-[20px]",
);
const isFeedRoute = computed(() => route.path === "/feed" || route.path === "/feed/scheduled");
const isSearchRoute = computed(() => route.path === "/search");
const isNotificationsRoute = computed(() => route.path === "/notifications");
const isMessagesRoute = computed(() => route.path === "/messages" || route.path.startsWith("/messages/"));

function isFooterNavActive(path: string): boolean {
  if (path === "/feed") return isFeedRoute.value;
  if (path === "/search") return isSearchRoute.value;
  if (path === "/notifications") return isNotificationsRoute.value;
  if (path === "/messages") return isMessagesRoute.value;
  return false;
}

function footerNavBadge(path: string): number {
  if (path === "/notifications") return unreadNotificationCount.value;
  if (path === "/messages") return unreadDMCount.value;
  return 0;
}

async function loadMe() {
  const token = getAccessToken();
  if (!token) {
    me.value = null;
    return;
  }
  try {
    const u = await api<{
      id: string;
      email: string;
      handle: string;
      display_name?: string;
      badges?: string[];
      avatar_url?: string | null;
      is_site_admin?: boolean;
      fanclub_patreon_enabled?: boolean;
      fanclub_gumroad_enabled?: boolean;
      payment_paypal_enabled?: boolean;
    }>("/api/v1/me", {
      method: "GET",
      token,
    });
    avatarImgFailed.value = false;
    me.value = {
      id: u.id,
      email: u.email,
      handle: u.handle ?? "",
      display_name: (u.display_name ?? "").trim() || displayNameFromEmail(u.email),
      badges: Array.isArray(u.badges) ? u.badges.map((badge) => String(badge)) : [],
      avatar_url: safeHttpURL(u.avatar_url) || null,
      is_site_admin: !!u.is_site_admin,
      fanclub_patreon_enabled: !!u.fanclub_patreon_enabled,
      fanclub_gumroad_enabled: !!u.fanclub_gumroad_enabled,
      payment_paypal_enabled: !!u.payment_paypal_enabled,
    };
    await refreshUnreadNotificationCount();
    await refreshUnreadDMCount();
  } catch {
    me.value = null;
  }
}

function stopNotifyStream() {
  disconnectNotifyStream?.();
  disconnectNotifyStream = null;
}

function stopDMStream() {
  disconnectDMStream?.();
  disconnectDMStream = null;
}

function showNotifyToast(msg: string) {
  if (notifyToastTimer) clearTimeout(notifyToastTimer);
  notifyToastMessageText.value = msg;
  notifyToastTimer = setTimeout(() => {
    notifyToastMessageText.value = "";
    notifyToastTimer = null;
  }, 5200);
}

function startNotifyStream() {
  stopNotifyStream();
  const token = getAccessToken();
  if (!token || !me.value) return;
  disconnectNotifyStream = connectNotifyStream({
    token,
    onPayload: (p: NotifyPayload) => {
      incrementUnreadNotificationCount();
      pingNotificationReceived();
      showNotifyToast(notifyToastMessage(p));
    },
  });
}

function startDMStream() {
  stopDMStream();
  const token = getAccessToken();
  if (!token || !me.value) return;
  disconnectDMStream = connectDMStream({
    token,
    onPayload: (p: DmStreamPayload) => {
      pingDMReceived(p);
      if (p.kind === "message" || p.kind === "federation_dm_invite" || p.kind === "federation_dm_message") {
        incrementUnreadDMCount();
      }
    },
  });
}

watch(
  () => route.fullPath,
  async () => {
    syncSearchQueryFromRoute();
    authed.value = !!getAccessToken();
    closeProfileMenu();
    mobileNavOpen.value = false;
    if (!authed.value) {
      me.value = null;
      stopNotifyStream();
      stopDMStream();
      return;
    }
    await loadMe();
  },
  { immediate: true },
);

watch(meHubTick, () => {
  if (authed.value) void loadMe();
});

watch(
  () => [me.value, authed.value] as const,
  () => {
    stopNotifyStream();
    stopDMStream();
    if (authed.value && me.value && getAccessToken()) {
      startNotifyStream();
      startDMStream();
    }
  },
  { immediate: true },
);

watch(
  themePreference,
  (next) => {
    persistThemePreference(next);
    syncTheme();
  },
  { immediate: true },
);

function closeProfileMenu() {
  profileMenuOpen.value = false;
}

function closeMobileNav() {
  mobileNavOpen.value = false;
}

function toggleMobileNav() {
  mobileNavOpen.value = !mobileNavOpen.value;
}

function onAsideNavClick(ev: MouseEvent) {
  const t = ev.target as HTMLElement | null;
  if (t?.closest("a")) {
    closeMobileNav();
  }
}

function toggleProfileMenu() {
  profileMenuOpen.value = !profileMenuOpen.value;
}

async function logout() {
  closeProfileMenu();
  stopNotifyStream();
  stopDMStream();
  clearRememberedUnlockedIdentity();
  notifyToastMessageText.value = "";
  try {
    await api("/api/v1/auth/logout", { method: "POST" });
  } catch {
    /* ignore logout network failures */
  }
  clearTokens();
  router.push("/login");
}

function syncAuthStateFromStorage() {
  const loggedIn = !!getAccessToken();
  authed.value = loggedIn;
  if (!loggedIn) {
    me.value = null;
    stopNotifyStream();
    stopDMStream();
    clearRememberedUnlockedIdentity();
    if (route.meta.requiresAuth) {
      void router.replace({ path: "/login", query: { next: route.fullPath } });
    }
    return;
  }
  void loadMe();
}

function onStorage(ev: StorageEvent) {
  if (ev.key !== null && ev.key !== ACCESS) return;
  syncAuthStateFromStorage();
}

function onDocumentPointerDown(ev: PointerEvent) {
  if (!profileMenuOpen.value) return;
  const root = profileMenuRoot.value;
  if (root && !root.contains(ev.target as Node)) {
    closeProfileMenu();
  }
}

function onDocumentKeydown(ev: KeyboardEvent) {
  if (ev.key === "Escape") {
    closeProfileMenu();
    closeMobileNav();
  }
}

let appHeaderResizeObserver: ResizeObserver | null = null;

onMounted(() => {
  themeMediaQuery = systemThemeMediaQuery();
  themeMediaQuery?.addEventListener("change", onSystemThemeChange);
  document.addEventListener("pointerdown", onDocumentPointerDown);
  document.addEventListener("keydown", onDocumentKeydown);
  window.addEventListener("storage", onStorage);
  void nextTick(syncAppHeaderOffset);
  if (typeof ResizeObserver !== "undefined") {
    appHeaderResizeObserver = new ResizeObserver(() => {
      syncAppHeaderOffset();
    });
    if (appHeaderEl.value) appHeaderResizeObserver.observe(appHeaderEl.value);
  }
  window.addEventListener("resize", syncAppHeaderOffset);
  void loadOperatorAnnouncements();
});

onBeforeUnmount(() => {
  stopNotifyStream();
  stopDMStream();
  themeMediaQuery?.removeEventListener("change", onSystemThemeChange);
  document.removeEventListener("pointerdown", onDocumentPointerDown);
  document.removeEventListener("keydown", onDocumentKeydown);
  window.removeEventListener("storage", onStorage);
  window.removeEventListener("resize", syncAppHeaderOffset);
  appHeaderResizeObserver?.disconnect();
  appHeaderResizeObserver = null;
});

watch(
  () => route.fullPath,
  () => {
    void nextTick(syncAppHeaderOffset);
  },
  { flush: "post" },
);

function avatarInitials(email: string): string {
  const local = email.split("@")[0] ?? "";
  const cleaned = local.replace(/[^a-zA-Z0-9]/g, "");
  if (cleaned.length >= 2) {
    return cleaned.slice(0, 2).toUpperCase();
  }
  if (local.length >= 2) {
    return local.slice(0, 2).toUpperCase();
  }
  return (local[0] ?? "?").toUpperCase();
}
</script>

<template>
  <div
    class="flex flex-col bg-white text-neutral-900"
    :class="appRootClass"
    :style="{ '--app-header-offset': appHeaderOffset }"
  >
    <div
      v-if="notifyToastMessageText"
      class="fixed right-4 z-[200] max-w-sm rounded-xl border border-lime-200 bg-white px-4 py-3 text-sm text-neutral-900 shadow-lg ring-1 ring-black/5 max-lg:top-[calc(1rem+env(safe-area-inset-top,0px))] lg:top-4"
      role="status"
    >
      {{ notifyToastMessageText }}
    </div>
    <header
      ref="appHeaderEl"
      v-if="!usesGuestSimpleLayout && !isAdminShell"
      class="sticky top-0 z-10 shrink-0 border-b border-lime-200 bg-white/90 pt-[env(safe-area-inset-top,0px)] backdrop-blur"
      :class="hideMobileChrome ? 'max-lg:hidden' : ''"
    >
      <div
        class="mx-auto w-full px-[20px]"
        :class="headerContainerClass"
      >
        <div
          v-if="authed"
          class="grid w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 py-3 lg:flex lg:items-end lg:justify-center lg:gap-[20px] lg:pb-0"
        >
          <div
            class="flex min-w-0 shrink-0 items-center lg:w-60 lg:max-w-[min(100vw-2rem,16rem)] lg:gap-2 lg:pb-3"
          >
            <button
              type="button"
              class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-neutral-200 text-neutral-700 hover:bg-neutral-50 lg:hidden"
              :aria-label="mobileNavOpen ? $t('app.menu.close') : $t('app.menu.open')"
              :aria-expanded="mobileNavOpen"
              aria-controls="app-sidebar"
              @click="toggleMobileNav"
            >
              <Icon v-if="!mobileNavOpen" name="menu" class="h-5 w-5" />
              <Icon v-else name="close" class="h-5 w-5" />
            </button>
            <RouterLink
              to="/feed"
              class="hidden min-w-0 shrink-0 items-center py-0.5 hover:opacity-90 lg:flex"
              aria-label="Glipz ホーム"
            >
              <img :src="logoImg" alt="Glipz" class="h-8 w-auto max-h-9 object-contain object-left" />
            </RouterLink>
          </div>
          <div class="min-w-0 lg:hidden">
            <div v-if="isSearchRoute" class="mx-auto w-full max-w-[min(100%,22rem)]">
              <label class="sr-only" for="global-search-mobile">{{ $t("app.search.label") }}</label>
              <input
                id="global-search-mobile"
                v-model="searchQuery"
                type="search"
                name="q"
                :placeholder="$t('app.search.placeholder')"
                autocomplete="off"
                class="w-full rounded-full border border-neutral-200 bg-white px-4 py-2 text-sm text-neutral-900 shadow-sm outline-none ring-lime-500/30 transition placeholder:text-neutral-400 focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
                @keydown.enter.prevent="onGlobalSearchEnter"
              />
            </div>
            <div
              v-else-if="isMessagesRoute"
              class="flex min-w-0 items-center justify-center py-0.5"
            >
              <span class="truncate text-lg font-semibold text-neutral-900">{{ $t("app.search.messages") }}</span>
            </div>
            <RouterLink
              v-else
              to="/feed"
              class="flex min-w-0 items-center justify-center py-0.5 hover:opacity-90"
              aria-label="Glipz ホーム"
            >
              <img :src="logoImg" alt="Glipz" class="h-8 w-auto max-h-9 object-contain" />
            </RouterLink>
          </div>
          <div
            class="hidden min-h-0 min-w-0 max-w-[598px] shrink-0 self-end flex-[0_1_598px] lg:block"
          >
            <div id="app-view-header-slot-desktop" class="min-h-0" />
          </div>
          <div class="h-10 w-10 shrink-0 lg:hidden" aria-hidden="true" />
          <div
            class="hidden min-h-0 w-[350px] shrink-0 flex-col justify-center lg:flex lg:pb-3"
          >
            <div class="w-full min-w-0 max-w-[350px]">
              <label class="sr-only" for="global-search">{{ $t("app.search.label") }}</label>
              <input
                id="global-search"
                v-model="searchQuery"
                type="search"
                name="q"
                :placeholder="$t('app.search.placeholder')"
                autocomplete="off"
                class="w-full rounded-full border border-neutral-200 bg-white px-4 py-2 text-sm text-neutral-900 shadow-sm outline-none ring-lime-500/30 transition placeholder:text-neutral-400 focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
                @keydown.enter.prevent="onGlobalSearchEnter"
              />
            </div>
          </div>
        </div>
        <div v-else class="flex w-full flex-wrap items-center justify-between gap-3 py-3">
          <RouterLink
            to="/"
            class="flex shrink-0 items-center py-0.5 hover:opacity-90"
            aria-label="Glipz ホーム"
          >
            <img :src="logoImg" alt="Glipz" class="h-8 w-auto max-h-9 object-contain object-left" />
          </RouterLink>
          <div class="order-3 w-full min-w-0 sm:order-2 sm:mx-auto sm:max-w-[min(100%,350px)] sm:flex-1">
            <label class="sr-only" for="global-search-guest">{{ $t("app.search.label") }}</label>
            <input
              id="global-search-guest"
              v-model="searchQuery"
              type="search"
              name="q"
              :placeholder="$t('app.search.guestPlaceholder')"
              autocomplete="off"
              class="w-full rounded-full border border-neutral-200 bg-white px-4 py-2 text-sm text-neutral-900 shadow-sm outline-none ring-lime-500/30 transition placeholder:text-neutral-400 focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              @keydown.enter.prevent="onGlobalSearchEnter"
            />
          </div>
          <RouterLink
            v-if="!route.path.startsWith('/mfa')"
            to="/login"
            class="order-2 shrink-0 rounded-md bg-lime-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-lime-600 sm:order-3"
          >
            {{ $t("app.guest.login") }}
          </RouterLink>
        </div>
        <div
          v-if="authed"
          class="flex w-full items-start justify-center gap-[20px] lg:hidden"
        >
          <div class="hidden min-h-0 min-w-0 w-60 shrink-0 lg:block" aria-hidden="true" />
          <div class="min-w-0 w-full max-w-[598px] flex-[0_1_598px]">
            <div id="app-view-header-slot-mobile" class="min-h-0" />
          </div>
          <div
            v-if="!hideRightAside"
            class="hidden min-h-0 min-w-0 w-[350px] shrink-0 lg:block"
            aria-hidden="true"
          />
        </div>
      </div>
    </header>

    <div
      class="relative mx-auto flex w-full min-h-0 flex-1"
      :class="[shellClass, shellPaddingClass]"
    >
      <div
        v-if="authed && !usesGuestSimpleLayout && !isAdminShell && mobileNavOpen"
        class="fixed inset-x-0 bottom-0 top-[var(--app-header-offset,56px)] z-30 bg-black/40 lg:hidden"
        aria-hidden="true"
        @click="closeMobileNav"
      />
      <aside
        v-if="authed && !usesGuestSimpleLayout && !isAdminShell"
        id="app-sidebar"
        class="flex min-h-0 w-60 max-w-[min(100vw-2rem,16rem)] shrink-0 flex-col self-stretch bg-white px-3 py-6 max-lg:fixed max-lg:bottom-0 max-lg:left-0 max-lg:top-[var(--app-header-offset,56px)] max-lg:z-40 max-lg:overflow-y-auto max-lg:transition-transform max-lg:duration-200 max-lg:ease-out"
        :class="[
          mobileNavOpen ? 'max-lg:translate-x-0 max-lg:shadow-xl max-lg:ring-1 max-lg:ring-black/5' : 'max-lg:-translate-x-full',
          useViewportScroll
            ? 'lg:sticky lg:self-start lg:overflow-visible lg:translate-x-0'
            : 'lg:relative lg:inset-auto lg:z-auto lg:h-auto lg:max-h-none lg:overflow-visible lg:translate-x-0',
        ]"
        :style="useViewportScroll ? { top: appHeaderOffset, height: `calc(100dvh - ${appHeaderOffset})` } : undefined"
        :aria-label="$t('app.menu.main')"
      >
        <nav class="flex min-h-0 flex-1 flex-col gap-1.5 overflow-y-auto px-0.5" @click="onAsideNavClick">
          <RouterLink
            to="/feed"
            class="flex items-center gap-3 rounded-full px-3 py-2.5 text-sm font-medium text-neutral-700 transition-colors hover:bg-lime-500 hover:text-white"
            active-class="!rounded-full !bg-lime-600 !text-white"
          >
            <Icon name="home" class="h-5 w-5 shrink-0" />
            <span>{{ $t("app.nav.home") }}</span>
          </RouterLink>
          <RouterLink
            to="/search"
            class="flex items-center gap-3 rounded-full px-3 py-2.5 text-sm font-medium text-neutral-700 transition-colors hover:bg-lime-500 hover:text-white"
            active-class="!rounded-full !bg-lime-600 !text-white"
          >
            <Icon name="search" class="h-5 w-5 shrink-0" />
            <span>{{ $t("app.nav.topics") }}</span>
          </RouterLink>
          <RouterLink
            to="/messages"
            class="flex items-center gap-3 rounded-full px-3 py-2.5 text-sm font-medium text-neutral-700 transition-colors hover:bg-lime-500 hover:text-white"
            :class="route.path === '/messages' || route.path.startsWith('/messages/') ? '!rounded-full !bg-lime-600 !text-white' : ''"
          >
            <span class="flex min-w-0 flex-1 items-center gap-3">
              <Icon name="message" class="h-5 w-5 shrink-0" />
              <span class="truncate">{{ $t("app.nav.messages") }}</span>
            </span>
            <span
              v-if="unreadDMCount > 0"
              class="inline-flex h-5 min-w-[1.25rem] shrink-0 items-center justify-center whitespace-nowrap rounded-full bg-red-500 px-1.5 text-[11px] font-bold leading-none text-white ring-2 ring-white"
            >
              {{ unreadDMCount > 99 ? "99+" : unreadDMCount }}
            </span>
          </RouterLink>
          <RouterLink
            to="/notifications"
            class="flex items-center gap-3 rounded-full px-3 py-2.5 text-sm font-medium text-neutral-700 transition-colors hover:bg-lime-500 hover:text-white"
            active-class="!rounded-full !bg-lime-600 !text-white"
          >
            <span class="flex min-w-0 flex-1 items-center gap-3">
              <Icon name="bell" class="h-5 w-5 shrink-0" />
              <span class="truncate">{{ $t("app.nav.notifications") }}</span>
            </span>
            <span
              v-if="unreadNotificationCount > 0"
              class="inline-flex h-5 min-w-[1.25rem] shrink-0 items-center justify-center whitespace-nowrap rounded-full bg-red-500 px-1.5 text-[11px] font-bold leading-none text-white ring-2 ring-white"
            >
              {{ unreadNotificationCount > 99 ? "99+" : unreadNotificationCount }}
            </span>
          </RouterLink>
          <RouterLink
            to="/bookmarks"
            class="flex items-center gap-3 rounded-full px-3 py-2.5 text-sm font-medium text-neutral-700 transition-colors hover:bg-lime-500 hover:text-white"
            active-class="!rounded-full !bg-lime-600 !text-white"
          >
            <Icon name="bookmark" class="h-5 w-5 shrink-0" />
            <span>{{ $t("app.nav.bookmarks") }}</span>
          </RouterLink>
          <RouterLink
            to="/settings"
            class="flex items-center gap-3 rounded-full px-3 py-2.5 text-sm font-medium text-neutral-700 transition-colors hover:bg-lime-500 hover:text-white"
            active-class="!rounded-full !bg-lime-600 !text-white"
          >
            <Icon name="settings" class="h-5 w-5 shrink-0" />
            <span>{{ $t("app.nav.settings") }}</span>
          </RouterLink>
          <RouterLink
            v-if="me?.handle"
            :to="`/@${me.handle}`"
            class="flex items-center gap-3 rounded-full px-3 py-2.5 text-sm font-medium text-neutral-700 transition-colors hover:bg-lime-500 hover:text-white"
            active-class="!rounded-full !bg-lime-600 !text-white"
          >
            <Icon name="user" class="h-5 w-5 shrink-0" />
            <span>{{ $t("app.nav.profile") }}</span>
          </RouterLink>
        </nav>

        <div ref="profileMenuRoot" class="relative mt-4 shrink-0 border-t border-neutral-200 pt-4">
          <button
            id="profile-menu-button"
            type="button"
            class="group flex w-full items-center gap-3 rounded-full px-2 py-2 text-left transition-colors hover:bg-lime-500 focus:outline-none focus:ring-2 focus:ring-lime-500 focus:ring-offset-2"
            :title="me?.email ?? $t('app.nav.account')"
            :aria-label="$t('app.menu.account')"
            :aria-expanded="profileMenuOpen"
            aria-haspopup="true"
            aria-controls="profile-menu"
            @click.stop="toggleProfileMenu"
          >
            <span
              class="relative flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-full text-sm font-semibold ring-2 ring-lime-200"
              :class="
                me?.avatar_url && !avatarImgFailed
                  ? 'bg-neutral-200 text-white'
                  : 'bg-lime-500 text-white ring-lime-300'
              "
            >
              <img
                v-if="me?.avatar_url && !avatarImgFailed"
                :src="me.avatar_url"
                alt=""
                referrerpolicy="no-referrer"
                class="h-full w-full object-cover"
                @error="avatarImgFailed = true"
              />
              <span v-else-if="me?.email">{{ avatarInitials(me.email) }}</span>
              <span v-else class="text-xs">···</span>
            </span>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-1.5">
                <p class="truncate text-sm font-semibold text-neutral-900 group-hover:text-white">
                  {{ me?.email ? me.display_name || displayNameFromEmail(me.email) : $t("app.loading") }}
                </p>
                <UserBadges v-if="me?.email" :badges="me?.badges" size="xs" />
              </div>
              <p class="truncate text-xs text-neutral-500 group-hover:text-lime-100">
                {{
                  me?.handle
                    ? currentDomain()
                      ? `@${me.handle}@${currentDomain()}`
                      : `@${me.handle}`
                    : ""
                }}
              </p>
            </div>
          </button>
          <div
            v-show="profileMenuOpen"
            id="profile-menu"
            role="menu"
            aria-labelledby="profile-menu-button"
            class="absolute bottom-full left-0 right-0 z-50 mb-1.5 min-w-[11rem] overflow-hidden rounded-lg border border-neutral-200 bg-white py-1 shadow-lg ring-1 ring-black/5"
          >
            <RouterLink
              to="/settings"
              role="menuitem"
              class="block px-4 py-2.5 text-sm text-neutral-800 hover:bg-lime-50"
              @click="closeProfileMenu"
            >
              {{ $t("app.nav.settings") }}
            </RouterLink>
            <RouterLink
              v-if="me?.is_site_admin"
              to="/admin"
              role="menuitem"
              class="block border-t border-neutral-200 px-4 py-2.5 text-sm text-neutral-800 hover:bg-lime-50"
              @click="closeProfileMenu"
            >
              {{ $t("app.nav.adminControlPanel") }}
            </RouterLink>
            <button
              type="button"
              role="menuitem"
              class="w-full border-t border-neutral-200 px-4 py-2.5 text-left text-sm text-neutral-800 hover:bg-lime-50"
              @click="logout"
            >
              {{ $t("app.nav.logout") }}
            </button>
          </div>
        </div>
      </aside>
      <main
        class="min-w-0"
        :class="mainClass"
      >
        <RouterView />
      </main>
      <aside
        v-if="authed && !usesGuestSimpleLayout && !isAdminShell && !hideRightAside"
        class="hidden min-h-0 w-[350px] shrink-0 flex-col gap-6 overflow-y-auto bg-white px-3 py-6 lg:flex"
        :class="useViewportScroll ? 'lg:sticky lg:self-start' : ''"
        :style="useViewportScroll ? { top: appHeaderOffset, maxHeight: `calc(100dvh - ${appHeaderOffset})` } : undefined"
        :aria-label="$t('app.menu.announcementsAndPolicies')"
      >
        <section>
          <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">{{ $t("app.announcements.heading") }}</h2>
          <ul v-if="operatorAnnouncements.length" class="mt-2 space-y-3">
            <li
              v-for="a in operatorAnnouncements"
              :key="a.id"
              class="rounded-xl border border-neutral-200 bg-neutral-50/90 p-3 text-sm shadow-sm"
            >
              <p class="font-semibold text-neutral-900">{{ a.title }}</p>
              <p class="mt-1.5 leading-relaxed text-neutral-600">{{ a.body }}</p>
              <p class="mt-2 text-[11px] text-neutral-400">{{ a.date }}</p>
            </li>
          </ul>
          <p v-else class="mt-2 text-sm text-neutral-500">{{ $t("app.announcements.empty") }}</p>
        </section>
        <nav class="border-t border-neutral-200 pt-4 text-sm" :aria-label="$t('app.menu.policyLinks')">
          <p class="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-500">{{ $t("app.links.heading") }}</p>
          <a
            v-if="legalLink('terms').external"
            :href="legalLink('terms').href"
            target="_blank"
            rel="noopener noreferrer"
            class="block rounded-lg px-2 py-2 text-neutral-700 hover:bg-lime-50 hover:text-lime-900"
          >
            {{ $t("app.links.terms") }}
          </a>
          <RouterLink
            v-else
            :to="legalLink('terms').href"
            class="block rounded-lg px-2 py-2 text-neutral-700 hover:bg-lime-50 hover:text-lime-900"
          >
            {{ $t("app.links.terms") }}
          </RouterLink>
          <a
            v-if="legalLink('privacy').external"
            :href="legalLink('privacy').href"
            target="_blank"
            rel="noopener noreferrer"
            class="block rounded-lg px-2 py-2 text-neutral-700 hover:bg-lime-50 hover:text-lime-900"
          >
            {{ $t("app.links.privacy") }}
          </a>
          <RouterLink
            v-else
            :to="legalLink('privacy').href"
            class="block rounded-lg px-2 py-2 text-neutral-700 hover:bg-lime-50 hover:text-lime-900"
          >
            {{ $t("app.links.privacy") }}
          </RouterLink>
          <a
            v-if="legalLink('nsfw').external"
            :href="legalLink('nsfw').href"
            target="_blank"
            rel="noopener noreferrer"
            class="block rounded-lg px-2 py-2 text-neutral-700 hover:bg-lime-50 hover:text-lime-900"
          >
            {{ $t("app.links.nsfw") }}
          </a>
          <RouterLink
            v-else
            :to="legalLink('nsfw').href"
            class="block rounded-lg px-2 py-2 text-neutral-700 hover:bg-lime-50 hover:text-lime-900"
          >
            {{ $t("app.links.nsfw") }}
          </RouterLink>
          <RouterLink
            to="/federation/guidelines"
            class="block rounded-lg px-2 py-2 text-neutral-700 hover:bg-lime-50 hover:text-lime-900"
          >
            {{ $t("app.links.federation") }}
          </RouterLink>
          <RouterLink
            to="/legal/api-guidelines"
            class="block rounded-lg px-2 py-2 text-neutral-700 hover:bg-lime-50 hover:text-lime-900"
          >
            {{ $t("app.links.apiReference") }}
          </RouterLink>
        </nav>
      </aside>
    </div>
    <nav
      v-if="authed && !usesGuestSimpleLayout && !isAdminShell && !hideMobileChrome"
      class="fixed inset-x-0 bottom-0 z-20 border-t border-neutral-200 bg-white/95 pl-[env(safe-area-inset-left,0px)] pr-[env(safe-area-inset-right,0px)] pb-[env(safe-area-inset-bottom,0px)] backdrop-blur supports-[backdrop-filter]:bg-white/90 lg:hidden"
      :aria-label="$t('app.menu.mobileFooter')"
    >
      <div class="grid grid-cols-4">
        <RouterLink
          v-for="item in mainFooterNavItems"
          :key="item.to"
          :to="item.to"
          class="relative flex min-h-[4.25rem] items-center justify-center px-2 py-2 transition-colors"
          :class="isFooterNavActive(item.to) ? 'text-lime-700' : 'text-neutral-500 hover:text-neutral-800'"
          :aria-label="item.label"
        >
          <Icon :name="item.icon" class="h-6 w-6" />
          <span class="sr-only">{{ item.label }}</span>
          <span
            v-if="footerNavBadge(item.to) > 0"
            class="absolute left-1/2 top-2 ml-2 inline-flex h-5 min-w-[1.25rem] items-center justify-center whitespace-nowrap rounded-full bg-red-500 px-1.5 text-[11px] font-bold leading-none text-white ring-2 ring-white"
          >
            {{ footerNavBadge(item.to) > 99 ? "99+" : footerNavBadge(item.to) }}
          </span>
        </RouterLink>
      </div>
    </nav>
  </div>
</template>
