<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import PullToRefresh from "../components/PullToRefresh.vue";
import PostTimeline from "../components/PostTimeline.vue";
import UserBadges from "../components/UserBadges.vue";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";
import { mapFeedItem } from "../lib/feedStream";
import { avatarInitials, fullHandleAt, postDetailPath } from "../lib/feedDisplay";
import { buildComposerReplyQuery, composeRoutePath } from "../lib/postComposer";
import { addTimelineReaction, removeTimelineReaction, toggleTimelineBookmark, toggleTimelineRepost } from "../lib/federationActions";
import type { TimelinePost } from "../types/timeline";
import { translate } from "../i18n";

const route = useRoute();
const router = useRouter();

type SearchTab = "latest" | "accounts" | "media";
type AccountSearchResult = {
  handle: string;
  display_name: string;
  badges?: string[];
  bio: string;
  avatar_url: string | null;
};

const { t } = useI18n();
const searchTabs = computed<Array<{ value: SearchTab; label: string }>>(() => [
  { value: "latest", label: t("views.search.tabs.latest") },
  { value: "accounts", label: t("views.search.tabs.accounts") },
  { value: "media", label: t("views.search.tabs.media") },
]);

const items = ref<TimelinePost[]>([]);
const accounts = ref<AccountSearchResult[]>([]);
const err = ref("");
const busy = ref(false);
const myEmail = ref<string | null>(null);
const actionBusy = ref<string | null>(null);

const currentQuery = computed(() => {
  const raw = route.query.q;
  if (typeof raw === "string") return raw.trim();
  if (Array.isArray(raw)) return String(raw[0] ?? "").trim();
  return "";
});
const currentTab = computed<SearchTab>(() => {
  const raw = route.query.tab;
  const value = typeof raw === "string" ? raw : Array.isArray(raw) ? String(raw[0] ?? "") : "";
  return value === "accounts" || value === "media" ? value : "latest";
});
const mediaItems = computed(() =>
  items.value.filter((item) => item.media_type === "image" || item.media_type === "video" || item.media_type === "audio"),
);

async function loadMe() {
  const token = getAccessToken();
  if (!token) {
    myEmail.value = null;
    return;
  }
  try {
    const res = await api<{ email: string }>("/api/v1/me", { method: "GET", token });
    myEmail.value = res.email;
  } catch {
    myEmail.value = null;
  }
}

async function load() {
  const token = getAccessToken();
  if (!token) {
    await router.replace("/login");
    return;
  }
  const q = currentQuery.value;
  err.value = "";
  if (!q) {
    items.value = [];
    accounts.value = [];
    busy.value = false;
    return;
  }
  busy.value = true;
  try {
    const res = await api<{ items: Record<string, unknown>[]; accounts?: AccountSearchResult[] }>(
      "/api/v1/search?" + new URLSearchParams({ q }).toString(),
      {
      method: "GET",
      token,
      },
    );
    items.value = (res.items ?? []).map((x) => mapFeedItem(x as Parameters<typeof mapFeedItem>[0]));
    accounts.value = Array.isArray(res.accounts) ? res.accounts : [];
  } catch (e: unknown) {
    items.value = [];
    accounts.value = [];
    err.value = e instanceof Error ? e.message : t("views.search.failed");
  } finally {
    busy.value = false;
  }
}

async function refreshSearch() {
  await load();
}

function setSearchTab(tab: SearchTab) {
  void router.replace({
    path: "/search",
    query: {
      ...(currentQuery.value ? { q: currentQuery.value } : {}),
      ...(tab !== "latest" ? { tab } : {}),
    },
  });
}

function patchItem(id: string, patch: Partial<TimelinePost>) {
  items.value = items.value.map((x) => (x.id === id ? { ...x, ...patch } : x));
}

function removePost(id: string) {
  items.value = items.value.filter((x) => x.id !== id);
}

function replyTo(it: TimelinePost) {
  void router.push({
    path: composeRoutePath(),
    query: buildComposerReplyQuery(it),
  });
}

function applyReactionPost(updated: TimelinePost) {
  patchItem(updated.id, {
    reactions: updated.reactions,
    like_count: updated.like_count,
    liked_by_me: updated.liked_by_me,
  });
}

async function toggleReaction(it: TimelinePost, emoji: string) {
  const token = getAccessToken();
  if (!token || actionBusy.value === `rx-${it.id}`) return;
  actionBusy.value = `rx-${it.id}`;
  try {
    const active = it.reactions.some((reaction) => reaction.emoji === emoji && reaction.reacted_by_me);
    const updated = active
      ? await removeTimelineReaction(token, it, emoji)
      : await addTimelineReaction(token, it, emoji);
    applyReactionPost(updated);
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.search.reactionFailed");
  } finally {
    actionBusy.value = null;
  }
}

async function toggleBookmark(it: TimelinePost) {
  const token = getAccessToken();
  if (!token || actionBusy.value === `bm-${it.id}`) return;
  actionBusy.value = `bm-${it.id}`;
  try {
    const res = await toggleTimelineBookmark(token, it);
    patchItem(it.id, { bookmarked_by_me: res.bookmarked });
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.search.bookmarkFailed");
  } finally {
    actionBusy.value = null;
  }
}

async function toggleRepost(it: TimelinePost) {
  const token = getAccessToken();
  if (!token) return;
  actionBusy.value = `rp-${it.id}`;
  try {
    const res = await toggleTimelineRepost(token, it);
    patchItem(it.id, { reposted_by_me: res.reposted, repost_count: res.repost_count });
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.search.repostFailed");
  } finally {
    actionBusy.value = null;
  }
}

async function sharePost(it: TimelinePost) {
  const resolved = router.resolve({ path: postDetailPath(it.id) });
  const url = new URL(resolved.href, window.location.origin).href;
  if (navigator.share) {
    try {
      await navigator.share({
        title: translate("views.search.shareFallbackTitle"),
        text: it.caption?.slice(0, 80) ?? translate("views.search.shareFallbackText"),
        url,
      });
      return;
    } catch (e: unknown) {
      if (e instanceof DOMException && e.name === "AbortError") return;
    }
  }
  try {
    await navigator.clipboard.writeText(url);
  } catch {
    err.value = t("views.search.shareFailed");
  }
}

function openLightbox(urls: string[], index: number) {
  const url = urls[index];
  if (!url) return;
  window.open(url, "_blank", "noopener,noreferrer");
}

watch(currentQuery, () => {
  void load();
}, { immediate: true });

void loadMe();
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="grid grid-cols-3">
      <button
        v-for="tab in searchTabs"
        :key="tab.value"
        type="button"
        class="relative border-b-2 py-3 text-base font-semibold transition-colors"
        :class="
          currentTab === tab.value
            ? 'border-lime-600 text-neutral-900'
            : 'border-transparent text-neutral-500 hover:text-neutral-800'
        "
        @click="setSearchTab(tab.value)"
      >
        {{ tab.label }}
      </button>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="grid grid-cols-3">
      <button
        v-for="tab in searchTabs"
        :key="tab.value"
        type="button"
        class="relative border-b-2 py-3 text-base font-semibold transition-colors"
        :class="
          currentTab === tab.value
            ? 'border-lime-600 text-neutral-900'
            : 'border-transparent text-neutral-500 hover:text-neutral-800'
        "
        @click="setSearchTab(tab.value)"
      >
        {{ tab.label }}
      </button>
    </div>
  </Teleport>
  <PullToRefresh :on-refresh="refreshSearch">
    <p v-if="err" class="border-b border-neutral-200 px-4 py-3 text-sm text-red-600">{{ err }}</p>
    <p v-if="busy" class="px-4 py-8 text-center text-sm text-neutral-500">{{ $t("views.search.searching") }}</p>
    <p v-else-if="!currentQuery" class="px-4 py-16 text-center text-sm text-neutral-500">
      {{ $t("views.search.enterQuery") }}
    </p>
    <p v-else-if="currentTab === 'latest' && !items.length" class="px-4 py-16 text-center text-sm text-neutral-500">
      {{ $t("views.search.noPosts") }}
    </p>
    <p v-else-if="currentTab === 'media' && !mediaItems.length" class="px-4 py-16 text-center text-sm text-neutral-500">
      {{ $t("views.search.noMedia") }}
    </p>
    <p v-else-if="currentTab === 'accounts' && !accounts.length" class="px-4 py-16 text-center text-sm text-neutral-500">
      {{ $t("views.search.noAccounts") }}
    </p>
    <ul v-else-if="currentTab === 'accounts'" class="divide-y divide-neutral-200">
      <li v-for="account in accounts" :key="account.handle">
        <RouterLink :to="`/@${account.handle}`" class="flex items-start gap-3 px-4 py-3 transition-colors hover:bg-neutral-50">
          <span
            class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-xs font-bold text-neutral-700"
          >
            <img
              v-if="account.avatar_url"
              :src="account.avatar_url"
              alt=""
              class="h-full w-full object-cover"
            />
            <span v-else>{{ avatarInitials(account.handle) }}</span>
          </span>
          <span class="min-w-0 flex-1">
            <span class="flex flex-wrap items-center gap-1.5">
              <span class="truncate text-sm font-semibold text-neutral-900">{{ account.display_name }}</span>
              <UserBadges :badges="account.badges" size="xs" />
            </span>
            <span class="block truncate text-xs text-neutral-500">{{ fullHandleAt(account.handle) }}</span>
            <span v-if="account.bio" class="mt-1 block whitespace-pre-wrap break-words text-sm text-neutral-600">
              {{ account.bio }}
            </span>
          </span>
        </RouterLink>
      </li>
    </ul>
    <PostTimeline
      v-else
      :items="currentTab === 'media' ? mediaItems : items"
      :action-busy="actionBusy"
      :viewer-email="myEmail"
      show-federated-reply-action
      show-federated-repost-action
      @reply="replyTo"
      @toggle-reaction="toggleReaction"
      @toggle-bookmark="toggleBookmark"
      @toggle-repost="toggleRepost"
      @share="sharePost"
      @open-lightbox="openLightbox"
      @patch-item="({ id, patch }) => patchItem(id, patch)"
      @remove-post="removePost"
    />
  </PullToRefresh>
</template>
