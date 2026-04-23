<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import UserBadges from "../components/UserBadges.vue";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";

type AdminBadgeUser = {
  id: string;
  handle: string;
  display_name: string;
  badges?: string[];
  visible_badges?: string[];
};

const { t } = useI18n();
const router = useRouter();

const loading = ref(true);
const isAdmin = ref(false);
const err = ref("");
const notice = ref("");
const lookupHandle = ref("");
const lookupBusy = ref(false);
const saveBusy = ref(false);
const availableBadges = ref<string[]>([]);
const user = ref<AdminBadgeUser | null>(null);
const selectedBadges = ref<string[]>([]);

const previewBadges = computed(() => {
  const base = Array.isArray(user.value?.visible_badges)
    ? user.value!.visible_badges.filter((badge) => badge !== "verified")
    : [];
  return [...base, ...selectedBadges.value];
});

const normalizedLookupHandle = computed(() => lookupHandle.value.trim().replace(/^@/, ""));

async function loadMe() {
  const token = getAccessToken();
  if (!token) {
    await router.replace("/login");
    return;
  }
  const me = await api<{ is_site_admin?: boolean }>("/api/v1/me", { method: "GET", token });
  isAdmin.value = !!me.is_site_admin;
  if (!isAdmin.value) {
    err.value = t("views.adminUserBadges.errors.adminOnly");
  }
}

async function lookupUser() {
  const token = getAccessToken();
  if (!token || !isAdmin.value) return;
  const handle = normalizedLookupHandle.value;
  if (!handle) {
    err.value = t("views.adminUserBadges.errors.handleRequired");
    return;
  }
  lookupBusy.value = true;
  err.value = "";
  notice.value = "";
  user.value = null;
  try {
    const res = await api<{ available_badges?: string[]; user?: AdminBadgeUser }>(
      `/api/v1/admin/users/by-handle/${encodeURIComponent(handle)}/badges`,
      { method: "GET", token },
    );
    availableBadges.value = Array.isArray(res.available_badges) ? res.available_badges.map((badge) => String(badge)) : [];
    user.value = res.user ?? null;
    selectedBadges.value = Array.isArray(res.user?.badges) ? res.user.badges.map((badge) => String(badge)) : [];
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminUserBadges.errors.lookupFailed");
  } finally {
    lookupBusy.value = false;
  }
}

async function saveBadges() {
  const token = getAccessToken();
  if (!token || !user.value) return;
  saveBusy.value = true;
  err.value = "";
  notice.value = "";
  try {
    const res = await api<{ user?: AdminBadgeUser }>(
      `/api/v1/admin/users/by-handle/${encodeURIComponent(user.value.handle)}/badges`,
      {
        method: "PUT",
        token,
        json: { badges: selectedBadges.value },
      },
    );
    user.value = res.user ?? user.value;
    selectedBadges.value = Array.isArray(res.user?.badges) ? res.user.badges.map((badge) => String(badge)) : [];
    notice.value = t("views.adminUserBadges.saved");
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminUserBadges.errors.saveFailed");
  } finally {
    saveBusy.value = false;
  }
}

onMounted(async () => {
  loading.value = true;
  err.value = "";
  try {
    await loadMe();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminUserBadges.errors.loadFailed");
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div class="mx-auto max-w-3xl px-4 py-8">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-semibold text-neutral-900">{{ $t("views.adminUserBadges.title") }}</h1>
        <p class="mt-2 text-sm text-neutral-600">{{ $t("views.adminUserBadges.description") }}</p>
      </div>
    </div>

    <p v-if="err" class="mt-4 rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">{{ err }}</p>
    <p v-if="notice" class="mt-4 rounded border border-lime-200 bg-lime-50 px-3 py-2 text-sm text-lime-800">{{ notice }}</p>
    <p v-if="loading" class="mt-6 text-sm text-neutral-500">{{ $t("views.adminUserBadges.loading") }}</p>

    <template v-else-if="isAdmin">
      <section class="mt-8 rounded-2xl border border-neutral-200 bg-white p-5 shadow-sm">
        <label class="block text-sm font-medium text-neutral-800">
          {{ $t("views.adminUserBadges.handleLabel") }}
        </label>
        <div class="mt-2 flex flex-col gap-3 sm:flex-row">
          <input
            v-model="lookupHandle"
            type="text"
            autocomplete="off"
            :placeholder="$t('views.adminUserBadges.handlePlaceholder')"
            class="min-w-0 flex-1 rounded-xl border border-neutral-200 px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
            @keydown.enter.prevent="lookupUser"
          />
          <button
            type="button"
            class="rounded-xl bg-lime-600 px-4 py-3 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
            :disabled="lookupBusy"
            @click="lookupUser"
          >
            {{ lookupBusy ? $t("views.adminUserBadges.lookupBusy") : $t("views.adminUserBadges.lookup") }}
          </button>
        </div>
      </section>

      <section v-if="user" class="mt-6 rounded-2xl border border-neutral-200 bg-white p-5 shadow-sm">
        <div class="flex flex-wrap items-center gap-2">
          <h2 class="text-lg font-semibold text-neutral-900">{{ user.display_name }}</h2>
          <UserBadges :badges="previewBadges" />
        </div>
        <p class="mt-1 text-sm text-neutral-500">@{{ user.handle }}</p>

        <div class="mt-5 space-y-3">
          <label
            v-for="badge in availableBadges"
            :key="badge"
            class="flex items-center gap-3 rounded-xl border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm text-neutral-800"
          >
            <input
              v-model="selectedBadges"
              type="checkbox"
              :value="badge"
              class="h-4 w-4 rounded border-neutral-300 text-lime-600 focus:ring-lime-500"
            />
            <UserBadges :badges="[badge]" />
          </label>
        </div>

        <div class="mt-5 flex justify-end">
          <button
            type="button"
            class="rounded-xl bg-neutral-900 px-4 py-2.5 text-sm font-semibold text-white hover:bg-neutral-800 disabled:opacity-50"
            :disabled="saveBusy"
            @click="saveBadges"
          >
            {{ saveBusy ? $t("views.adminUserBadges.saveBusy") : $t("views.adminUserBadges.save") }}
          </button>
        </div>
      </section>
    </template>
  </div>
</template>
