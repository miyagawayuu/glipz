<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import { listCommunities, type Community } from "../lib/communities";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const communities = ref<Community[]>([]);
const busy = ref(false);
const err = ref("");
const signedIn = ref(false);
const searchQuery = ref("");
let searchTimer: ReturnType<typeof setTimeout> | null = null;

const activeQuery = computed(() => {
  const raw = route.query.q;
  if (typeof raw === "string") return raw.trim();
  if (Array.isArray(raw)) return String(raw[0] ?? "").trim();
  return "";
});
const isSearching = computed(() => activeQuery.value.length > 0);

async function load() {
  busy.value = true;
  err.value = "";
  signedIn.value = Boolean(getAccessToken());
  try {
    communities.value = await listCommunities(activeQuery.value);
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : "load_failed";
  } finally {
    busy.value = false;
  }
}

function statusLabel(status: Community["viewer_status"]): string {
  if (status === "approved") return t("views.communities.status.approved");
  if (status === "pending") return t("views.communities.status.pending");
  if (status === "rejected") return t("views.communities.status.rejected");
  return t("views.communities.status.none");
}

function syncSearchFromRoute() {
  searchQuery.value = activeQuery.value;
}

function replaceSearchQuery(q: string) {
  const query = { ...route.query };
  const trimmed = q.trim();
  if (trimmed) {
    query.q = trimmed;
  } else {
    delete query.q;
  }
  void router.replace({ path: route.path, query });
}

function submitSearch() {
  if (searchTimer) {
    clearTimeout(searchTimer);
    searchTimer = null;
  }
  replaceSearchQuery(searchQuery.value);
}

function onSearchInput() {
  if (searchTimer) clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    searchTimer = null;
    replaceSearchQuery(searchQuery.value);
  }, 300);
}

watch(
  () => route.query.q,
  () => {
    syncSearchFromRoute();
    void load();
  },
);

onMounted(() => {
  syncSearchFromRoute();
  void load();
});

onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer);
});
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="flex items-center justify-between gap-3 border-b border-neutral-200 px-4 py-3">
      <div class="min-w-0">
        <h1 class="truncate text-xl font-bold text-neutral-900">{{ $t("views.communities.title") }}</h1>
        <p class="truncate text-xs text-neutral-500">{{ $t("views.communities.description") }}</p>
      </div>
      <RouterLink
        v-if="signedIn"
        to="/communities/new"
        class="shrink-0 rounded-full bg-lime-500 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-600"
      >
        {{ $t("views.communities.create") }}
      </RouterLink>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="flex items-center justify-between gap-3 border-b border-neutral-200 px-4 py-3">
      <h1 class="min-w-0 truncate text-lg font-bold text-neutral-900">{{ $t("views.communities.title") }}</h1>
      <RouterLink
        v-if="signedIn"
        to="/communities/new"
        class="shrink-0 rounded-full bg-lime-500 px-3 py-1.5 text-xs font-semibold text-white hover:bg-lime-600"
      >
        {{ $t("views.communities.create") }}
      </RouterLink>
    </div>
  </Teleport>

  <div class="border-b border-neutral-200 px-4 py-4">
    <form class="flex gap-2" role="search" @submit.prevent="submitSearch">
      <label class="sr-only" for="community-search">{{ $t("views.communities.searchLabel") }}</label>
      <input
        id="community-search"
        v-model="searchQuery"
        type="search"
        autocomplete="off"
        class="min-w-0 flex-1 rounded-full border border-neutral-200 bg-white px-4 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 placeholder:text-neutral-400 focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
        :placeholder="$t('views.communities.searchPlaceholder')"
        @input="onSearchInput"
      />
      <button
        type="submit"
        class="shrink-0 rounded-full bg-neutral-900 px-4 py-2 text-sm font-semibold text-white hover:bg-neutral-800"
      >
        {{ $t("views.communities.searchSubmit") }}
      </button>
    </form>
  </div>

  <div v-if="err" class="m-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
    {{ $t("views.communities.loadFailed") }}
  </div>
  <div v-else-if="busy" class="p-6 text-sm text-neutral-500">{{ $t("app.loading") }}</div>
  <div v-else-if="communities.length === 0" class="p-6 text-sm text-neutral-500">
    {{ isSearching ? $t("views.communities.emptySearch") : $t("views.communities.empty") }}
  </div>
  <div v-else class="divide-y divide-neutral-200">
    <RouterLink
      v-for="community in communities"
      :key="community.id"
      :to="`/communities/${community.id}`"
      class="block px-4 py-4 transition-colors hover:bg-neutral-50"
    >
      <div class="flex items-start justify-between gap-3">
        <div class="flex min-w-0 gap-3">
          <div class="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-2xl bg-lime-100 text-base font-bold text-lime-800">
            <img v-if="community.icon_url" :src="community.icon_url" alt="" class="h-full w-full object-cover" />
            <span v-else>{{ community.name.slice(0, 1) }}</span>
          </div>
          <div class="min-w-0">
          <h2 class="truncate text-lg font-semibold text-neutral-900">{{ community.name }}</h2>
          <p class="mt-0.5 truncate font-mono text-xs text-neutral-500">{{ community.id }}</p>
          <p v-if="community.description" class="mt-2 line-clamp-2 text-sm text-neutral-700">{{ community.description }}</p>
          </div>
        </div>
        <div class="shrink-0 text-right text-xs text-neutral-500">
          <div>{{ community.approved_member_count }} {{ $t("views.communities.members") }}</div>
          <div v-if="signedIn" class="mt-1 rounded-full bg-neutral-100 px-2 py-0.5 text-neutral-700">
            {{ statusLabel(community.viewer_status) }}
          </div>
        </div>
      </div>
    </RouterLink>
  </div>
</template>
