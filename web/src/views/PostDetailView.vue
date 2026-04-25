<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";
import { fetchFeedItem, fetchFederatedThreadReplies, fetchPostThreadReplies, fetchPublicFederatedIncoming } from "../lib/feedStream";
import Icon from "../components/Icon.vue";
import PostTimeline from "../components/PostTimeline.vue";
import RepostModal from "../components/RepostModal.vue";
import { addTimelineReaction, removeTimelineReaction, toggleTimelineBookmark, toggleTimelineRepost } from "../lib/federationActions";
import type { TimelinePost } from "../types/timeline";
import { postDetailPath } from "../lib/feedDisplay";
import { buildComposerReplyQuery, composeRoutePath } from "../lib/postComposer";

type LightboxState = { urls: string[]; index: number };
const lightbox = ref<LightboxState | null>(null);
let lightboxTouchStartX = 0;

function openLightbox(urls: string[], startIndex: number) {
  if (!urls.length) return;
  const i = Math.max(0, Math.min(startIndex, urls.length - 1));
  lightbox.value = { urls, index: i };
}

function closeLightbox() {
  lightbox.value = null;
}

function lightboxPrev() {
  const lb = lightbox.value;
  if (!lb || lb.urls.length <= 1) return;
  const n = lb.urls.length;
  const index = (lb.index - 1 + n) % n;
  lightbox.value = { urls: lb.urls, index };
}

function lightboxNext() {
  const lb = lightbox.value;
  if (!lb || lb.urls.length <= 1) return;
  const n = lb.urls.length;
  const index = (lb.index + 1) % n;
  lightbox.value = { urls: lb.urls, index };
}

function onLightboxTouchStart(e: TouchEvent) {
  lightboxTouchStartX = e.touches[0]?.clientX ?? 0;
}

function onLightboxTouchEnd(e: TouchEvent) {
  const x = e.changedTouches[0]?.clientX ?? 0;
  const dx = x - lightboxTouchStartX;
  const threshold = 56;
  if (dx < -threshold) lightboxNext();
  else if (dx > threshold) lightboxPrev();
}

function onLightboxKeydown(e: KeyboardEvent) {
  if (!lightbox.value) return;
  if (e.key === "Escape") {
    e.preventDefault();
    closeLightbox();
  } else if (e.key === "ArrowLeft") {
    e.preventDefault();
    lightboxPrev();
  } else if (e.key === "ArrowRight") {
    e.preventDefault();
    lightboxNext();
  }
}

const router = useRouter();
const route = useRoute();
const { t } = useI18n();

const item = ref<TimelinePost | null>(null);
const threadRepliesByRoot = ref<Record<string, TimelinePost[]>>({});
const err = ref("");
const loading = ref(true);
const myEmail = ref<string | null>(null);
const actionBusy = ref<string | null>(null);
const repostModalOpen = ref(false);
const repostTarget = ref<TimelinePost | null>(null);
const actionToast = ref("");
const paypalSubscriptionNotice = ref<"ok" | "cancelled" | "">("");
let toastTimer: ReturnType<typeof setTimeout> | null = null;

function showToast(msg: string) {
  if (toastTimer) clearTimeout(toastTimer);
  actionToast.value = msg;
  toastTimer = setTimeout(() => {
    actionToast.value = "";
    toastTimer = null;
  }, 2200);
}

function consumePayPalSubscriptionQuery() {
  const raw = route.query.paypal_subscription;
  const value = typeof raw === "string" ? raw : Array.isArray(raw) ? String(raw[0] ?? "") : "";
  if (value !== "ok" && value !== "cancelled") return;
  paypalSubscriptionNotice.value = value;
  if (value === "ok") {
    showToast(t("views.postDetail.paypalSubscriptionOkToast"));
  }
  const nextQuery = { ...route.query };
  delete nextQuery.paypal_subscription;
  void router.replace({ path: route.path, query: nextQuery, hash: route.hash });
}

function patchItem(id: string, patch: Partial<TimelinePost>) {
  if (item.value?.id === id) {
    item.value = { ...item.value, ...patch };
  }
  const rootId = item.value?.id;
  if (!rootId) return;
  const list = threadRepliesByRoot.value[rootId];
  if (!list?.some((x) => x.id === id)) return;
  threadRepliesByRoot.value = {
    ...threadRepliesByRoot.value,
    [rootId]: list.map((x) => (x.id === id ? { ...x, ...patch } : x)),
  };
}

async function loadThread() {
  const root = item.value;
  if (!root) {
    threadRepliesByRoot.value = {};
    return;
  }
  const token = getAccessToken();
  const list = root.is_federated
    ? await fetchFederatedThreadReplies(root.id.replace(/^federated:/, ""), token)
    : await fetchPostThreadReplies(root.id, token);
  threadRepliesByRoot.value = list.length ? { [root.id]: list } : {};
}

async function refreshThreadDetail() {
  const rootId = item.value?.id;
  if (!rootId) {
    threadRepliesByRoot.value = {};
    return;
  }
  await loadThread();
}

async function loadMe() {
  const token = getAccessToken();
  if (!token) return;
  try {
    const u = await api<{ email: string }>("/api/v1/me", { method: "GET", token });
    myEmail.value = u.email;
  } catch {
    myEmail.value = null;
  }
}

async function loadPost() {
  const incomingId =
    typeof route.params.incomingId === "string"
      ? route.params.incomingId
      : Array.isArray(route.params.incomingId)
        ? route.params.incomingId[0]
        : "";
  if (incomingId) {
    loading.value = true;
    err.value = "";
    try {
      const row = await fetchPublicFederatedIncoming(incomingId);
      item.value = row;
      if (!row) {
        err.value = t("views.postDetail.notFoundFederated");
        threadRepliesByRoot.value = {};
      } else {
        await loadThread();
      }
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : t("views.postDetail.loadFailed");
      item.value = null;
      threadRepliesByRoot.value = {};
    } finally {
      loading.value = false;
    }
    return;
  }

  const id = typeof route.params.postId === "string" ? route.params.postId : Array.isArray(route.params.postId) ? route.params.postId[0] : "";
  if (!id) {
    err.value = t("views.postDetail.invalidId");
    item.value = null;
    loading.value = false;
    return;
  }
  loading.value = true;
  err.value = "";
  try {
    const token = getAccessToken();
    const row = await fetchFeedItem(id, token);
    item.value = row;
    if (!row) {
      err.value = t("views.postDetail.notFound");
      threadRepliesByRoot.value = {};
    } else {
      await loadThread();
    }
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.postDetail.loadFailed");
    item.value = null;
    threadRepliesByRoot.value = {};
  } finally {
    loading.value = false;
  }
}

function startReply(it: TimelinePost) {
  const replyQuery = buildComposerReplyQuery(it);
  const composePath = composeRoutePath();
  const token = getAccessToken();
  if (!token) {
    void router.push({
      path: "/login",
      query: {
        next: router.resolve({ path: composePath, query: replyQuery }).fullPath,
      },
    });
    return;
  }
  void router.push({
    path: composePath,
    query: replyQuery,
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
  } catch {
    showToast(t("views.feed.toasts.reactionUpdateFailed"));
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
  } catch {
    showToast(t("views.feed.toasts.bookmarkUpdateFailed"));
  } finally {
    actionBusy.value = null;
  }
}

async function onToggleRepost(it: TimelinePost) {
  const token = getAccessToken();
  if (!token || actionBusy.value === `rp-${it.id}`) return;
  if (it.reposted_by_me) {
    actionBusy.value = `rp-${it.id}`;
    try {
      const res = await toggleTimelineRepost(token, it);
      patchItem(it.id, { reposted_by_me: res.reposted, repost_count: res.repost_count });
    } catch {
      showToast(t("views.feed.toasts.repostUpdateFailed"));
    } finally {
      actionBusy.value = null;
    }
    return;
  }
  repostTarget.value = it;
  repostModalOpen.value = true;
}

async function confirmRepostPlain() {
  const it = repostTarget.value;
  const token = getAccessToken();
  if (!it || !token) {
    repostModalOpen.value = false;
    repostTarget.value = null;
    return;
  }
  actionBusy.value = `rp-${it.id}`;
  try {
    const res = await toggleTimelineRepost(token, it);
    patchItem(it.id, { reposted_by_me: res.reposted, repost_count: res.repost_count });
    repostModalOpen.value = false;
    repostTarget.value = null;
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : "";
    showToast(msg === "repost_comment_too_long" ? t("views.feed.toasts.repostCommentTooLong") : t("views.feed.toasts.repostFailed"));
  } finally {
    actionBusy.value = null;
  }
}

async function confirmRepostWithComment(text: string) {
  const it = repostTarget.value;
  const token = getAccessToken();
  if (!it || !token) {
    repostModalOpen.value = false;
    repostTarget.value = null;
    return;
  }
  actionBusy.value = `rp-${it.id}`;
  try {
    const res = await toggleTimelineRepost(token, it, text);
    patchItem(it.id, { reposted_by_me: res.reposted, repost_count: res.repost_count });
    repostModalOpen.value = false;
    repostTarget.value = null;
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : "";
    showToast(msg === "repost_comment_too_long" ? t("views.feed.toasts.repostCommentTooLong") : t("views.feed.toasts.repostFailed"));
  } finally {
    actionBusy.value = null;
  }
}

async function sharePost(it: TimelinePost) {
  const resolved = router.resolve({ path: postDetailPath(it.id) });
  const url = new URL(resolved.href, window.location.origin).href;
  if (navigator.share) {
    try {
      await navigator.share({ title: t("views.search.shareFallbackTitle"), text: it.caption?.slice(0, 80) ?? t("views.search.shareFallbackText"), url });
      return;
    } catch (e: unknown) {
      if (e instanceof DOMException && e.name === "AbortError") return;
    }
  }
  try {
    await navigator.clipboard.writeText(url);
    showToast(t("views.feed.toasts.linkCopied"));
  } catch {
    showToast(t("views.feed.toasts.shareFailed"));
  }
}

async function removePost(id: string) {
  if (item.value?.id === id) {
    item.value = null;
    threadRepliesByRoot.value = {};
    void router.back();
    return;
  }
  const token = getAccessToken();
  await refreshThreadDetail();
  if (item.value) {
    const row = await fetchFeedItem(item.value.id, token);
    if (row) item.value = row;
  }
}

watch(
  () => [route.params.postId, route.params.incomingId],
  () => {
    void loadPost();
  },
);

watch(
  () => route.query.paypal_subscription,
  () => consumePayPalSubscriptionQuery(),
);

watch(lightbox, (v) => {
  if (v) {
    document.body.style.overflow = "hidden";
    window.addEventListener("keydown", onLightboxKeydown);
  } else {
    document.body.style.overflow = "";
    window.removeEventListener("keydown", onLightboxKeydown);
  }
});

onBeforeUnmount(() => {
  document.body.style.overflow = "";
  window.removeEventListener("keydown", onLightboxKeydown);
  if (toastTimer) clearTimeout(toastTimer);
});

onMounted(() => {
  void loadMe();
  void loadPost();
  consumePayPalSubscriptionQuery();
});
</script>

<template>
  <div class="min-h-0 h-full w-full min-w-0 text-neutral-900">
    <header
      class="sticky top-0 z-10 flex h-12 items-center gap-3 border-b border-neutral-200 bg-white/90 px-3 backdrop-blur supports-[backdrop-filter]:bg-white/70"
    >
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="$t('common.back.previous')"
        @click="router.back()"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <h1 class="text-lg font-bold tracking-tight">{{ t("views.postDetail.title") }}</h1>
    </header>

    <p v-if="actionToast" class="border-b border-lime-100 bg-lime-50 px-4 py-2 text-center text-sm text-lime-900">
      {{ actionToast }}
    </p>

    <div
      v-if="paypalSubscriptionNotice"
      class="border-b px-4 py-3 text-sm"
      :class="paypalSubscriptionNotice === 'ok' ? 'border-emerald-100 bg-emerald-50 text-emerald-950' : 'border-amber-100 bg-amber-50 text-amber-950'"
    >
      <p class="font-medium">
        {{
          paypalSubscriptionNotice === "ok"
            ? t("views.postDetail.paypalSubscriptionOkTitle")
            : t("views.postDetail.paypalSubscriptionCancelledTitle")
        }}
      </p>
      <p class="mt-1 text-xs">
        {{
          paypalSubscriptionNotice === "ok"
            ? t("views.postDetail.paypalSubscriptionOkBody")
            : t("views.postDetail.paypalSubscriptionCancelledBody")
        }}
      </p>
    </div>

    <div
      v-if="!loading && !err && item?.is_federated && item.remote_object_url"
      class="border-b border-violet-200 bg-violet-50 px-4 py-2.5 text-sm text-violet-950"
    >
      <a
        :href="item.remote_object_url"
        target="_blank"
        rel="noopener noreferrer"
        class="font-medium text-lime-800 underline decoration-lime-600/60 underline-offset-2 hover:text-lime-900"
      >
        {{ t("views.postDetail.openOriginalExternal") }}
      </a>
    </div>

    <p v-if="loading" class="border-b border-neutral-200 px-4 py-12 text-center text-sm text-neutral-500">{{ t("app.loading") }}</p>
    <p v-else-if="err" class="border-b border-neutral-200 px-4 py-8 text-center text-sm text-red-600">{{ err }}</p>

    <PostTimeline
      v-else-if="item"
      :items="[item]"
      :thread-replies-by-root="threadRepliesByRoot"
      :action-busy="actionBusy"
      :viewer-email="myEmail"
      hide-post-detail-link
      show-remote-object-link
      show-federated-reply-action
      show-federated-repost-action
      @reply="startReply"
      @toggle-reaction="toggleReaction"
      @toggle-bookmark="toggleBookmark"
      @toggle-repost="onToggleRepost"
      @share="sharePost"
      @open-lightbox="openLightbox"
      @patch-item="({ id, patch }) => patchItem(id, patch)"
      @remove-post="removePost"
    />
  </div>

  <Teleport to="body">
    <div
      v-if="lightbox"
      class="fixed inset-0 z-[100] flex flex-col"
      role="dialog"
      aria-modal="true"
      :aria-label="t('views.feed.lightboxTitle')"
    >
      <div class="absolute inset-0 bg-black/90" aria-hidden="true" @click="closeLightbox" />
      <div class="relative z-10 flex min-h-0 flex-1 flex-col">
        <div
          class="flex shrink-0 items-center gap-2 px-2 py-2"
          :class="lightbox.urls.length > 1 ? 'justify-between' : 'justify-end'"
        >
          <p v-if="lightbox.urls.length > 1" class="text-sm tabular-nums text-white/90">
            {{ lightbox.index + 1 }} / {{ lightbox.urls.length }}
          </p>
          <button
            type="button"
            class="rounded-full p-2 text-white hover:bg-white/10 focus-visible:outline focus-visible:ring-2 focus-visible:ring-lime-400"
            :aria-label="t('views.feed.lightboxClose')"
            @click="closeLightbox"
          >
            <Icon name="close" class="h-6 w-6" stroke-width="2" />
          </button>
        </div>
        <div class="relative flex min-h-0 flex-1 items-stretch justify-center px-0 pb-4 sm:px-2">
          <button
            v-if="lightbox.urls.length > 1"
            type="button"
            class="z-20 hidden w-10 shrink-0 items-center justify-center self-center rounded-r-md text-3xl font-light text-white/90 hover:bg-white/10 sm:flex"
            :aria-label="t('views.feed.lightboxPrev')"
            @click="lightboxPrev"
          >
            ‹
          </button>
          <div
            class="relative min-h-0 w-full max-w-6xl flex-1 overflow-hidden"
            @touchstart.passive="onLightboxTouchStart"
            @touchend.passive="onLightboxTouchEnd"
          >
            <div
              class="flex h-full w-full transition-transform duration-300 ease-out"
              :style="{ transform: `translateX(-${lightbox.index * 100}%)` }"
            >
              <div
                v-for="(url, li) in lightbox.urls"
                :key="`${url}-${li}`"
                class="flex h-full w-full shrink-0 items-center justify-center px-2 sm:px-4"
              >
                <img
                  :src="url"
                  :alt="t('views.feed.lightboxImageAlt', { n: li + 1 })"
                  class="max-h-[min(88vh,100%)] max-w-full object-contain select-none"
                  draggable="false"
                />
              </div>
            </div>
          </div>
          <button
            v-if="lightbox.urls.length > 1"
            type="button"
            class="z-20 hidden w-10 shrink-0 items-center justify-center self-center rounded-l-md text-3xl font-light text-white/90 hover:bg-white/10 sm:flex"
            :aria-label="t('views.feed.lightboxNext')"
            @click="lightboxNext"
          >
            ›
          </button>
        </div>
        <p v-if="lightbox.urls.length > 1" class="pb-3 text-center text-xs text-white/60 sm:hidden">
          {{ t("views.feed.lightboxSwipeHint") }}
        </p>
      </div>
    </div>
  </Teleport>

  <RepostModal
    v-model:open="repostModalOpen"
    :post="repostTarget"
    :submitting="!!actionBusy && actionBusy.startsWith('rp-')"
    @plain="confirmRepostPlain"
    @with-comment="confirmRepostWithComment"
  />
</template>
