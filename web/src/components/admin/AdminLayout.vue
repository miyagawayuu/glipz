<script setup lang="ts">
import type { Ref } from "vue";
import { computed, inject, onMounted, ref, watch } from "vue";
import { RouterLink, RouterView, useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import Icon from "../Icon.vue";
import logoImg from "../../assets/logo.png";
import { clearTokens, getAccessToken } from "../../auth";
import { api } from "../../lib/api";

type AppMe = {
  email: string;
  handle: string;
  display_name?: string;
  is_site_admin?: boolean;
} | null;

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const appMe = inject<Ref<AppMe> | null>("appMe", null);
const mobileOpen = ref(false);
const loading = ref(true);
const err = ref("");
const isAdmin = ref(false);

const navItems = computed(() => [
  { to: "/admin", exact: true, label: t("views.adminShell.nav.dashboard"), icon: "chart" as const },
  { to: "/admin/users", label: t("views.adminShell.nav.users"), icon: "user" as const },
  { to: "/admin/reports", label: t("views.adminShell.nav.reports"), icon: "warning" as const },
  { to: "/admin/federation", label: t("views.adminShell.nav.federation"), icon: "share" as const },
  { to: "/admin/custom-emojis", label: t("views.adminShell.nav.customEmojis"), icon: "image" as const },
  { to: "/admin/instance-settings", label: t("views.adminShell.nav.instanceSettings"), icon: "settings" as const },
]);

const adminName = computed(() => appMe?.value?.display_name || appMe?.value?.email || t("app.loading"));

function isActive(to: string, exact?: boolean): boolean {
  return exact ? route.path === to : route.path === to || route.path.startsWith(`${to}/`);
}

function closeMobile() {
  mobileOpen.value = false;
}

async function verifyAdmin() {
  const token = getAccessToken();
  if (!token) {
    await router.replace({ path: "/login", query: { next: route.fullPath } });
    return;
  }
  loading.value = true;
  err.value = "";
  try {
    const me = await api<{ is_site_admin?: boolean }>("/api/v1/me", { method: "GET", token });
    isAdmin.value = !!me.is_site_admin;
    if (!isAdmin.value) {
      err.value = t("views.adminShell.adminOnly");
    }
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminShell.loadFailed");
  } finally {
    loading.value = false;
  }
}

function logout() {
  clearTokens();
  void router.push("/login");
}

watch(() => route.fullPath, closeMobile);

onMounted(() => {
  void verifyAdmin();
});
</script>

<template>
  <div class="min-h-screen bg-white text-neutral-900">
    <div class="border-b border-lime-200 bg-white/95 px-4 py-3 backdrop-blur lg:hidden">
      <div class="flex items-center justify-between gap-3">
        <button
          type="button"
          class="inline-flex h-10 w-10 items-center justify-center rounded-full border border-neutral-200 text-neutral-700 hover:bg-neutral-50"
          :aria-label="mobileOpen ? $t('app.menu.close') : $t('app.menu.open')"
          :aria-expanded="mobileOpen"
          aria-controls="admin-sidebar"
          @click="mobileOpen = !mobileOpen"
        >
          <Icon v-if="!mobileOpen" name="menu" class="h-5 w-5" />
          <Icon v-else name="close" class="h-5 w-5" />
        </button>
        <RouterLink to="/admin" class="flex min-w-0 items-center gap-2">
          <img :src="logoImg" alt="Glipz" class="h-8 w-auto" />
          <span class="truncate text-sm font-semibold">{{ $t("views.adminShell.title") }}</span>
        </RouterLink>
        <RouterLink to="/feed" class="text-sm font-medium text-lime-700 hover:underline">
          {{ $t("views.adminShell.backToAppShort") }}
        </RouterLink>
      </div>
    </div>

    <div class="min-h-screen">
      <div
        v-if="mobileOpen"
        class="fixed inset-0 z-30 bg-black/40 lg:hidden"
        aria-hidden="true"
        @click="closeMobile"
      />
      <aside
        id="admin-sidebar"
        class="fixed inset-y-0 left-0 z-40 flex h-[100dvh] w-72 max-w-[85vw] flex-col border-r border-neutral-200 bg-white px-4 py-5 transition-transform lg:z-20 lg:max-w-none lg:translate-x-0"
        :class="mobileOpen ? 'translate-x-0 shadow-xl' : '-translate-x-full lg:translate-x-0'"
        :aria-label="$t('views.adminShell.navLabel')"
      >
        <RouterLink to="/admin" class="hidden items-center gap-2 px-2 py-1 hover:opacity-90 lg:flex">
          <img :src="logoImg" alt="Glipz" class="h-9 w-auto" />
          <span class="text-sm font-bold text-neutral-900">{{ $t("views.adminShell.title") }}</span>
        </RouterLink>

        <nav class="mt-6 flex flex-1 flex-col gap-1.5 overflow-y-auto">
          <RouterLink
            v-for="item in navItems"
            :key="item.to"
            :to="item.to"
            class="flex items-center gap-3 rounded-full px-3 py-2.5 text-sm font-medium transition-colors"
            :class="isActive(item.to, item.exact) ? 'bg-lime-600 text-white' : 'text-neutral-700 hover:bg-lime-50 hover:text-lime-900'"
            @click="closeMobile"
          >
            <Icon :name="item.icon" class="h-5 w-5 shrink-0" />
            <span>{{ item.label }}</span>
          </RouterLink>
        </nav>

        <div class="mt-5 border-t border-neutral-200 pt-4">
          <p class="truncate text-xs text-neutral-500">{{ $t("views.adminShell.signedInAs") }}</p>
          <p class="truncate text-sm font-semibold text-neutral-900">{{ adminName }}</p>
          <div class="mt-3 flex flex-wrap gap-2">
            <RouterLink
              to="/feed"
              class="rounded-full border border-neutral-200 px-3 py-1.5 text-xs font-semibold text-neutral-700 hover:bg-lime-50"
            >
              {{ $t("views.adminShell.backToApp") }}
            </RouterLink>
            <button
              type="button"
              class="rounded-full border border-neutral-200 px-3 py-1.5 text-xs font-semibold text-neutral-700 hover:bg-neutral-50"
              @click="logout"
            >
              {{ $t("app.nav.logout") }}
            </button>
          </div>
        </div>
      </aside>

      <main class="min-w-0 bg-neutral-50/60 lg:ml-72">
        <div v-if="loading" class="mx-auto max-w-6xl px-4 py-10 text-sm text-neutral-500">
          {{ $t("app.loading") }}
        </div>
        <div v-else-if="err" class="mx-auto max-w-3xl px-4 py-10">
          <div class="rounded-2xl border border-red-200 bg-red-50 p-5 text-sm text-red-800">
            {{ err }}
          </div>
        </div>
        <RouterView v-else-if="isAdmin" />
      </main>
    </div>
  </div>
</template>
