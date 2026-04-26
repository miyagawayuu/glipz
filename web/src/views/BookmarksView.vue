<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import PullToRefresh from "../components/PullToRefresh.vue";
import PostTimeline from "../components/PostTimeline.vue";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";
import { safeMediaURL } from "../lib/redirect";
import { fetchFederatedThreadReplies, fetchPostThreadReplies, mapFeedItem } from "../lib/feedStream";
import { postDetailPath } from "../lib/feedDisplay";
import { buildComposerReplyQuery, composeRoutePath } from "../lib/postComposer";
import { addTimelineReaction, removeTimelineReaction, toggleTimelineBookmark, toggleTimelineRepost } from "../lib/federationActions";
import type { TimelinePost } from "../types/timeline";
import { translate } from "../i18n";

const router = useRouter();
const { t } = useI18n();

const items = ref<TimelinePost[]>([]);
const threadRepliesByRoot = ref<Record<string, TimelinePost[]>>({});
const err = ref("");
const busy = ref(false);
const myEmail = ref<string | null>(null);
const actionBusy = ref<string | null>(null);

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

function patchItem(id: string, patch: Partial<TimelinePost>) {
  items.value = items.value.map((x) => (x.id === id ? { ...x, ...patch } : x));
  const next = { ...threadRepliesByRoot.value };
  let changed = false;
  for (const key of Object.keys(next)) {
    if (!next[key].some((x) => x.id === id)) continue;
    next[key] = next[key].map((x) => (x.id === id ? { ...x, ...patch } : x));
    changed = true;
  }
  if (changed) threadRepliesByRoot.value = next;
}

function removePost(id: string) {
  items.value = items.value.filter((x) => x.id !== id);
  const next = { ...threadRepliesByRoot.value };
  delete next[id];
  for (const key of Object.keys(next)) {
    next[key] = next[key].filter((x) => x.id !== id);
    if (!next[key].length) delete next[key];
  }
  threadRepliesByRoot.value = next;
}

async function loadThreadsForFeed() {
  const token = getAccessToken();
  if (!token) return;
  const roots = items.value.filter((x) => !x.reply_to_post_id && !x.reply_to_object_url && (x.reply_count > 0 || x.is_federated));
  const next: Record<string, TimelinePost[]> = {};
  await Promise.all(
    roots.map(async (it) => {
      const rows = it.is_federated
        ? await fetchFederatedThreadReplies(it.id.replace(/^federated:/, ""), token)
        : await fetchPostThreadReplies(it.id, token);
      if (rows.length) next[it.id] = rows;
    }),
  );
  threadRepliesByRoot.value = next;
}

async function load() {
  const token = getAccessToken();
  if (!token) {
    await router.replace("/login");
    return;
  }
  busy.value = true;
  err.value = "";
  try {
    const res = await api<{ items: Record<string, unknown>[] }>("/api/v1/posts/bookmarks", {
      method: "GET",
      token,
    });
    items.value = (res.items ?? []).map((x) => mapFeedItem(x as Parameters<typeof mapFeedItem>[0]));
    await loadThreadsForFeed();
  } catch (e: unknown) {
    items.value = [];
    threadRepliesByRoot.value = {};
    err.value = e instanceof Error ? e.message : t("views.bookmarks.loadFailed");
  } finally {
    busy.value = false;
  }
}

async function refreshBookmarks() {
  await load();
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
    err.value = e instanceof Error ? e.message : t("views.bookmarks.reactionFailed");
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
    if (res.bookmarked) {
      patchItem(it.id, { bookmarked_by_me: true });
    } else {
      removePost(it.id);
    }
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.bookmarks.bookmarkFailed");
  } finally {
    actionBusy.value = null;
  }
}

async function toggleRepost(it: TimelinePost) {
  const token = getAccessToken();
  if (!token || actionBusy.value === `rp-${it.id}`) return;
  actionBusy.value = `rp-${it.id}`;
  try {
    const res = await toggleTimelineRepost(token, it);
    patchItem(it.id, { reposted_by_me: res.reposted, repost_count: res.repost_count });
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.bookmarks.repostFailed");
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
        title: translate("views.bookmarks.shareFallbackTitle"),
        text: it.caption?.slice(0, 80) ?? translate("views.bookmarks.shareFallbackText"),
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
    err.value = t("views.bookmarks.shareFailed");
  }
}

function openLightbox(urls: string[], index: number) {
  const url = safeMediaURL(urls[index]);
  if (!url) return;
  window.open(url, "_blank", "noopener,noreferrer");
}

onMounted(() => {
  void loadMe();
  void load();
});
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="flex h-14 items-center justify-between gap-3">
      <h1 class="truncate text-lg font-bold">{{ $t("views.bookmarks.title") }}</h1>
      <button
        type="button"
        class="shrink-0 rounded-full border border-neutral-200 px-3 py-1.5 text-sm font-medium text-neutral-700 hover:bg-neutral-50"
        @click="load"
      >
        {{ $t("views.bookmarks.refresh") }}
      </button>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="flex items-center justify-between gap-3 px-4 py-4">
      <div>
        <h1 class="text-xl font-bold">{{ $t("views.bookmarks.title") }}</h1>
        <p class="mt-1 text-sm text-neutral-600">{{ $t("views.bookmarks.description") }}</p>
      </div>
      <button
        type="button"
        class="rounded-full border border-neutral-200 px-3 py-1.5 text-sm font-medium text-neutral-700 hover:bg-neutral-50"
        @click="load"
      >
        {{ $t("views.bookmarks.refresh") }}
      </button>
    </div>
  </Teleport>
  <PullToRefresh :on-refresh="refreshBookmarks">
    <p v-if="err" class="border-b border-neutral-200 px-4 py-3 text-sm text-red-600">{{ err }}</p>
    <p v-if="busy" class="px-4 py-8 text-center text-sm text-neutral-500">{{ $t("app.loading") }}</p>
    <p v-else-if="!items.length" class="px-4 py-16 text-center text-sm text-neutral-500">
      {{ $t("views.bookmarks.empty") }}
    </p>
    <PostTimeline
      v-else
      :items="items"
      :thread-replies-by-root="threadRepliesByRoot"
      :action-busy="actionBusy"
      :viewer-email="myEmail"
      show-reply-parent-link
      show-remote-object-link
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
