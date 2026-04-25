<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import UserBadges from "../components/UserBadges.vue";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";
import { formatDateTime } from "../i18n";

type AdminUser = {
  id: string;
  email: string;
  handle: string;
  display_name: string;
  badges: string[];
  is_site_admin: boolean;
  suspended_at?: string;
  created_at: string;
};

type AdminBadgeUser = {
  id: string;
  handle: string;
  display_name: string;
  badges?: string[];
  visible_badges?: string[];
};

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const loading = ref(true);
const err = ref("");
const query = ref("");
const status = ref("all");
const users = ref<AdminUser[]>([]);
const total = ref(0);
const busyUserID = ref("");
const badgeEditorUserID = ref("");
const badgeBusyUserID = ref("");
const availableBadges = ref<string[]>([]);
const selectedBadgesByUserID = ref<Record<string, string[]>>({});
const badgeNotices = ref<Record<string, string>>({});

function formatDate(iso: string): string {
  return formatDateTime(iso, { dateStyle: "medium", timeStyle: "short" }) || iso;
}

async function loadUsers() {
  const token = getAccessToken();
  if (!token) return;
  loading.value = true;
  err.value = "";
  try {
    const params = new URLSearchParams();
    if (query.value.trim()) params.set("query", query.value.trim());
    if (status.value !== "all") params.set("status", status.value);
    params.set("limit", "80");
    const res = await api<{ items: AdminUser[]; total: number }>(`/api/v1/admin/users?${params.toString()}`, {
      method: "GET",
      token,
    });
    users.value = res.items ?? [];
    total.value = Number(res.total) || users.value.length;
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminUsers.errors.loadFailed");
  } finally {
    loading.value = false;
  }
}

async function setSuspended(user: AdminUser, suspended: boolean) {
  const token = getAccessToken();
  if (!token) return;
  busyUserID.value = user.id;
  err.value = "";
  try {
    const res = await api<{ user: AdminUser }>(`/api/v1/admin/users/${encodeURIComponent(user.id)}/suspension`, {
      method: "PATCH",
      token,
      json: { suspended },
    });
    users.value = users.value.map((row) => (row.id === user.id ? res.user : row));
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminUsers.errors.suspensionFailed");
  } finally {
    busyUserID.value = "";
  }
}

function selectableBadgeSet(): Set<string> {
  return new Set(availableBadges.value);
}

function selectedBadgesFor(user: AdminUser): string[] {
  return selectedBadgesByUserID.value[user.id] ?? [];
}

function previewBadgesFor(user: AdminUser): string[] {
  if (badgeEditorUserID.value !== user.id) return user.badges ?? [];
  const selectable = selectableBadgeSet();
  const automaticBadges = (user.badges ?? []).filter((badge) => !selectable.has(badge));
  return [...automaticBadges, ...selectedBadgesFor(user)];
}

function updateSelectedBadges(user: AdminUser, badges: string[]) {
  selectedBadgesByUserID.value = {
    ...selectedBadgesByUserID.value,
    [user.id]: badges,
  };
}

async function openBadgeEditor(user: AdminUser) {
  if (badgeEditorUserID.value === user.id) {
    badgeEditorUserID.value = "";
    return;
  }
  const token = getAccessToken();
  if (!token) return;
  badgeEditorUserID.value = user.id;
  badgeBusyUserID.value = user.id;
  badgeNotices.value = { ...badgeNotices.value, [user.id]: "" };
  err.value = "";
  try {
    const res = await api<{ available_badges?: string[]; user?: AdminBadgeUser }>(
      `/api/v1/admin/users/by-handle/${encodeURIComponent(user.handle)}/badges`,
      { method: "GET", token },
    );
    availableBadges.value = Array.isArray(res.available_badges) ? res.available_badges.map((badge) => String(badge)) : [];
    updateSelectedBadges(user, Array.isArray(res.user?.badges) ? res.user.badges.map((badge) => String(badge)) : []);
    if (Array.isArray(res.user?.visible_badges)) {
      users.value = users.value.map((row) => (row.id === user.id ? { ...row, badges: res.user!.visible_badges!.map((badge) => String(badge)) } : row));
    }
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminUsers.errors.badgesLoadFailed");
    badgeEditorUserID.value = "";
  } finally {
    badgeBusyUserID.value = "";
  }
}

async function saveBadges(user: AdminUser) {
  const token = getAccessToken();
  if (!token) return;
  badgeBusyUserID.value = user.id;
  badgeNotices.value = { ...badgeNotices.value, [user.id]: "" };
  err.value = "";
  try {
    const res = await api<{ user?: AdminBadgeUser }>(
      `/api/v1/admin/users/by-handle/${encodeURIComponent(user.handle)}/badges`,
      {
        method: "PUT",
        token,
        json: { badges: selectedBadgesFor(user) },
      },
    );
    updateSelectedBadges(user, Array.isArray(res.user?.badges) ? res.user.badges.map((badge) => String(badge)) : []);
    users.value = users.value.map((row) =>
      row.id === user.id && Array.isArray(res.user?.visible_badges)
        ? { ...row, badges: res.user.visible_badges.map((badge) => String(badge)) }
        : row,
    );
    badgeNotices.value = { ...badgeNotices.value, [user.id]: t("views.adminUsers.badgesSaved") };
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminUsers.errors.badgesSaveFailed");
  } finally {
    badgeBusyUserID.value = "";
  }
}

function applyFilters() {
  void router.replace({ path: "/admin/users", query: { ...(query.value.trim() ? { query: query.value.trim() } : {}), ...(status.value !== "all" ? { status: status.value } : {}) } });
  void loadUsers();
}

watch(
  () => route.query,
  () => {
    query.value = typeof route.query.query === "string" ? route.query.query : "";
    status.value = typeof route.query.status === "string" ? route.query.status : "all";
  },
  { immediate: true },
);

onMounted(() => {
  void loadUsers();
});
</script>

<template>
  <div class="mx-auto max-w-6xl px-4 py-8">
    <header>
      <p class="text-xs font-semibold uppercase tracking-[0.18em] text-lime-700">{{ $t("views.adminShell.eyebrow") }}</p>
      <h1 class="mt-2 text-2xl font-bold text-neutral-900">{{ $t("views.adminUsers.title") }}</h1>
      <p class="mt-2 text-sm text-neutral-600">{{ $t("views.adminUsers.description") }}</p>
    </header>

    <section class="mt-6 rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
      <div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_12rem_auto]">
        <label class="block text-sm">
          <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminUsers.searchLabel") }}</span>
          <input
            v-model="query"
            type="search"
            class="w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
            :placeholder="$t('views.adminUsers.searchPlaceholder')"
            @keydown.enter.prevent="applyFilters"
          />
        </label>
        <label class="block text-sm">
          <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminUsers.statusLabel") }}</span>
          <select
            v-model="status"
            class="w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
          >
            <option value="all">{{ $t("views.adminUsers.statusAll") }}</option>
            <option value="active">{{ $t("views.adminUsers.statusActive") }}</option>
            <option value="suspended">{{ $t("views.adminUsers.statusSuspended") }}</option>
          </select>
        </label>
        <div class="flex items-end">
          <button
            type="button"
            class="w-full rounded-xl bg-lime-600 px-4 py-3 text-sm font-semibold text-white hover:bg-lime-700"
            @click="applyFilters"
          >
            {{ $t("views.adminUsers.apply") }}
          </button>
        </div>
      </div>
    </section>

    <p v-if="err" class="mt-5 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">{{ err }}</p>
    <p v-if="loading" class="mt-8 text-sm text-neutral-500">{{ $t("app.loading") }}</p>

    <section v-else class="mt-6 rounded-3xl border border-neutral-200 bg-white shadow-sm">
      <div class="flex items-center justify-between gap-3 border-b border-neutral-200 px-5 py-4">
        <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.adminUsers.listHeading") }}</h2>
        <span class="text-sm text-neutral-500">{{ total }}{{ $t("views.adminUsers.countSuffix") }}</span>
      </div>
      <div v-if="!users.length" class="px-5 py-8 text-sm text-neutral-500">{{ $t("views.adminUsers.empty") }}</div>
      <div v-else class="divide-y divide-neutral-200">
        <article v-for="user in users" :key="user.id" class="grid gap-4 px-5 py-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="truncate text-sm font-semibold text-neutral-900">{{ user.display_name || user.email }}</h3>
              <span v-if="user.is_site_admin" class="rounded-full bg-lime-50 px-2 py-0.5 text-xs font-semibold text-lime-800">{{ $t("views.adminUsers.siteAdmin") }}</span>
              <span v-if="user.suspended_at" class="rounded-full bg-red-50 px-2 py-0.5 text-xs font-semibold text-red-700">{{ $t("views.adminUsers.suspended") }}</span>
              <UserBadges :badges="user.badges" size="xs" />
            </div>
            <p class="mt-1 truncate text-sm text-neutral-500">@{{ user.handle }} · {{ user.email }}</p>
            <p class="mt-1 text-xs text-neutral-400">
              {{ $t("views.adminUsers.createdAt") }} {{ formatDate(user.created_at) }}
              <span v-if="user.suspended_at"> · {{ $t("views.adminUsers.suspendedAt") }} {{ formatDate(user.suspended_at) }}</span>
            </p>
          </div>
          <div class="flex flex-wrap gap-2 lg:justify-end">
            <a :href="`/@${user.handle}`" class="rounded-full border border-neutral-200 px-3 py-1.5 text-xs font-semibold text-neutral-700 hover:bg-lime-50">
              {{ $t("views.adminUsers.openProfile") }}
            </a>
            <button
              type="button"
              class="rounded-full border border-lime-200 px-3 py-1.5 text-xs font-semibold text-lime-800 hover:bg-lime-50 disabled:opacity-50"
              :disabled="badgeBusyUserID === user.id"
              @click="openBadgeEditor(user)"
            >
              {{ badgeEditorUserID === user.id ? $t("views.adminUsers.closeBadges") : $t("views.adminUsers.editBadges") }}
            </button>
            <button
              v-if="!user.suspended_at"
              type="button"
              class="rounded-full border border-red-200 px-3 py-1.5 text-xs font-semibold text-red-700 hover:bg-red-50 disabled:opacity-50"
              :disabled="busyUserID === user.id || user.is_site_admin"
              @click="setSuspended(user, true)"
            >
              {{ $t("views.adminUsers.suspend") }}
            </button>
            <button
              v-else
              type="button"
              class="rounded-full border border-lime-200 px-3 py-1.5 text-xs font-semibold text-lime-800 hover:bg-lime-50 disabled:opacity-50"
              :disabled="busyUserID === user.id"
              @click="setSuspended(user, false)"
            >
              {{ $t("views.adminUsers.unsuspend") }}
            </button>
          </div>
          <section v-if="badgeEditorUserID === user.id" class="rounded-2xl border border-lime-100 bg-lime-50/50 p-4 lg:col-span-2">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h4 class="text-sm font-semibold text-neutral-900">{{ $t("views.adminUsers.badgesHeading") }}</h4>
                <p class="mt-1 text-xs text-neutral-600">{{ $t("views.adminUsers.badgesHint") }}</p>
              </div>
              <UserBadges :badges="previewBadgesFor(user)" size="xs" />
            </div>
            <p v-if="badgeNotices[user.id]" class="mt-3 rounded-xl border border-lime-200 bg-white px-3 py-2 text-xs text-lime-800">
              {{ badgeNotices[user.id] }}
            </p>
            <p v-if="badgeBusyUserID === user.id && !availableBadges.length" class="mt-4 text-sm text-neutral-500">{{ $t("app.loading") }}</p>
            <div v-else class="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              <label
                v-for="badge in availableBadges"
                :key="badge"
                class="flex items-center gap-3 rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-800"
              >
                <input
                  :checked="selectedBadgesFor(user).includes(badge)"
                  type="checkbox"
                  class="h-4 w-4 rounded border-neutral-300 text-lime-600 focus:ring-lime-500"
                  @change="
                    updateSelectedBadges(
                      user,
                      ($event.target as HTMLInputElement).checked
                        ? [...selectedBadgesFor(user), badge]
                        : selectedBadgesFor(user).filter((item) => item !== badge),
                    )
                  "
                />
                <UserBadges :badges="[badge]" size="xs" />
              </label>
            </div>
            <div class="mt-4 flex justify-end">
              <button
                type="button"
                class="rounded-xl bg-neutral-900 px-4 py-2.5 text-sm font-semibold text-white hover:bg-neutral-800 disabled:opacity-50"
                :disabled="badgeBusyUserID === user.id"
                @click="saveBadges(user)"
              >
                {{ badgeBusyUserID === user.id ? $t("views.adminUsers.badgesSaving") : $t("views.adminUsers.saveBadges") }}
              </button>
            </div>
          </section>
        </article>
      </div>
    </section>
  </div>
</template>
