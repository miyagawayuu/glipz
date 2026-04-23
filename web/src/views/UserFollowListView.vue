<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRoute, useRouter } from "vue-router";
import Icon from "../components/Icon.vue";
import UserBadges from "../components/UserBadges.vue";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";
import { avatarInitials, fullHandleAt } from "../lib/feedDisplay";

type FollowUserRow = {
  handle: string;
  display_name: string;
  badges?: string[];
  bio?: string;
  avatar_url: string | null;
  followed_by_me?: boolean;
  follows_you?: boolean;
  is_remote?: boolean;
  remote_actor_id?: string;
};

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

const handleParam = computed(() =>
  String(route.params.handle ?? "")
    .replace(/^@/, "")
    .trim(),
);

type Kind = "followers" | "following";
const kind = computed<Kind>(() => (route.path.endsWith("/following") ? "following" : "followers"));

const title = computed(() =>
  kind.value === "following" ? t("views.userProfile.followingLabel") : t("views.userProfile.followersLabel"),
);

const items = ref<FollowUserRow[]>([]);
const err = ref("");
const busy = ref(false);
const nextOffset = ref<number | null>(null);

const viewerAuthed = computed(() => Boolean(getAccessToken()));

function rowBio(it: FollowUserRow): string {
  return String(it.bio ?? "").trim();
}

async function loadPage(reset: boolean) {
  const h = handleParam.value;
  if (!h) {
    err.value = t("views.userProfile.errors.userNotFound");
    return;
  }
  if (busy.value) return;
  busy.value = true;
  err.value = "";
  try {
    const token = getAccessToken();
    const offset = reset ? 0 : (nextOffset.value ?? 0);
    const enc = encodeURIComponent(h);
    const path = `/api/v1/users/by-handle/${enc}/${kind.value}?limit=50&offset=${offset}`;
    const res = await api<{ items: FollowUserRow[]; next_offset?: number }>(path, {
      method: "GET",
      ...(token ? { token } : {}),
    });
    if (reset) {
      items.value = res.items ?? [];
    } else {
      items.value = [...items.value, ...(res.items ?? [])];
    }
    nextOffset.value = typeof res.next_offset === "number" ? res.next_offset : null;
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.userProfile.errors.loadFailed");
  } finally {
    busy.value = false;
  }
}

async function toggleFollowRow(it: FollowUserRow) {
  if (it.is_remote) return;
  const token = getAccessToken();
  if (!token || !it.handle) return;
  const h = encodeURIComponent(it.handle);
  try {
    const res = await api<{ following: boolean }>(`/api/v1/users/by-handle/${h}/follow`, { method: "POST", token });
    items.value = items.value.map((x) => (x.handle === it.handle ? { ...x, followed_by_me: res.following } : x));
  } catch {
    // ignore
  }
}

function openProfile(it: FollowUserRow) {
  if (it.is_remote && it.remote_actor_id) {
    router.push({ path: "/remote/profile", query: { actor: it.remote_actor_id } });
    return;
  }
  if (!it.handle) return;
  router.push(`/@${encodeURIComponent(it.handle)}`);
}

onMounted(() => {
  void loadPage(true);
});

watch(
  () => route.fullPath,
  () => void loadPage(true),
);
</script>

<template>
  <div class="min-h-0 h-full w-full min-w-0 text-neutral-900">
    <header
      class="sticky top-0 z-10 flex h-14 items-center gap-3 border-b border-neutral-200 bg-white/90 px-4 backdrop-blur supports-[backdrop-filter]:bg-white/70"
    >
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="$t('views.userProfile.backAria')"
        @click="router.back()"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <div class="min-w-0">
        <h1 class="truncate text-lg font-bold leading-tight text-neutral-900">{{ title }}</h1>
        <RouterLink :to="`/@${encodeURIComponent(handleParam)}`" class="truncate text-sm text-neutral-500 hover:underline">
          {{ handleParam ? fullHandleAt(handleParam) : "" }}
        </RouterLink>
      </div>
    </header>

    <p v-if="err" class="border-b border-neutral-200 px-4 py-3 text-sm text-red-600">{{ err }}</p>

    <div v-if="!items.length && busy" class="border-b border-neutral-200 px-4 py-12 text-center text-sm text-neutral-500">
      {{ $t("app.loading") }}
    </div>

    <div v-else-if="!items.length" class="border-b border-neutral-200 px-4 py-12 text-center text-sm text-neutral-500">
      —
    </div>

    <ul v-else class="divide-y divide-neutral-200 border-b border-neutral-200">
      <li v-for="it in items" :key="it.is_remote ? it.remote_actor_id : it.handle">
        <div class="flex items-start gap-3 px-4 py-3">
          <button type="button" class="flex min-w-0 flex-1 items-start gap-3 text-left" @click="openProfile(it)">
            <div
              class="flex h-11 w-11 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-sm font-bold text-neutral-700"
            >
              <img v-if="it.avatar_url" :src="it.avatar_url" alt="" class="h-full w-full object-cover" />
              <span v-else>{{ avatarInitials(it.display_name || it.handle) }}</span>
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-1.5">
                <p class="truncate text-sm font-semibold text-neutral-900">{{ it.display_name?.trim() || fullHandleAt(it.handle) }}</p>
                <UserBadges :badges="it.badges" size="xs" />
              </div>
              <p class="truncate text-xs text-neutral-500">{{ fullHandleAt(it.handle) }}</p>
              <p
                v-if="rowBio(it)"
                class="mt-1 line-clamp-3 whitespace-pre-line break-words text-xs text-neutral-600"
              >
                {{ rowBio(it) }}
              </p>
            </div>
          </button>

          <button
            v-if="viewerAuthed && !it.is_remote"
            type="button"
            class="shrink-0 rounded-full border px-3 py-1.5 text-xs font-semibold transition-colors disabled:opacity-50"
            :class="
              it.followed_by_me
                ? 'border-neutral-200 bg-white text-neutral-800 hover:bg-neutral-50'
                : 'border-transparent bg-neutral-900 text-white hover:bg-neutral-800'
            "
            :disabled="busy"
            @click="toggleFollowRow(it)"
          >
            {{ it.followed_by_me ? $t("views.userProfile.following") : $t("views.userProfile.follow") }}
          </button>
        </div>
      </li>
    </ul>

    <div v-if="nextOffset !== null" class="px-4 py-4">
      <button
        type="button"
        class="w-full rounded-full border border-neutral-200 bg-white px-4 py-2 text-sm font-semibold text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
        :disabled="busy"
        @click="loadPage(false)"
      >
        {{ busy ? $t("app.loading") : "もっと見る" }}
      </button>
    </div>
  </div>
</template>

