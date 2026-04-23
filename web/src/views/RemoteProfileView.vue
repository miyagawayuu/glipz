<script setup lang="ts">
import DOMPurify from "dompurify";
import { computed, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import { api, apiBase, apiPublicGet } from "../lib/api";
import { addTimelineReaction, removeTimelineReaction, toggleTimelineBookmark, toggleTimelineRepost } from "../lib/federationActions";
import { mapFeedItem } from "../lib/feedStream";
import { buildComposerReplyQuery, composeRoutePath } from "../lib/postComposer";
import Icon from "../components/Icon.vue";
import PostTimeline from "../components/PostTimeline.vue";
import type { TimelinePost } from "../types/timeline";
type RemoteProfile = {
  actor_id: string;
  acct: string;
  name: string;
  summary?: string;
  icon_url?: string;
  header_url?: string;
  profile_url?: string;
};

const route = useRoute();
const router = useRouter();

const profile = ref<RemoteProfile | null>(null);
const posts = ref<TimelinePost[]>([]);
const err = ref("");
const busy = ref(true);
const followModalOpen = ref(false);
const followBusy = ref(false);
const toast = ref("");
const remoteFollowState = ref<"none" | "pending" | "accepted">("none");
const actionBusy = ref<string | null>(null);
const dmBusy = ref(false);

const acctForQuery = computed(() => {
  const fu = route.params.fedUser;
  const fh = route.params.fedHost;
  if (typeof fu === "string" && typeof fh === "string" && fu && fh) {
    return `${fu}@${fh}`.replace(/^@/, "");
  }
  const q = route.query.acct;
  if (typeof q === "string" && q.trim()) return q.trim().replace(/^@/, "");
  return "";
});

const actorForQuery = computed(() => {
  const a = route.query.actor;
  return typeof a === "string" && a.trim() ? a.trim() : "";
});

const profileTitle = computed(() => profile.value?.name || profile.value?.acct || "連合ユーザー");
const viewerAuthed = computed(() => !!getAccessToken());
const profileSummaryHtml = computed(() => {
  const raw = String(profile.value?.summary ?? "").trim();
  if (!raw) return "";
  return DOMPurify.sanitize(raw, {
    ALLOWED_TAGS: ["p", "br", "a", "span", "strong", "em", "b", "i", "ul", "ol", "li", "code"],
    ALLOWED_ATTR: ["href", "target", "rel"],
  });
});
const followButtonLabel = computed(() => {
  if (remoteFollowState.value === "accepted") return "フォロー中";
  if (remoteFollowState.value === "pending") return "承認待ち";
  return "フォロー";
});
const followButtonClass = computed(() => {
  if (!viewerAuthed.value) {
    return "border-neutral-900 bg-neutral-900 text-white hover:bg-neutral-800";
  }
  if (remoteFollowState.value === "accepted") {
    return "border-neutral-200 bg-white text-neutral-800 hover:bg-neutral-50";
  }
  if (remoteFollowState.value === "pending") {
    return "border-neutral-200 bg-white text-neutral-600 hover:bg-neutral-50";
  }
  return "border-transparent bg-neutral-900 text-white hover:bg-neutral-800";
});

function showToast(msg: string) {
  toast.value = msg;
  setTimeout(() => {
    toast.value = "";
  }, 3200);
}

async function loadProfile() {
  err.value = "";
  busy.value = true;
  profile.value = null;
  posts.value = [];
  try {
    let path = "";
    if (actorForQuery.value) {
      path = `/api/v1/public/federation/profile?actor=${encodeURIComponent(actorForQuery.value)}`;
    } else if (acctForQuery.value) {
      path = `/api/v1/public/federation/profile?acct=${encodeURIComponent(acctForQuery.value)}`;
    } else {
      err.value = "acct または actor を指定してください。";
      busy.value = false;
      return;
    }
    profile.value = await apiPublicGet<RemoteProfile>(path);
    await loadPosts();
    await refreshRemoteFollowState();
    startIncomingStream();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : "読み込みに失敗しました";
  } finally {
    busy.value = false;
  }
}

async function loadPosts() {
  if (!profile.value?.actor_id) return;
  try {
    const res = await apiPublicGet<{ items: unknown[] }>(
      `/api/v1/public/federation/incoming?actor=${encodeURIComponent(profile.value.actor_id)}`,
    );
    posts.value = (res.items ?? []).map((x) => mapFeedItem(x as never));
  } catch {
    posts.value = [];
  }
}

let incomingStream: EventSource | null = null;

function stopIncomingStream() {
  try {
    incomingStream?.close();
  } catch {
    // ignore
  }
  incomingStream = null;
}

async function onIncomingEvent(payload: any) {
  const kind = String(payload?.kind ?? "");
  const id = String(payload?.incoming_id ?? "").trim();
  if (!id) return;
  if (kind === "federated_post_deleted") {
    posts.value = posts.value.filter((x) => !String((x as any).id ?? "").endsWith(id));
    return;
  }
  if (kind !== "federated_post_upsert") return;
  try {
    const res = await apiPublicGet<{ item: unknown }>(`/api/v1/public/federation/incoming/${encodeURIComponent(id)}`);
    const it = mapFeedItem(res.item as never);
    const itId = String((it as any).id ?? "");
    posts.value = [it, ...posts.value.filter((x) => String((x as any).id ?? "") !== itId)];
  } catch {
    await loadPosts();
  }
}

function startIncomingStream() {
  stopIncomingStream();
  if (!profile.value?.actor_id) return;
  const url = `${apiBase()}/api/v1/public/federation/incoming/stream?actor=${encodeURIComponent(profile.value.actor_id)}`;
  incomingStream = new EventSource(url);
  incomingStream.addEventListener("federated_incoming", (ev) => {
    try {
      const payload = JSON.parse(String((ev as MessageEvent).data ?? "{}"));
      void onIncomingEvent(payload);
    } catch {
      // ignore
    }
  });
}

function patchPost(id: string, patch: Partial<TimelinePost>) {
  posts.value = posts.value.map((x) => (x.id === id ? { ...x, ...patch } : x));
}

function applyReactionPost(updated: TimelinePost) {
  patchPost(updated.id, {
    reactions: updated.reactions,
    like_count: updated.like_count,
    liked_by_me: updated.liked_by_me,
  });
}

function startReply(it: TimelinePost) {
  void router.push({
    path: composeRoutePath(),
    query: buildComposerReplyQuery(it),
  });
}

async function toggleRepost(it: TimelinePost) {
  const token = getAccessToken();
  if (!token || actionBusy.value === `rp-${it.id}`) return;
  actionBusy.value = `rp-${it.id}`;
  try {
    const res = await toggleTimelineRepost(token, it);
    patchPost(it.id, { reposted_by_me: res.reposted, repost_count: res.repost_count });
  } catch (e: unknown) {
    showToast(e instanceof Error ? e.message : "リポストに失敗しました");
  } finally {
    actionBusy.value = null;
  }
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
    showToast(e instanceof Error ? e.message : "リアクションに失敗しました");
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
    patchPost(it.id, { bookmarked_by_me: res.bookmarked });
  } catch (e: unknown) {
    showToast(e instanceof Error ? e.message : "ブックマークに失敗しました");
  } finally {
    actionBusy.value = null;
  }
}

async function refreshRemoteFollowState() {
  remoteFollowState.value = "none";
  const token = getAccessToken();
  if (!token || !profile.value?.actor_id) return;
  try {
    const r = await api<{ items: { remote_actor_id: string; state: string }[] }>("/api/v1/federation/remote-follow", {
      method: "GET",
      token,
    });
    const row = (r.items ?? []).find((x) => x.remote_actor_id === profile.value!.actor_id);
    if (!row) remoteFollowState.value = "none";
    else if (row.state === "accepted") remoteFollowState.value = "accepted";
    else remoteFollowState.value = "pending";
  } catch {
    remoteFollowState.value = "none";
  }
}

async function doRemoteFollow() {
  const token = getAccessToken();
  if (!token) {
    followModalOpen.value = false;
    router.push({ path: "/login", query: { next: route.fullPath } });
    return;
  }
  if (!profile.value) return;
  followBusy.value = true;
  try {
    const body = actorForQuery.value
      ? { actor: profile.value.actor_id }
      : { acct: profile.value.acct || acctForQuery.value };
    await api("/api/v1/federation/remote-follow", { method: "POST", token, json: body });
    followModalOpen.value = false;
    showToast("フォローを送信しました");
    await refreshRemoteFollowState();
  } catch (e: unknown) {
    showToast(e instanceof Error ? e.message : "フォローに失敗しました");
  } finally {
    followBusy.value = false;
  }
}

async function doRemoteUnfollow() {
  const token = getAccessToken();
  if (!token || !profile.value) return;
  followBusy.value = true;
  try {
    await api("/api/v1/federation/remote-follow", {
      method: "DELETE",
      token,
      json: { actor: profile.value.actor_id },
    });
    followModalOpen.value = false;
    showToast("フォローを解除しました");
    await refreshRemoteFollowState();
  } catch (e: unknown) {
    showToast(e instanceof Error ? e.message : "解除に失敗しました");
  } finally {
    followBusy.value = false;
  }
}

async function copyActorURL() {
  if (!profile.value?.profile_url) return;
  try {
    await navigator.clipboard.writeText(profile.value.profile_url);
    followModalOpen.value = false;
    showToast("プロフィール URL をコピーしました");
  } catch {
    showToast("コピーできませんでした");
  }
}

async function inviteFederationDM() {
  const token = getAccessToken();
  if (!token || !profile.value?.acct || dmBusy.value) return;
  dmBusy.value = true;
  try {
    const res = await api<{ thread_id: string }>("/api/v1/federation/dm/invite", {
      method: "POST",
      token,
      json: { to_acct: profile.value.acct },
    });
    showToast("DM招待を送信しました");
    if (res.thread_id) {
      void router.push({ path: "/messages", query: { fed_thread: res.thread_id } });
    }
  } catch (e: unknown) {
    showToast(e instanceof Error ? e.message : "DM招待に失敗しました");
  } finally {
    dmBusy.value = false;
  }
}

const avatarLetter = computed(() => {
  const a = profile.value?.acct || "";
  const u = a.split("@")[0] || a;
  return u.slice(0, 2).toUpperCase() || "?";
});

watch(
  () => route.fullPath,
  () => {
    void loadProfile();
  },
  { immediate: true },
);

watch(
  () => profile.value?.actor_id,
  () => {
    startIncomingStream();
  },
);
</script>

<template>
  <div class="min-h-0 h-full w-full min-w-0 text-neutral-900">
    <p
      v-if="toast"
      class="fixed right-4 top-20 z-[150] max-w-sm rounded-lg border border-lime-200 bg-white px-3 py-2 text-sm text-neutral-900 shadow-md"
      role="status"
    >
      {{ toast }}
    </p>
    <header
      class="sticky top-0 z-10 flex h-14 items-center gap-3 border-b border-neutral-200 bg-white/90 px-4 backdrop-blur supports-[backdrop-filter]:bg-white/70"
    >
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        aria-label="戻る"
        @click="router.back()"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <div class="min-w-0">
        <h1 class="truncate text-lg font-bold leading-tight text-neutral-900">
          {{ profile ? profileTitle : "連合ユーザー" }}
        </h1>
        <p class="truncate text-sm text-neutral-500">{{ profile ? `@${profile.acct}` : "" }}</p>
      </div>
    </header>

    <p v-if="err" class="border-b border-neutral-200 px-4 py-3 text-sm text-red-600">{{ err }}</p>
    <div v-if="busy && !profile" class="border-b border-neutral-200 px-4 py-12 text-center text-sm text-neutral-500">読み込み中…</div>
    <template v-else-if="profile">
      <div class="relative">
        <div
          class="relative h-36 w-full overflow-hidden bg-gradient-to-br from-lime-200 via-lime-100 to-neutral-200 sm:h-44"
          :style="
            profile.header_url
              ? `background-image: url(${profile.header_url}); background-size: cover; background-position: center`
              : ''
          "
        />
        <div class="relative -mt-12 flex flex-col gap-3 px-4 pb-2">
          <div class="flex items-end justify-between gap-3">
            <div
              class="flex h-24 w-24 shrink-0 items-center justify-center overflow-hidden rounded-full border-4 border-white bg-lime-500 text-xl font-bold text-white shadow-sm"
            >
              <img
                v-if="profile.icon_url"
                :src="profile.icon_url"
                alt=""
                class="h-full w-full object-cover"
              />
              <span v-else>{{ avatarLetter }}</span>
            </div>
            <div class="flex flex-wrap items-center justify-end gap-2">
              <button
                type="button"
                class="shrink-0 rounded-full border px-4 py-1.5 text-sm font-semibold transition-colors disabled:opacity-50"
                :class="followButtonClass"
                :disabled="followBusy"
                @click="followModalOpen = true"
              >
                {{ followButtonLabel }}
              </button>
              <button
                v-if="viewerAuthed && remoteFollowState === 'accepted'"
                type="button"
                class="shrink-0 rounded-full border border-neutral-900 bg-neutral-900 px-4 py-1.5 text-sm font-semibold text-white hover:bg-neutral-800 disabled:opacity-50"
                :disabled="dmBusy"
                @click="inviteFederationDM"
              >
                DM招待
              </button>
              <a
                v-if="profile.profile_url"
                :href="profile.profile_url"
                target="_blank"
                rel="noopener noreferrer"
                class="shrink-0 rounded-full border border-lime-600 bg-white px-4 py-1.5 text-sm font-semibold text-lime-700 hover:bg-lime-50"
              >
                元のプロフィール
              </a>
              <RouterLink
                v-else-if="!viewerAuthed"
                :to="{ path: '/login', query: { next: route.fullPath } }"
                class="shrink-0 rounded-full border border-lime-600 bg-white px-4 py-1.5 text-sm font-semibold text-lime-700 hover:bg-lime-50"
              >
                ログイン
              </RouterLink>
            </div>
          </div>
          <div>
            <p class="text-xl font-bold text-neutral-900">{{ profileTitle }}</p>
            <p class="text-sm text-neutral-500">@{{ profile.acct }}</p>
            <p class="mt-1 text-sm text-neutral-600">
              連合アカウント
              <span class="mx-1.5 text-neutral-300">·</span>
              このサーバーで受信した投稿を表示
            </p>
          </div>
          <div v-if="profileSummaryHtml" class="prose prose-sm max-w-none text-neutral-800" v-html="profileSummaryHtml" />
          <p v-else class="text-sm text-neutral-400">自己紹介はまだありません。</p>
        </div>
      </div>

      <div class="sticky top-0 z-[5] border-b border-neutral-200 bg-white/95 px-2 pt-1 backdrop-blur supports-[backdrop-filter]:bg-white/80">
        <div class="flex flex-nowrap gap-1 overflow-x-auto">
          <button
            type="button"
            class="relative shrink-0 -mb-px border-b-2 border-lime-600 px-3 py-2.5 text-sm font-semibold text-neutral-900"
          >
            投稿
          </button>
        </div>
      </div>

      <section>
        <PostTimeline
          v-if="posts.length"
          :items="posts"
          :action-busy="actionBusy"
          :hide-post-detail-link="false"
          :thread-replies-by-root="null"
          :embed-thread-replies="false"
          show-federated-reply-action
          show-federated-repost-action
          @reply="startReply"
          @toggle-repost="toggleRepost"
          @toggle-reaction="toggleReaction"
          @toggle-bookmark="toggleBookmark"
          @patch-item="({ id, patch }) => patchPost(id, patch)"
        />
        <p v-else class="border-b border-neutral-200 px-4 py-10 text-center text-sm text-neutral-500">
          まだ受信した投稿がありません。
        </p>
      </section>
    </template>

    <div
      v-if="followModalOpen"
      class="fixed inset-0 z-[160] flex items-center justify-center bg-black/40 p-4"
      role="presentation"
      @click.self="followModalOpen = false"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="follow-modal-title"
        class="w-full max-w-md rounded-2xl border border-neutral-200 bg-white p-5 shadow-xl"
        @click.stop
      >
        <h2 id="follow-modal-title" class="text-lg font-semibold text-neutral-900">フォロー</h2>
        <p class="mt-2 text-sm text-neutral-600">
          Glipz の独自連合でフォローするか、プロフィール URL をコピーできます。
        </p>
        <div class="mt-5 flex flex-col gap-2">
          <button
            v-if="remoteFollowState === 'none'"
            type="button"
            class="w-full rounded-lg bg-lime-600 py-2.5 text-sm font-medium text-white hover:bg-lime-700 disabled:opacity-50"
            :disabled="followBusy"
            @click="doRemoteFollow"
          >
            Glipz からフォローする
          </button>
          <button
            v-if="remoteFollowState === 'pending'"
            type="button"
            disabled
            class="w-full rounded-lg border border-neutral-200 bg-neutral-50 py-2.5 text-sm text-neutral-600"
          >
            承認待ち（相手の Accept を待っています）
          </button>
          <button
            v-if="remoteFollowState === 'accepted'"
            type="button"
            class="w-full rounded-lg border border-red-200 bg-red-50 py-2.5 text-sm font-medium text-red-800 hover:bg-red-100 disabled:opacity-50"
            :disabled="followBusy"
            @click="doRemoteUnfollow"
          >
            フォロー解除
          </button>
          <button
            type="button"
            class="w-full rounded-lg border border-neutral-200 bg-white py-2.5 text-sm font-medium text-neutral-800 hover:bg-neutral-50"
            @click="copyActorURL"
          >
            プロフィール URL をコピー
          </button>
          <button
            type="button"
            class="w-full rounded-lg py-2 text-sm text-neutral-600 hover:bg-neutral-50"
            @click="followModalOpen = false"
          >
            閉じる
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
