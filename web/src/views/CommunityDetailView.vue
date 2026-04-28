<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import Icon from "../components/Icon.vue";
import PostTimeline from "../components/PostTimeline.vue";
import PullToRefresh from "../components/PullToRefresh.vue";
import RepostModal from "../components/RepostModal.vue";
import { api, uploadMediaFile } from "../lib/api";
import {
  getCommunityPostMediaTiles,
  getCommunityPosts,
  listCommunityJoinRequests,
  requestCommunityJoin,
  reviewCommunityJoinRequest,
  updateCommunity,
  deleteCommunity,
  type Community,
  type CommunityJoinRequest,
  type CommunityMediaTile,
} from "../lib/communities";
import { addTimelineReaction, removeTimelineReaction, toggleTimelineBookmark, toggleTimelineRepost } from "../lib/federationActions";
import { avatarInitials, postDetailPath } from "../lib/feedDisplay";
import { fetchPostThreadReplies } from "../lib/feedStream";
import { buildComposerReplyQuery, composeRoutePath } from "../lib/postComposer";
import { safeMediaURL } from "../lib/redirect";
import type { TimelinePost } from "../types/timeline";

type CommunityTab = "recommended" | "latest" | "media" | "details";

const route = useRoute();
const router = useRouter();
const { t } = useI18n();
const community = ref<Community | null>(null);
const posts = ref<TimelinePost[]>([]);
const activeTab = ref<CommunityTab>("recommended");
const threadRepliesByRoot = ref<Record<string, TimelinePost[]>>({});
const mediaTiles = ref<CommunityMediaTile[]>([]);
const mediaTilesLoaded = ref(false);
const mediaTilesBusy = ref(false);
const mediaTilesErr = ref("");
const joinRequests = ref<CommunityJoinRequest[]>([]);
const myEmail = ref<string | null>(null);
const busy = ref(false);
const err = ref("");
const actionBusy = ref<string | null>(null);
const toast = ref("");
const repostModalOpen = ref(false);
const repostTarget = ref<TimelinePost | null>(null);
const editingCommunity = ref(false);
const editCommunityName = ref("");
const editCommunityDescription = ref("");
const editCommunityDetails = ref("");
const imageEditBusy = ref<"icon" | "header" | null>(null);
let toastTimer: ReturnType<typeof setTimeout> | null = null;
type LightboxState = { urls: string[]; index: number };
const lightbox = ref<LightboxState | null>(null);
let lightboxTouchStartX = 0;

const communityID = computed(() => String(route.params.id || ""));
const signedIn = computed(() => Boolean(getAccessToken()));
const isOwner = computed(() => community.value?.viewer_role === "owner" && community.value?.viewer_status === "approved");
const canManageCommunity = computed(() => Boolean(community.value?.can_manage || isOwner.value));
const timelineTabActive = computed(() => activeTab.value === "recommended" || activeTab.value === "latest");
const latestPosts = computed(() =>
  [...posts.value].sort((a, b) => String(b.visible_at || b.created_at || "").localeCompare(String(a.visible_at || a.created_at || ""))),
);
const recommendedPosts = computed(() =>
  [...posts.value].sort((a, b) => {
    const scoreA = a.like_count * 3 + a.repost_count * 4 + a.reply_count * 2 + a.reactions.reduce((sum, rx) => sum + rx.count, 0);
    const scoreB = b.like_count * 3 + b.repost_count * 4 + b.reply_count * 2 + b.reactions.reduce((sum, rx) => sum + rx.count, 0);
    if (scoreA !== scoreB) return scoreB - scoreA;
    return String(b.visible_at || b.created_at || "").localeCompare(String(a.visible_at || a.created_at || ""));
  }),
);
const visiblePosts = computed(() => {
  if (activeTab.value === "latest") return latestPosts.value;
  if (activeTab.value === "recommended") return recommendedPosts.value;
  return [];
});
const emptyTabMessage = computed(() => {
  if (activeTab.value === "latest") return t("views.communityDetail.emptyLatest");
  return t("views.communityDetail.emptyTimeline");
});
const communityTabs = computed<Array<{ key: CommunityTab; label: string }>>(() => [
  { key: "recommended", label: t("views.communityDetail.tabs.recommended") },
  { key: "latest", label: t("views.communityDetail.tabs.latest") },
  { key: "media", label: t("views.communityDetail.tabs.media") },
  { key: "details", label: t("views.communityDetail.tabs.details") },
]);
const memberPreviews = computed(() => community.value?.member_previews?.slice(0, 5) ?? []);

function safeMemberAvatarURL(raw: unknown): string {
  return safeMediaURL(raw);
}

function showToast(message: string) {
  if (toastTimer) clearTimeout(toastTimer);
  toast.value = message;
  toastTimer = setTimeout(() => {
    toast.value = "";
    toastTimer = null;
  }, 2200);
}

async function load() {
  busy.value = true;
  err.value = "";
  try {
    mediaTilesLoaded.value = false;
    mediaTiles.value = [];
    mediaTilesErr.value = "";
    const res = await getCommunityPosts(communityID.value);
    community.value = res.community;
    posts.value = res.items;
    await loadThreadsForCommunity();
    if (res.community.viewer_role === "owner" && res.community.viewer_status === "approved") {
      joinRequests.value = await listCommunityJoinRequests(communityID.value);
    } else {
      joinRequests.value = [];
    }
    if (activeTab.value === "media") await loadMediaTiles();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : "load_failed";
  } finally {
    busy.value = false;
  }
}

async function loadMeEmail() {
  const token = getAccessToken();
  if (!token) return;
  try {
    const u = await api<{ email: string }>("/api/v1/me", { method: "GET", token });
    myEmail.value = u.email;
  } catch {
    myEmail.value = null;
  }
}

function patchItem(id: string, patch: Partial<TimelinePost>) {
  posts.value = posts.value.map((x) => (x.id === id ? { ...x, ...patch } : x));
  const tr = { ...threadRepliesByRoot.value };
  let changed = false;
  for (const k of Object.keys(tr)) {
    const list = tr[k];
    if (!list.some((x) => x.id === id)) continue;
    tr[k] = list.map((x) => (x.id === id ? { ...x, ...patch } : x));
    changed = true;
  }
  if (changed) threadRepliesByRoot.value = tr;
}

function removePost(id: string) {
  posts.value = posts.value.filter((x) => x.id !== id);
  const tr = { ...threadRepliesByRoot.value };
  delete tr[id];
  for (const k of Object.keys(tr)) {
    tr[k] = tr[k].filter((x) => x.id !== id);
  }
  threadRepliesByRoot.value = tr;
}

async function refreshCommunity() {
  await load();
}

async function loadThreadsForCommunity() {
  const token = getAccessToken();
  const withReplies = posts.value.filter((x) => x.reply_count > 0 && !x.is_federated);
  const next: Record<string, TimelinePost[]> = {};
  await Promise.all(
    withReplies.map(async (it) => {
      const list = await fetchPostThreadReplies(it.id, token);
      if (list.length) next[it.id] = list;
    }),
  );
  threadRepliesByRoot.value = next;
}

async function loadMediaTiles() {
  if (mediaTilesLoaded.value || mediaTilesBusy.value) return;
  mediaTilesBusy.value = true;
  mediaTilesErr.value = "";
  try {
    mediaTiles.value = await getCommunityPostMediaTiles(communityID.value);
    mediaTilesLoaded.value = true;
  } catch (e: unknown) {
    mediaTilesErr.value = e instanceof Error ? e.message : t("views.userProfile.errors.loadFailed");
  } finally {
    mediaTilesBusy.value = false;
  }
}

function openLightbox(urls: string[], startIndex: number) {
  const safeURLs = urls.map((url) => safeMediaURL(url)).filter(Boolean);
  if (!safeURLs.length) return;
  const i = Math.max(0, Math.min(startIndex, safeURLs.length - 1));
  lightbox.value = { urls: safeURLs, index: i };
}

function closeLightbox() {
  lightbox.value = null;
}

function lightboxPrev() {
  const lb = lightbox.value;
  if (!lb || lb.urls.length <= 1) return;
  const n = lb.urls.length;
  lightbox.value = { urls: lb.urls, index: (lb.index - 1 + n) % n };
}

function lightboxNext() {
  const lb = lightbox.value;
  if (!lb || lb.urls.length <= 1) return;
  const n = lb.urls.length;
  lightbox.value = { urls: lb.urls, index: (lb.index + 1) % n };
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

function onSidebarPostCreated(e: Event) {
  const detail = (e as CustomEvent<{ mode?: string; communityId?: string | null }>).detail;
  if (detail?.mode !== "community") return;
  if (detail.communityId && detail.communityId !== communityID.value) return;
  void load();
}

function openCommunityEdit() {
  if (!community.value) return;
  editCommunityName.value = community.value.name;
  editCommunityDescription.value = community.value.description;
  editCommunityDetails.value = community.value.details;
  editingCommunity.value = true;
}

async function submitCommunityEdit() {
  const token = getAccessToken();
  if (!token || !community.value) return;
  actionBusy.value = "community-edit";
  try {
    const input: { name: string; description: string; details: string; icon_object_key?: string; header_object_key?: string } = {
      name: editCommunityName.value,
      description: editCommunityDescription.value,
      details: editCommunityDetails.value,
    };
    community.value = await updateCommunity(community.value.id, input);
    editingCommunity.value = false;
    showToast(t("views.communityDetail.toasts.communitySaved"));
  } catch {
    showToast(t("views.communityDetail.toasts.communitySaveFailed"));
  } finally {
    actionBusy.value = null;
  }
}

async function updateCommunityImage(kind: "icon" | "header", file: File) {
  const token = getAccessToken();
  if (!token || !community.value) return;
  imageEditBusy.value = kind;
  try {
    const up = await uploadMediaFile(token, file);
    const input: { name: string; description: string; details: string; icon_object_key?: string; header_object_key?: string } = {
      name: community.value.name,
      description: community.value.description,
      details: community.value.details,
    };
    if (kind === "icon") input.icon_object_key = up.object_key;
    else input.header_object_key = up.object_key;
    community.value = await updateCommunity(community.value.id, input);
    showToast(t("views.communityDetail.toasts.communitySaved"));
  } catch {
    showToast(t("views.communityDetail.toasts.communitySaveFailed"));
  } finally {
    imageEditBusy.value = null;
  }
}

function selectInlineCommunityImage(kind: "icon" | "header", e: Event) {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (file) void updateCommunityImage(kind, file);
}

async function requestDeleteCommunity() {
  const token = getAccessToken();
  if (!token || !community.value) return;
  if (!window.confirm(t("views.communityDetail.deleteConfirm"))) return;
  actionBusy.value = "community-delete";
  try {
    await deleteCommunity(community.value.id);
    await router.push("/communities");
  } catch {
    showToast(t("views.communityDetail.toasts.communityDeleteFailed"));
  } finally {
    actionBusy.value = null;
  }
}

async function join() {
  if (!community.value) return;
  actionBusy.value = "join";
  try {
    community.value = await requestCommunityJoin(community.value.id);
  } catch {
    showToast(t("views.communityDetail.toasts.joinFailed"));
  } finally {
    actionBusy.value = null;
  }
}

async function reviewRequest(userId: string, approve: boolean) {
  if (!community.value) return;
  actionBusy.value = `review-${userId}`;
  try {
    await reviewCommunityJoinRequest(community.value.id, userId, approve);
    joinRequests.value = joinRequests.value.filter((x) => x.user_id !== userId);
    await load();
  } catch {
    showToast(t("views.communityDetail.toasts.reviewFailed"));
  } finally {
    actionBusy.value = null;
  }
}

function reply(it: TimelinePost) {
  void router.push({
    path: composeRoutePath(),
    query: buildComposerReplyQuery(it),
  });
}

async function toggleReaction(it: TimelinePost, emoji: string) {
  const token = getAccessToken();
  if (!token || actionBusy.value === `rx-${it.id}`) return;
  actionBusy.value = `rx-${it.id}`;
  try {
    const active = it.reactions.some((reaction) => reaction.emoji === emoji && reaction.reacted_by_me);
    const updated = active ? await removeTimelineReaction(token, it, emoji) : await addTimelineReaction(token, it, emoji);
    patchItem(updated.id, {
      reactions: updated.reactions,
      like_count: updated.like_count,
      liked_by_me: updated.liked_by_me,
    });
  } catch {
    showToast(t("views.communityDetail.toasts.reactionFailed"));
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
    showToast(t("views.communityDetail.toasts.bookmarkFailed"));
  } finally {
    actionBusy.value = null;
  }
}

async function onToggleRepost(it: TimelinePost) {
  const token = getAccessToken();
  if (!token) return;
  if (!it.reposted_by_me) {
    repostTarget.value = it;
    repostModalOpen.value = true;
    return;
  }
  actionBusy.value = `rp-${it.id}`;
  try {
    const res = await toggleTimelineRepost(token, it);
    patchItem(it.id, { reposted_by_me: res.reposted, repost_count: res.repost_count });
  } catch {
    showToast(t("views.communityDetail.toasts.repostUpdateFailed"));
  } finally {
    actionBusy.value = null;
  }
}

async function confirmRepost(comment?: string | null) {
  const token = getAccessToken();
  const it = repostTarget.value;
  if (!token || !it) return;
  actionBusy.value = `rp-${it.id}`;
  try {
    const res = await toggleTimelineRepost(token, it, comment);
    patchItem(it.id, { reposted_by_me: res.reposted, repost_count: res.repost_count });
    repostModalOpen.value = false;
    repostTarget.value = null;
  } catch {
    showToast(t("views.communityDetail.toasts.repostFailed"));
  } finally {
    actionBusy.value = null;
  }
}

async function sharePost(it: TimelinePost) {
  const resolved = router.resolve({ path: postDetailPath(it.id) });
  const url = new URL(resolved.href, window.location.origin).href;
  try {
    await navigator.clipboard.writeText(url);
    showToast(t("views.communityDetail.toasts.linkCopied"));
  } catch {
    showToast(t("views.communityDetail.toasts.shareFailed"));
  }
}

watch(communityID, () => void load());
watch(activeTab, (tab) => {
  if (tab === "media") void loadMediaTiles();
});
watch(lightbox, (v) => {
  if (v) {
    document.body.style.overflow = "hidden";
    window.addEventListener("keydown", onLightboxKeydown);
  } else {
    document.body.style.overflow = "";
    window.removeEventListener("keydown", onLightboxKeydown);
  }
});
onMounted(() => {
  window.addEventListener("glipz:post-created", onSidebarPostCreated);
  void loadMeEmail();
  void load();
});
onUnmounted(() => {
  document.body.style.overflow = "";
  window.removeEventListener("keydown", onLightboxKeydown);
  window.removeEventListener("glipz:post-created", onSidebarPostCreated);
  if (toastTimer) clearTimeout(toastTimer);
});
</script>

<template>
  <div class="min-h-0 h-full w-full min-w-0 text-neutral-900">
  <PullToRefresh :on-refresh="refreshCommunity">
  <div class="border-b border-neutral-200">
    <div
      class="group relative h-36 overflow-hidden bg-gradient-to-br from-lime-200 via-lime-100 to-neutral-200 sm:h-48"
      :class="canManageCommunity ? 'cursor-pointer' : ''"
    >
      <img v-if="community?.header_url" :src="community.header_url" alt="" class="h-full w-full object-cover" />
      <template v-if="canManageCommunity">
        <input
          id="community-header-inline-file"
          type="file"
          accept="image/*"
          class="sr-only"
          :disabled="imageEditBusy !== null"
          @change="selectInlineCommunityImage('header', $event)"
        />
        <label
          for="community-header-inline-file"
          class="absolute inset-0 flex cursor-pointer items-center justify-center bg-black/0 transition-colors group-hover:bg-black/35"
        >
          <span class="inline-flex h-11 w-11 items-center justify-center rounded-full bg-black/50 text-white opacity-0 shadow-lg ring-1 ring-white/25 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
            <Icon name="camera" class="h-5 w-5" />
          </span>
        </label>
      </template>
    </div>
    <div class="px-4 pb-4">
      <div v-if="community" class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <div
            class="group relative -mt-10 mb-3 flex h-20 w-20 items-center justify-center overflow-hidden rounded-full bg-lime-100 text-2xl font-bold text-lime-800 ring-4 ring-white"
            :class="canManageCommunity ? 'cursor-pointer' : ''"
          >
            <img v-if="community.icon_url" :src="community.icon_url" alt="" class="h-full w-full object-cover" />
            <span v-else>{{ community.name.slice(0, 1) }}</span>
            <template v-if="canManageCommunity">
              <input
                id="community-icon-inline-file"
                type="file"
                accept="image/*"
                class="sr-only"
                :disabled="imageEditBusy !== null"
                @change="selectInlineCommunityImage('icon', $event)"
              />
              <label
                for="community-icon-inline-file"
                class="absolute inset-0 flex cursor-pointer items-center justify-center rounded-full bg-black/0 transition-colors group-hover:bg-black/35"
              >
                <span class="inline-flex h-9 w-9 items-center justify-center rounded-full bg-black/50 text-white opacity-0 shadow-lg ring-1 ring-white/25 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
                  <Icon name="camera" class="h-4 w-4" />
                </span>
              </label>
            </template>
          </div>
          <RouterLink to="/communities" class="text-sm font-medium text-lime-800 hover:underline">
            {{ $t("views.communities.backToList") }}
          </RouterLink>
          <h1 class="mt-2 truncate text-2xl font-bold text-neutral-900">{{ community.name }}</h1>
          <p class="mt-0.5 truncate font-mono text-xs text-neutral-500">{{ community.id }}</p>
          <p v-if="community.description" class="mt-3 whitespace-pre-wrap text-sm text-neutral-700">{{ community.description }}</p>
          <div class="mt-4 flex items-center gap-2.5">
            <div v-if="memberPreviews.length" class="flex -space-x-1.5">
              <div
                v-for="member in memberPreviews"
                :key="member.user_id"
                class="flex h-8 w-8 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-[11px] font-semibold text-neutral-700 ring-2 ring-white"
                :title="member.display_name || member.handle"
              >
                <img
                  v-if="safeMemberAvatarURL(member.avatar_url)"
                  :src="safeMemberAvatarURL(member.avatar_url)"
                  alt=""
                  referrerpolicy="no-referrer"
                  class="h-full w-full object-cover"
                />
                <span v-else>{{ avatarInitials(member.display_name || member.handle) }}</span>
              </div>
            </div>
            <p class="text-base font-normal leading-none text-neutral-800 sm:text-lg">
              {{ $t("views.communityDetail.memberCount", { count: community.approved_member_count }) }}
            </p>
          </div>
        </div>
        <div class="mt-4 shrink-0">
          <div v-if="canManageCommunity" class="mb-2 flex justify-end gap-2">
          <button
            type="button"
            class="rounded-full bg-neutral-100 px-3 py-1 text-xs font-semibold text-neutral-700 hover:bg-neutral-200"
            @click="openCommunityEdit"
          >
            {{ $t("views.communityDetail.editCommunity") }}
          </button>
          <button
            type="button"
            class="rounded-full bg-red-50 px-3 py-1 text-xs font-semibold text-red-700 hover:bg-red-100"
            @click="requestDeleteCommunity"
          >
            {{ $t("views.communityDetail.deleteCommunity") }}
          </button>
        </div>
        <button
          v-if="signedIn && community.viewer_status !== 'approved' && community.viewer_status !== 'pending'"
          type="button"
          class="rounded-full bg-lime-500 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-600 disabled:opacity-50"
          :disabled="actionBusy === 'join'"
          @click="join"
        >
          {{ $t("views.communityDetail.requestJoin") }}
        </button>
        <span v-else-if="community.viewer_status === 'pending'" class="rounded-full bg-amber-50 px-3 py-1 text-sm font-medium text-amber-800">
          {{ $t("views.communityDetail.pending") }}
        </span>
        <span v-else-if="community.viewer_status === 'approved'" class="rounded-full bg-lime-50 px-3 py-1 text-sm font-medium text-lime-800">
          {{ $t("views.communityDetail.member") }}
        </span>
        </div>
      </div>
    </div>
  </div>

  <div v-if="toast" class="fixed bottom-20 left-1/2 z-50 -translate-x-1/2 rounded-full bg-neutral-900 px-4 py-2 text-sm text-white shadow-lg">
    {{ toast }}
  </div>

  <p v-if="err" class="border-b border-neutral-200 px-4 py-3 text-sm text-red-600">{{ err }}</p>
  <p v-else-if="busy" class="border-b border-neutral-200 px-4 py-3 text-sm text-neutral-500">{{ $t("app.loading") }}</p>

  <section v-if="community && editingCommunity" class="border-b border-neutral-200 bg-neutral-50 px-4 py-4">
    <div class="space-y-3 rounded-2xl border border-neutral-200 bg-white p-4">
      <h2 class="text-sm font-semibold text-neutral-900">{{ $t("views.communityDetail.editCommunity") }}</h2>
      <input
        v-model="editCommunityName"
        maxlength="80"
        class="w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        :placeholder="$t('views.communityCreate.name')"
      />
      <textarea
        v-model="editCommunityDescription"
        maxlength="500"
        rows="4"
        class="w-full resize-none rounded-xl border border-neutral-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        :placeholder="$t('views.communityCreate.body')"
      />
      <textarea
        v-model="editCommunityDetails"
        maxlength="5000"
        rows="8"
        class="w-full resize-y rounded-xl border border-neutral-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        :placeholder="$t('views.communityDetail.detailsPlaceholder')"
      />
      <p class="text-xs text-neutral-500">{{ $t("views.communityDetail.imageEditHint") }}</p>
      <div class="flex justify-end gap-2">
        <button type="button" class="rounded-full px-4 py-1.5 text-sm font-semibold text-neutral-700 hover:bg-neutral-100" @click="editingCommunity = false">
          {{ $t("views.compose.cancel") }}
        </button>
        <button
          type="button"
          class="rounded-full bg-lime-500 px-4 py-1.5 text-sm font-semibold text-white hover:bg-lime-600 disabled:opacity-50"
          :disabled="actionBusy === 'community-edit'"
          @click="submitCommunityEdit"
        >
          {{ $t("views.communityDetail.saveCommunity") }}
        </button>
      </div>
    </div>
  </section>

  <section v-if="community && isOwner && joinRequests.length" class="border-b border-neutral-200 px-4 py-4">
    <h2 class="text-sm font-semibold text-neutral-900">{{ $t("views.communityDetail.joinRequests") }}</h2>
    <div class="mt-3 space-y-2">
      <div v-for="req in joinRequests" :key="req.user_id" class="flex items-center justify-between gap-3 rounded-xl border border-neutral-200 px-3 py-2">
        <div class="min-w-0">
          <div class="truncate text-sm font-medium text-neutral-900">{{ req.display_name || req.handle }}</div>
          <div class="text-xs text-neutral-500">@{{ req.handle }}</div>
        </div>
        <div class="flex gap-2">
          <button type="button" class="rounded-full bg-lime-500 px-3 py-1 text-xs font-semibold text-white" @click="reviewRequest(req.user_id, true)">
            {{ $t("views.communityDetail.approve") }}
          </button>
          <button type="button" class="rounded-full bg-neutral-100 px-3 py-1 text-xs font-semibold text-neutral-700" @click="reviewRequest(req.user_id, false)">
            {{ $t("views.communityDetail.reject") }}
          </button>
        </div>
      </div>
    </div>
  </section>

  <nav v-if="community" class="border-b border-neutral-200 px-4" :aria-label="$t('views.communityDetail.tabsLabel')">
    <div class="flex gap-4 overflow-x-auto">
      <button
        v-for="tab in communityTabs"
        :key="tab.key"
        type="button"
        class="relative shrink-0 -mb-px border-b-2 px-1 py-3 text-sm font-semibold transition-colors"
        :class="
          activeTab === tab.key
            ? 'border-lime-600 text-neutral-900'
            : 'border-transparent text-neutral-500 hover:text-neutral-800'
        "
        :aria-current="activeTab === tab.key ? 'page' : undefined"
        @click="activeTab = tab.key"
      >
        {{ tab.label }}
      </button>
    </div>
  </nav>

  <section v-if="community && activeTab === 'details'" class="border-b border-neutral-200 px-4 py-5">
    <div class="rounded-2xl border border-neutral-200 bg-white p-4">
      <div class="flex items-start justify-between gap-3">
        <div>
          <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.communityDetail.detailsTitle") }}</h2>
          <p class="mt-1 text-sm text-neutral-500">{{ $t("views.communityDetail.detailsDescription") }}</p>
        </div>
        <button
          v-if="canManageCommunity"
          type="button"
          class="shrink-0 rounded-full bg-neutral-100 px-3 py-1.5 text-xs font-semibold text-neutral-700 hover:bg-neutral-200"
          @click="openCommunityEdit"
        >
          {{ $t("views.communityDetail.editCommunity") }}
        </button>
      </div>
      <p v-if="community.details" class="mt-4 whitespace-pre-wrap text-sm leading-6 text-neutral-800">{{ community.details }}</p>
      <p v-else class="mt-4 rounded-xl bg-neutral-50 px-4 py-6 text-center text-sm text-neutral-500">
        {{ $t(canManageCommunity ? "views.communityDetail.emptyDetailsOwner" : "views.communityDetail.emptyDetails") }}
      </p>
    </div>
  </section>

  <template v-if="community && activeTab === 'media'">
    <p v-if="mediaTilesBusy" class="border-b border-neutral-200 px-4 py-10 text-center text-sm text-neutral-500">
      {{ $t("app.loading") }}
    </p>
    <p v-else-if="mediaTilesErr" class="border-b border-neutral-200 px-4 py-8 text-center text-sm text-red-600">
      {{ mediaTilesErr }}
    </p>
    <p v-else-if="!mediaTiles.length" class="border-b border-neutral-200 px-4 py-10 text-center text-sm text-neutral-500">
      {{ $t("views.communityDetail.emptyMedia") }}
    </p>
    <div v-else class="grid grid-cols-3 gap-px border-b border-neutral-200 bg-neutral-200 sm:gap-0.5">
      <RouterLink
        v-for="tile in mediaTiles"
        :key="tile.post_id"
        :to="postDetailPath(tile.post_id)"
        class="relative aspect-square overflow-hidden bg-neutral-100 outline-none ring-inset ring-lime-500/0 transition hover:opacity-95 focus-visible:ring-2"
      >
        <img
          v-if="tile.media_type === 'image' && tile.preview_url"
          :src="tile.preview_url"
          alt=""
          class="h-full w-full object-cover"
          loading="lazy"
        />
        <video
          v-else-if="tile.media_type === 'video' && tile.preview_url"
          :src="tile.preview_url"
          muted
          playsinline
          preload="metadata"
          class="h-full w-full object-cover"
        />
        <div
          v-else-if="tile.media_type === 'audio' && tile.preview_url"
          class="flex h-full w-full flex-col items-center justify-center gap-1 bg-neutral-800 px-1 text-lime-400"
        >
          <Icon name="note" class="h-8 w-8 opacity-90" :stroke-width="1.5" />
          <span class="text-[10px] font-medium uppercase tracking-wide text-lime-300/90">{{
            $t("views.userProfile.mediaTileAudio")
          }}</span>
        </div>
        <div
          v-else
          class="flex h-full w-full flex-col items-center justify-center gap-1 bg-neutral-200 px-1 text-center text-xs text-neutral-600"
        >
          <Icon name="lock" class="h-6 w-6 text-neutral-500" :stroke-width="1.5" />
          <span v-if="tile.locked">{{ $t("views.userProfile.mediaLocked") }}</span>
          <span v-else>—</span>
        </div>
      </RouterLink>
    </div>
  </template>

  <p v-if="!busy && timelineTabActive && visiblePosts.length === 0 && !err" class="border-b border-neutral-200 px-4 py-16 text-center text-neutral-500">
    {{ emptyTabMessage }}
  </p>

  <PostTimeline
    v-if="timelineTabActive"
    :items="visiblePosts"
    :thread-replies-by-root="threadRepliesByRoot"
    :action-busy="actionBusy"
    :viewer-email="myEmail"
    show-federated-reply-action
    @reply="reply"
    @toggle-reaction="toggleReaction"
    @toggle-bookmark="toggleBookmark"
    @toggle-repost="onToggleRepost"
    @share="sharePost"
    @open-lightbox="openLightbox"
    @patch-item="({ id, patch }) => patchItem(id, patch)"
    @remove-post="removePost"
  />
  </PullToRefresh>

  <RepostModal
    :open="repostModalOpen"
    :post="repostTarget"
    @update:open="(v) => (repostModalOpen = v)"
    @plain="confirmRepost(null)"
    @with-comment="confirmRepost"
  />
  </div>
  <Teleport to="body">
    <div
      v-if="lightbox"
      class="fixed inset-0 z-[100] flex flex-col"
      role="dialog"
      aria-modal="true"
      :aria-label="$t('views.feed.lightboxTitle')"
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
            :aria-label="$t('views.feed.lightboxClose')"
            @click="closeLightbox"
          >
            <Icon name="close" class="h-6 w-6" :stroke-width="2" />
          </button>
        </div>
        <div class="relative flex min-h-0 flex-1 items-stretch justify-center px-0 pb-4 sm:px-2">
          <button
            v-if="lightbox.urls.length > 1"
            type="button"
            class="z-20 hidden w-10 shrink-0 items-center justify-center self-center rounded-r-md text-3xl font-light text-white/90 hover:bg-white/10 sm:flex"
            :aria-label="$t('views.feed.lightboxPrev')"
            @click="lightboxPrev"
          >
            ‹
          </button>
          <div
            class="relative min-w-0 flex-1 overflow-hidden"
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
                  :alt="$t('views.feed.lightboxImageAlt', { n: li + 1 })"
                  class="max-h-[min(88vh,100%)] max-w-full select-none object-contain"
                  draggable="false"
                />
              </div>
            </div>
          </div>
          <button
            v-if="lightbox.urls.length > 1"
            type="button"
            class="z-20 hidden w-10 shrink-0 items-center justify-center self-center rounded-l-md text-3xl font-light text-white/90 hover:bg-white/10 sm:flex"
            :aria-label="$t('views.feed.lightboxNext')"
            @click="lightboxNext"
          >
            ›
          </button>
        </div>
        <p v-if="lightbox.urls.length > 1" class="pb-3 text-center text-xs text-white/60 sm:hidden">
          {{ $t("views.feed.lightboxSwipeHint") }}
        </p>
      </div>
    </div>
  </Teleport>
</template>
