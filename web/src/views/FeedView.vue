<script setup lang="ts">
import { computed, nextTick, onActivated, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import { api, uploadMediaFile } from "../lib/api";
import {
  connectFeedStream,
  fetchFederatedIncomingFeedItem,
  fetchFederatedThreadReplies,
  fetchFeedItem,
  fetchPostThreadReplies,
  mapFeedItem,
  type FeedPubPayload,
} from "../lib/feedStream";
import Icon from "../components/Icon.vue";
import ComposerEmojiPicker from "../components/ComposerEmojiPicker.vue";
import GlipzAudioPlayer from "../components/GlipzAudioPlayer.vue";
import GlipzVideoPlayer from "../components/GlipzVideoPlayer.vue";
import PostTimeline from "../components/PostTimeline.vue";
import RepostModal from "../components/RepostModal.vue";
import { addTimelineReaction, removeTimelineReaction, toggleTimelineBookmark, toggleTimelineRepost } from "../lib/federationActions";
import type { TimelinePost, ViewPasswordTextRange } from "../types/timeline";
import { avatarInitials, handleAt, postDetailPath } from "../lib/feedDisplay";
import { buildComposerReplyQuery, composeRoutePath } from "../lib/postComposer";
import {
  buildViewPasswordScope,
  codeUnitOffsetToRuneIndex,
  normalizeViewPasswordRanges,
  sliceRunes,
} from "../lib/viewPassword";
import PullToRefresh from "../components/PullToRefresh.vue";
import { patreonSettingsPath, usePatreonComposer } from "../composables/usePatreonComposer";
import {
  composerAttachmentLabel,
  inferPostMediaType,
  MAX_COMPOSER_IMAGE_SLOTS,
  mergePickedComposerFiles,
} from "../lib/composerMedia";

const MAX_IMAGES = MAX_COMPOSER_IMAGE_SLOTS;

type ReplyingTo = { id: string; user_email: string; user_handle: string; is_federated?: boolean; remote_object_url?: string };

type LightboxState = { urls: string[]; index: number };
const lightbox = ref<LightboxState | null>(null);
const isNearFeedBottom = ref(false);
let lightboxTouchStartX = 0;

function openLightbox(urls: string[], startIndex: number) {
  if (!urls.length) return;
  const i = Math.max(0, Math.min(startIndex, urls.length - 1));
  lightbox.value = { urls, index: i };
}

function closeLightbox() {
  lightbox.value = null;
}

function updateFeedFabVisibility() {
  if (typeof window === "undefined") return;
  const scrollTop = window.scrollY || document.documentElement.scrollTop || 0;
  const viewportHeight = window.innerHeight || document.documentElement.clientHeight || 0;
  const documentHeight = document.documentElement.scrollHeight || document.body.scrollHeight || 0;
  isNearFeedBottom.value = documentHeight - (scrollTop + viewportHeight) <= 220;
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

type FeedScope = "all" | "following" | "recommended";
const feedScope = ref<FeedScope>("all");

const items = ref<TimelinePost[]>([]);
/** Maps root post IDs to flat reply lists used for thread rendering. */
const threadRepliesByRoot = ref<Record<string, TimelinePost[]>>({});
const err = ref("");
const busy = ref(false);
const caption = ref("");
const composerCaptionEl = ref<HTMLTextAreaElement | null>(null);
const myEmail = ref<string | null>(null);
const myHandle = ref<string | null>(null);
const myAvatarUrl = ref<string | null>(null);
const composerAvatarImgFailed = ref(false);
const selectedImages = ref<File[]>([]);
const previewUrls = ref<string[]>([]);
const replyingTo = ref<ReplyingTo | null>(null);
const actionBusy = ref<string | null>(null);
const actionToast = ref("");
const repostModalOpen = ref(false);
const repostTarget = ref<TimelinePost | null>(null);
let toastTimer: ReturnType<typeof setTimeout> | null = null;
let disconnectFeedStream: (() => void) | null = null;
type ComposerVisibility = "public" | "logged_in" | "followers" | "private";

const visibilityOptions = computed<Array<{ value: ComposerVisibility; label: string; description: string }>>(() => [
  { value: "public", label: t("views.compose.visibility.public.label"), description: t("views.compose.visibility.public.description") },
  { value: "logged_in", label: t("views.compose.visibility.loggedIn.label"), description: t("views.compose.visibility.loggedIn.description") },
  { value: "followers", label: t("views.compose.visibility.followers.label"), description: t("views.compose.visibility.followers.description") },
  { value: "private", label: t("views.compose.visibility.private.label"), description: t("views.compose.visibility.private.description") },
]);

const isNsfw = ref(false);
const composerVisibility = ref<ComposerVisibility>("public");
const viewPassword = ref("");
const viewPasswordConfirm = ref("");
const composerProtectText = ref(false);
const composerProtectMedia = ref(false);
const composerProtectAll = ref(false);
const composerTextRanges = ref<ViewPasswordTextRange[]>([]);
/** Whether the NSFW settings panel is open. */
const composerNsfwOpen = ref(false);
/** Whether the view-password panel is open. */
const composerPasswordOpen = ref(false);
/** Whether the composer includes a poll. */
const composerPollOpen = ref(false);
const pollOptionInputs = ref(["", ""]);
const pollDurationHours = ref(24);
/** `datetime-local` value used for scheduled publishing. */
const scheduleLocal = ref("");
/** Whether the scheduled-publish panel is open. */
const composerScheduleOpen = ref(false);
/** Whether the visibility settings panel is open. */
const composerVisibilityOpen = ref(false);

const attachmentKind = computed(() =>
  selectedImages.value.length ? inferPostMediaType(selectedImages.value) : "none",
);
const attachmentPickerDisabled = computed(
  () =>
    busy.value ||
    attachmentKind.value === "video" ||
    attachmentKind.value === "audio" ||
    selectedImages.value.length >= MAX_IMAGES,
);

const {
  patreonAvailable,
  patreonConnected,
  patreonCampaigns,
  composerMembershipOpen,
  membershipUsePatreon,
  membershipProvider,
  membershipCampaignId,
  membershipTierId,
  patreonConnectBusy,
  membershipTierOptions,
  loadPatreon,
  resetPatreonComposerState,
  validateMembershipForSubmit,
  applyMembershipToBody,
  connectPatreonOAuth,
} = usePatreonComposer({ viewPassword, viewPasswordConfirm, composerPasswordOpen });

const patreonSettingsHref = patreonSettingsPath;

/** Checks whether the composer has enough content to submit. */
function hasComposerContent(): boolean {
  if (selectedImages.value.length > 0) return true;
  if (caption.value.trim().length > 0) return true;
  if (composerPollOpen.value) {
    const pollOpts = pollOptionInputs.value.map((s) => s.trim()).filter(Boolean);
    if (pollOpts.length >= 2) return true;
  }
  return false;
}

function showToast(msg: string) {
  if (toastTimer) clearTimeout(toastTimer);
  actionToast.value = msg;
  toastTimer = setTimeout(() => {
    actionToast.value = "";
    toastTimer = null;
  }, 2200);
}

function patchItem(id: string, patch: Partial<TimelinePost>) {
  items.value = items.value.map((x) => (x.id === id ? { ...x, ...patch } : x));
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

function localPostIdFromObjectUrl(raw: string | undefined): string | null {
  const s = (raw ?? "").trim();
  const m = s.match(/\/posts\/([0-9a-fA-F-]{36})$/);
  return m?.[1] ?? null;
}

async function refreshThreadForRoot(rootId: string) {
  const token = getAccessToken();
  if (!token) return;
  const root = items.value.find((x) => x.id === rootId);
  if (!root) return;
  const list = root.is_federated
    ? await fetchFederatedThreadReplies(rootId.replace(/^federated:/, ""), token)
    : await fetchPostThreadReplies(rootId, token);
  const tr = { ...threadRepliesByRoot.value };
  if (list.length) tr[rootId] = list;
  else delete tr[rootId];
  threadRepliesByRoot.value = tr;
}

async function loadThreadsForFeed() {
  const token = getAccessToken();
  if (!token) return;
  const withReplies = items.value.filter((x) => x.reply_count > 0 || x.is_federated);
  const next: Record<string, TimelinePost[]> = {};
  await Promise.all(
    withReplies.map(async (it) => {
      const list = it.is_federated
        ? await fetchFederatedThreadReplies(it.id.replace(/^federated:/, ""), token)
        : await fetchPostThreadReplies(it.id, token);
      if (list.length) next[it.id] = list;
    }),
  );
  threadRepliesByRoot.value = next;
}

async function removePost(id: string) {
  const affectedRoots = Object.entries(threadRepliesByRoot.value)
    .filter(([, list]) => list.some((x) => x.id === id))
    .map(([rootId]) => rootId);
  items.value = items.value.filter((x) => x.id !== id);
  const tr = { ...threadRepliesByRoot.value };
  delete tr[id];
  threadRepliesByRoot.value = tr;
  for (const rid of affectedRoots) {
    if (rid !== id && items.value.some((x) => x.id === rid)) {
      await refreshThreadForRoot(rid);
    }
  }
}

function stopFeedStream() {
  disconnectFeedStream?.();
  disconnectFeedStream = null;
}

function startFeedStream() {
  stopFeedStream();
  const token = getAccessToken();
  if (!token) return;
  if (feedScope.value === "recommended") return;
  disconnectFeedStream = connectFeedStream({
    scope: feedScope.value,
    token,
    onPayload: (p: FeedPubPayload) => void handleFeedPub(p),
  });
}

async function handleFeedPub(p: FeedPubPayload) {
  if (p.kind === "post_deleted") {
    removePost(p.post_id);
    return;
  }
  if (p.kind === "federated_post_deleted") {
    removePost(`federated:${p.incoming_id}`);
    return;
  }
  const token = getAccessToken();
  if (!token) return;
  if (p.kind === "post_created" || p.kind === "post_updated") {
    if (p.kind === "post_created" && items.value.some((x) => x.id === p.post_id)) return;
    const row = await fetchFeedItem(p.post_id, token);
    if (!row) {
      if (p.kind === "post_updated") {
        await removePost(p.post_id);
      }
      return;
    }
    if (p.kind === "post_updated") {
      const idx = items.value.findIndex((x) => x.id === row.id);
      if (idx >= 0) {
        const next = items.value.slice();
        next[idx] = row;
        items.value = next;
      }
      return;
    }
    items.value = [row, ...items.value].slice(0, 80);
    return;
  }
  if (p.kind !== "federated_post_upsert") return;
  const row = await fetchFederatedIncomingFeedItem(p.incoming_id, token);
  if (!row) return;
  if (row.reply_to_object_url) {
    const root = items.value.find((x) => x.remote_object_url === row.reply_to_object_url);
    if (root) {
      await refreshThreadForRoot(root.id);
      return;
    }
    const localRootId = localPostIdFromObjectUrl(row.reply_to_object_url);
    if (localRootId) {
      const rootRow = await fetchFeedItem(localRootId, token);
      if (rootRow) {
        const idx = items.value.findIndex((x) => x.id === localRootId);
        if (idx >= 0) {
          const next = items.value.slice();
          next[idx] = rootRow;
          items.value = next;
        }
        await refreshThreadForRoot(localRootId);
        return;
      }
    }
  }
  const idx = items.value.findIndex((x) => x.id === row.id);
  if (idx >= 0) {
    const next = items.value.slice();
    next[idx] = row;
    items.value = next;
    return;
  }
  items.value = [row, ...items.value].slice(0, 80);
}

watch(
  selectedImages,
  (files) => {
    previewUrls.value.forEach((u) => URL.revokeObjectURL(u));
    previewUrls.value = files.map((f) => URL.createObjectURL(f));
  },
  { deep: true },
);

onBeforeUnmount(() => {
  stopFeedStream();
  previewUrls.value.forEach((u) => URL.revokeObjectURL(u));
  window.removeEventListener("keydown", onLightboxKeydown);
  window.removeEventListener("scroll", updateFeedFabVisibility);
  window.removeEventListener("resize", updateFeedFabVisibility);
  document.body.style.overflow = "";
  if (toastTimer) clearTimeout(toastTimer);
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

async function loadMe() {
  const token = getAccessToken();
  if (!token) return;
  try {
    const u = await api<{ email: string; handle?: string; avatar_url?: string | null }>("/api/v1/me", { method: "GET", token });
    myEmail.value = u.email;
    myHandle.value = typeof u.handle === "string" ? u.handle : null;
    myAvatarUrl.value = u.avatar_url && String(u.avatar_url).trim() !== "" ? String(u.avatar_url) : null;
    composerAvatarImgFailed.value = false;
    void loadPatreon(token);
  } catch {
    myEmail.value = null;
    myHandle.value = null;
    myAvatarUrl.value = null;
    composerAvatarImgFailed.value = false;
  }
}

async function setFeedScope(s: FeedScope) {
  if (feedScope.value === s) return;
  stopFeedStream();
  feedScope.value = s;
  await load();
  startFeedStream();
}

async function load() {
  const token = getAccessToken();
  if (!token) {
    await router.replace("/login");
    return;
  }
  err.value = "";
  try {
    const path =
      feedScope.value === "following"
        ? "/api/v1/posts/feed?scope=following"
        : feedScope.value === "recommended"
          ? "/api/v1/posts/feed?scope=recommended"
          : "/api/v1/posts/feed";
    const res = await api<{ items: TimelinePost[] }>(path, {
      method: "GET",
      token,
    });
    items.value = res.items.map((x) => mapFeedItem(x as Parameters<typeof mapFeedItem>[0]));
    await loadThreadsForFeed();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.feed.loadFailed");
    threadRepliesByRoot.value = {};
  }
}

async function refreshFeed() {
  await load();
}

function cancelReply() {
  replyingTo.value = null;
}

function startReply(it: TimelinePost) {
  if (composeRoutePath() === "/compose") {
    void router.push({
      path: "/compose",
      query: buildComposerReplyQuery(it),
    });
    return;
  }
  replyingTo.value = {
    id: it.id,
    user_email: it.user_email,
    user_handle: it.user_handle ?? "",
    is_federated: Boolean(it.is_federated),
    remote_object_url: it.remote_object_url,
  };
  void nextTick(() => {
    document.querySelector(".composer-anchor")?.scrollIntoView({ behavior: "smooth", block: "center" });
    const ta = document.querySelector<HTMLTextAreaElement>(".composer-anchor textarea");
    ta?.focus();
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
    showToast(
      msg === "repost_comment_too_long" ? t("views.feed.toasts.repostCommentTooLong") : t("views.feed.toasts.repostFailed"),
    );
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
    showToast(
      msg === "repost_comment_too_long" ? t("views.feed.toasts.repostCommentTooLong") : t("views.feed.toasts.repostFailed"),
    );
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
        title: t("app.name"),
        text: it.caption?.slice(0, 80) ?? t("views.search.shareFallbackText"),
        url,
      });
      return;
    } catch (e: unknown) {
      if (e instanceof DOMException && e.name === "AbortError") return;
    }
  }
  try {
    await navigator.clipboard.writeText(url);
    showToast(t("views.feed.toasts.linkCopied"));
  } catch {
    try {
      const ta = document.createElement("textarea");
      ta.value = url;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
      showToast(t("views.feed.toasts.linkCopied"));
    } catch {
      showToast(t("views.feed.toasts.shareFailed"));
    }
  }
}

onActivated(() => {
  const tok = getAccessToken();
  if (tok) void loadPatreon(tok);
});

onMounted(() => {
  window.addEventListener("scroll", updateFeedFabVisibility, { passive: true });
  window.addEventListener("resize", updateFeedFabVisibility);
  void loadMe();
  void load().then(() => {
    startFeedStream();
    updateFeedFabVisibility();
    const h = router.currentRoute.value.hash;
    if (h?.startsWith("#post-")) {
      void nextTick(() => document.querySelector(h)?.scrollIntoView({ behavior: "smooth", block: "center" }));
    }
    const q = route.query;
    const rid = typeof q.reply === "string" ? q.reply : Array.isArray(q.reply) ? q.reply[0] : "";
    const rh = typeof q.rh === "string" ? q.rh : Array.isArray(q.rh) ? q.rh[0] : "";
    const rf = typeof q.rf === "string" ? q.rf : Array.isArray(q.rf) ? q.rf[0] : "";
    const rou = typeof q.rou === "string" ? q.rou : Array.isArray(q.rou) ? q.rou[0] : "";
    if (rid) {
      if (composeRoutePath() === "/compose") {
        void router.replace({ path: "/compose", query: route.query });
        return;
      }
      replyingTo.value = {
        id: rid,
        user_email: "",
        user_handle: rh ?? "",
        is_federated: rf === "1",
        remote_object_url: rou || undefined,
      };
      void router.replace({ path: "/feed", query: {} });
    }
  });
});

function onFilesSelect(e: Event) {
  const input = e.target as HTMLInputElement;
  const incoming = Array.from(input.files ?? []);
  input.value = "";
  if (!incoming.length) return;

  const { files, replacedKind, partialImageDrop, excludedImages } = mergePickedComposerFiles(
    selectedImages.value,
    incoming,
  );
  selectedImages.value = files;
  if (replacedKind) {
    err.value = t("views.compose.errors.attachmentsReplaced");
  } else if (partialImageDrop) {
    err.value = t("views.compose.errors.maxImagesPartial", {
      max: MAX_IMAGES,
      excluded: excludedImages,
    });
  } else {
    err.value = "";
  }
}

function removeImage(i: number) {
  selectedImages.value = selectedImages.value.filter((_, j) => j !== i);
  err.value = "";
}

function composerViewPasswordScope(): number {
  return buildViewPasswordScope({
    protectAll: composerProtectAll.value,
    protectText: composerProtectText.value,
    protectMedia: composerProtectMedia.value,
  });
}

function toggleComposerProtectAll() {
  composerProtectAll.value = !composerProtectAll.value;
  if (composerProtectAll.value) {
    composerProtectText.value = false;
    composerProtectMedia.value = false;
    composerTextRanges.value = [];
  }
}

function toggleComposerProtectText() {
  composerProtectText.value = !composerProtectText.value;
  if (composerProtectText.value) {
    composerProtectAll.value = false;
  } else {
    composerTextRanges.value = [];
  }
}

function toggleComposerProtectMedia() {
  composerProtectMedia.value = !composerProtectMedia.value;
  if (composerProtectMedia.value) {
    composerProtectAll.value = false;
  }
}

function addComposerSelectedTextRange() {
  const el = composerCaptionEl.value;
  if (!el) {
    err.value = t("views.compose.errors.selectTextFromComposer");
    return;
  }
  const startOffset = Math.min(el.selectionStart ?? 0, el.selectionEnd ?? 0);
  const endOffset = Math.max(el.selectionStart ?? 0, el.selectionEnd ?? 0);
  if (startOffset === endOffset) {
    err.value = t("views.compose.errors.selectTextRangeComposer");
    return;
  }
  composerProtectText.value = true;
  composerProtectAll.value = false;
  composerTextRanges.value = normalizeViewPasswordRanges([
    ...composerTextRanges.value,
    {
      start: codeUnitOffsetToRuneIndex(caption.value, startOffset),
      end: codeUnitOffsetToRuneIndex(caption.value, endOffset),
    },
  ]);
  err.value = "";
}

function removeComposerTextRange(idx: number) {
  composerTextRanges.value = composerTextRanges.value.filter((_, i) => i !== idx);
}

function composerTextRangePreview(rg: ViewPasswordTextRange): string {
  const text = sliceRunes(caption.value, rg.start, rg.end).trim();
  return text || t("views.compose.whitespaceOnly");
}

function insertComposerEmoji(emoji: string) {
  const el = composerCaptionEl.value;
  if (!el) {
    caption.value += emoji;
    return;
  }
  const start = el.selectionStart ?? caption.value.length;
  const end = el.selectionEnd ?? start;
  caption.value = `${caption.value.slice(0, start)}${emoji}${caption.value.slice(end)}`;
  void nextTick(() => {
    const pos = start + emoji.length;
    el.focus();
    el.setSelectionRange(pos, pos);
  });
}

function composerVisibilityMeta(value: ComposerVisibility) {
  return visibilityOptions.value.find((option) => option.value === value) ?? visibilityOptions.value[0];
}

async function connectPatreonFromFeed() {
  err.value = "";
  const r = await connectPatreonOAuth("/feed");
  if (r.error) {
    err.value = r.error === "patreon_oauth_failed" ? t("views.compose.errors.postFailed") : r.error;
  }
}

async function submitPost() {
  if (!hasComposerContent()) {
    err.value = t("views.compose.errors.composerNeedsContent");
    return;
  }
  const token = getAccessToken();
  if (!token) return;

  const pw = viewPassword.value.trim();
  const pw2 = viewPasswordConfirm.value.trim();
  const pwScope = composerViewPasswordScope();
  const memErr = validateMembershipForSubmit(pw, pw2, t);
  if (memErr) {
    err.value = memErr;
    return;
  }
  if (pw || pw2) {
    if (pw !== pw2) {
      err.value = t("views.compose.errors.viewPasswordMismatch");
      return;
    }
    if (pw.length < 4 || pw.length > 72) {
      err.value = t("views.compose.errors.viewPasswordLength");
      return;
    }
    if (pwScope === 0) {
      err.value = t("views.compose.errors.viewPasswordScopeRequired");
      return;
    }
    if (composerProtectText.value && !composerProtectAll.value && composerTextRanges.value.length === 0) {
      err.value = t("views.compose.errors.viewPasswordRangesRequired");
      return;
    }
  }

  const pollOpts = pollOptionInputs.value.map((s) => s.trim()).filter(Boolean);
  if (composerPollOpen.value && pollOpts.length < 2) {
    err.value = t("views.compose.errors.pollMinOptions");
    return;
  }

  let visibleIso: string | undefined;
  if (scheduleLocal.value.trim()) {
    const d = new Date(scheduleLocal.value);
    if (Number.isNaN(d.getTime())) {
      err.value = t("views.compose.errors.scheduleInvalid");
      return;
    }
    visibleIso = d.toISOString();
  }

  if (composerPollOpen.value && pollOpts.length >= 2) {
    const baseMs = visibleIso ? new Date(visibleIso).getTime() : Date.now();
    const ends = new Date(baseMs + pollDurationHours.value * 3600000);
    if (ends.getTime() <= baseMs) {
      err.value = t("views.compose.errors.pollEndsTooSoon");
      return;
    }
  }

  busy.value = true;
  err.value = "";
  const capText = caption.value;
  try {
    const objectKeys: string[] = [];
    for (const file of selectedImages.value) {
      const up = await uploadMediaFile(token, file);
      objectKeys.push(up.object_key);
    }
    const body: Record<string, unknown> = {
      caption: capText,
      media_type: objectKeys.length ? inferPostMediaType(selectedImages.value) : "none",
      object_keys: objectKeys,
      is_nsfw: isNsfw.value,
      visibility: composerVisibility.value,
    };
    if (visibleIso) {
      body.visible_at = visibleIso;
    }
    if (composerPollOpen.value && pollOpts.length >= 2) {
      const baseMs = visibleIso ? new Date(visibleIso).getTime() : Date.now();
      body.poll = {
        options: pollOpts,
        ends_at: new Date(baseMs + pollDurationHours.value * 3600000).toISOString(),
      };
    }
    if (pw) {
      body.view_password = pw;
      body.view_password_scope = pwScope;
      if (composerProtectText.value && !composerProtectAll.value) {
        body.view_password_text_ranges = composerTextRanges.value;
      }
    }
    applyMembershipToBody(body);
    if (replyingTo.value) {
      if (replyingTo.value.is_federated) {
        const incomingId = replyingTo.value.id.startsWith("federated:")
          ? replyingTo.value.id.slice("federated:".length)
          : replyingTo.value.id;
        body.reply_to_incoming_id = incomingId;
        if (replyingTo.value.remote_object_url) {
          body.reply_to_object_url = replyingTo.value.remote_object_url;
        }
      } else {
        body.reply_to_post_id = replyingTo.value.id;
      }
    }
    await api("/api/v1/posts", {
      method: "POST",
      token,
      json: body,
    });
    caption.value = "";
    selectedImages.value = [];
    replyingTo.value = null;
    isNsfw.value = false;
    composerVisibility.value = "public";
    viewPassword.value = "";
    viewPasswordConfirm.value = "";
    composerProtectText.value = false;
    composerProtectMedia.value = false;
    composerProtectAll.value = false;
    composerTextRanges.value = [];
    composerNsfwOpen.value = false;
    composerPasswordOpen.value = false;
    composerPollOpen.value = false;
    pollOptionInputs.value = ["", ""];
    pollDurationHours.value = 24;
    scheduleLocal.value = "";
    composerScheduleOpen.value = false;
    composerVisibilityOpen.value = false;
    resetPatreonComposerState();
    await load();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.compose.errors.postFailed");
    await load();
  } finally {
    busy.value = false;
  }
}

function addPollOptionField() {
  if (pollOptionInputs.value.length >= 4) return;
  pollOptionInputs.value = [...pollOptionInputs.value, ""];
}
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="grid grid-cols-3">
      <button
        type="button"
        class="relative border-b-2 py-3 text-base font-semibold transition-colors"
        :class="
          feedScope === 'all'
            ? 'border-lime-600 text-neutral-900'
            : 'border-transparent text-neutral-500 hover:text-neutral-800'
        "
        @click="setFeedScope('all')"
      >
        {{ $t("views.feed.scopeAll") }}
      </button>
      <button
        type="button"
        class="relative border-b-2 py-3 text-base font-semibold transition-colors"
        :class="
          feedScope === 'recommended'
            ? 'border-lime-600 text-neutral-900'
            : 'border-transparent text-neutral-500 hover:text-neutral-800'
        "
        @click="setFeedScope('recommended')"
      >
        {{ $t("views.feed.scopeRecommended") }}
      </button>
      <button
        type="button"
        class="relative border-b-2 py-3 text-base font-semibold transition-colors"
        :class="
          feedScope === 'following'
            ? 'border-lime-600 text-neutral-900'
            : 'border-transparent text-neutral-500 hover:text-neutral-800'
        "
        @click="setFeedScope('following')"
      >
        {{ $t("views.feed.scopeFollowing") }}
      </button>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="grid grid-cols-3">
      <button
        type="button"
        class="relative border-b-2 py-3 text-base font-semibold transition-colors"
        :class="
          feedScope === 'all'
            ? 'border-lime-600 text-neutral-900'
            : 'border-transparent text-neutral-500 hover:text-neutral-800'
        "
        @click="setFeedScope('all')"
      >
        {{ $t("views.feed.scopeAll") }}
      </button>
      <button
        type="button"
        class="relative border-b-2 py-3 text-base font-semibold transition-colors"
        :class="
          feedScope === 'recommended'
            ? 'border-lime-600 text-neutral-900'
            : 'border-transparent text-neutral-500 hover:text-neutral-800'
        "
        @click="setFeedScope('recommended')"
      >
        {{ $t("views.feed.scopeRecommended") }}
      </button>
      <button
        type="button"
        class="relative border-b-2 py-3 text-base font-semibold transition-colors"
        :class="
          feedScope === 'following'
            ? 'border-lime-600 text-neutral-900'
            : 'border-transparent text-neutral-500 hover:text-neutral-800'
        "
        @click="setFeedScope('following')"
      >
        {{ $t("views.feed.scopeFollowing") }}
      </button>
    </div>
  </Teleport>
  <PullToRefresh :on-refresh="refreshFeed">
    <div class="composer-anchor hidden gap-3 border-b border-neutral-200 px-4 py-3 lg:flex">
      <div
        class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-lime-500 text-xs font-bold text-white"
        aria-hidden="true"
      >
        <img
          v-if="myAvatarUrl && !composerAvatarImgFailed"
          :src="myAvatarUrl"
          alt=""
          class="h-full w-full object-cover"
          @error="composerAvatarImgFailed = true"
        />
        <span v-else>{{ myEmail ? avatarInitials(myEmail) : "?" }}</span>
      </div>
      <div class="min-w-0 flex-1">
        <div
          v-if="replyingTo"
          class="mb-2 flex items-center justify-between gap-2 rounded-lg border border-lime-200 bg-lime-50/80 px-3 py-2 text-sm text-neutral-800"
        >
          <span class="min-w-0 truncate">
            <span class="text-neutral-500">{{ $t("views.compose.replyTo") }}</span>
            {{ handleAt(replyingTo) }}
          </span>
          <button
            type="button"
            class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium text-lime-800 hover:bg-lime-100"
            @click="cancelReply"
          >
            {{ $t("views.compose.cancel") }}
          </button>
        </div>
        <label class="sr-only">{{ $t("views.compose.caption") }}</label>
        <textarea
          ref="composerCaptionEl"
          v-model="caption"
          rows="3"
          :placeholder="replyingTo ? $t('views.compose.replyPlaceholder', { handle: handleAt(replyingTo) }) : $t('views.compose.placeholder')"
          class="w-full resize-none border-0 bg-transparent text-xl text-neutral-900 placeholder:text-neutral-500 focus:ring-0"
        />
        <div class="mt-2 flex flex-wrap items-center justify-between gap-2 border-t border-neutral-200 pt-3">
          <div class="flex flex-wrap items-center gap-1">
            <label
              class="inline-flex cursor-pointer items-center rounded-full p-2 text-lime-600 hover:bg-lime-50 disabled:cursor-not-allowed disabled:opacity-50"
              :class="{ 'pointer-events-none opacity-50': attachmentPickerDisabled }"
            >
              <input
                type="file"
                accept="image/*,video/*,audio/*"
                multiple
                class="hidden"
                :disabled="attachmentPickerDisabled"
                @change="onFilesSelect"
              />
              <span class="sr-only">{{ $t("views.compose.addImages") }}</span>
              <Icon name="image" class="h-5 w-5" />
            </label>
            <ComposerEmojiPicker :disabled="busy" :viewer-handle="myHandle" @select="insertComposerEmoji" />
            <button
              type="button"
              class="rounded-full p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800"
              :class="(isNsfw || composerNsfwOpen) && 'bg-amber-50 text-amber-800'"
              :title="composerNsfwOpen ? $t('views.compose.nsfwClose') : $t('views.compose.nsfwTitle')"
              :aria-expanded="composerNsfwOpen"
              aria-controls="composer-nsfw-panel"
              @click="composerNsfwOpen = !composerNsfwOpen"
            >
              <span class="sr-only">{{ $t("views.compose.nsfwOpen") }}</span>
              <Icon name="warning" class="h-5 w-5" />
            </button>
            <button
              type="button"
              class="rounded-full p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800"
              :class="((viewPassword || viewPasswordConfirm) || composerPasswordOpen) && 'bg-neutral-200/80 text-neutral-900'"
              :title="composerPasswordOpen ? $t('views.compose.passwordClose') : $t('views.compose.passwordTitle')"
              :aria-expanded="composerPasswordOpen"
              aria-controls="composer-password-panel"
              @click="composerPasswordOpen = !composerPasswordOpen"
            >
              <span class="sr-only">{{ $t("views.compose.passwordOpen") }}</span>
              <Icon name="lock" class="h-5 w-5" />
            </button>
            <button
              type="button"
              class="rounded-full p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800"
              :class="(membershipUsePatreon || composerMembershipOpen) && 'bg-sky-50 text-sky-800'"
              :title="composerMembershipOpen ? $t('views.compose.membershipClose') : $t('views.compose.membershipTitle')"
              :aria-expanded="composerMembershipOpen"
              aria-controls="feed-composer-membership-panel"
              @click="composerMembershipOpen = !composerMembershipOpen"
            >
              <span class="sr-only">{{ $t("views.compose.membershipOpen") }}</span>
              <Icon name="user" class="h-5 w-5" />
            </button>
            <button
              type="button"
              class="rounded-full p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800"
              :class="composerPollOpen && 'bg-lime-50 text-lime-800'"
              :title="$t('views.compose.pollTitle')"
              :aria-pressed="composerPollOpen"
              @click="composerPollOpen = !composerPollOpen"
            >
              <span class="sr-only">{{ $t("views.compose.pollOpen") }}</span>
              <Icon name="chart" class="h-5 w-5" />
            </button>
            <button
              type="button"
              class="rounded-full p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800"
              :class="(composerScheduleOpen || scheduleLocal) && 'bg-violet-50 text-violet-800'"
              :title="composerScheduleOpen ? $t('views.compose.scheduleClose') : $t('views.compose.scheduleTitle')"
              :aria-expanded="composerScheduleOpen"
              aria-controls="composer-schedule-panel"
              @click="composerScheduleOpen = !composerScheduleOpen"
            >
              <span class="sr-only">{{ $t("views.compose.scheduleOpen") }}</span>
              <Icon name="calendar" class="h-5 w-5" />
            </button>
            <button
              type="button"
              class="rounded-full p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800"
              :class="(composerVisibilityOpen || composerVisibility !== 'public') && 'bg-neutral-200/80 text-neutral-900'"
              :title="composerVisibilityOpen ? $t('views.compose.visibilityClose') : $t('views.compose.visibilityTitle', { label: composerVisibilityMeta(composerVisibility).label })"
              :aria-expanded="composerVisibilityOpen"
              aria-controls="composer-visibility-panel"
              @click="composerVisibilityOpen = !composerVisibilityOpen"
            >
              <span class="sr-only">{{ $t("views.compose.visibilityOpen") }}</span>
              <Icon name="eye" class="h-5 w-5" />
            </button>
            <span class="text-xs text-neutral-500">{{ composerAttachmentLabel(selectedImages) }}</span>
          </div>
          <button
            type="button"
            class="rounded-full bg-lime-500 px-4 py-1.5 text-sm font-semibold text-white hover:bg-lime-600 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="busy"
            @click="submitPost"
          >
            {{ busy ? $t("views.compose.submitting") : $t("views.compose.submit") }}
          </button>
        </div>
        <div v-if="composerPollOpen" class="mt-3 rounded-xl border border-lime-200/80 bg-lime-50/50 px-3 py-3 text-sm">
          <p class="mb-2 text-xs font-medium text-neutral-700">{{ $t("views.compose.pollSectionTitle") }}</p>
          <div class="space-y-2">
            <div v-for="(_po, i) in pollOptionInputs" :key="i">
              <label class="sr-only" :for="`poll-opt-${i}`">{{ $t("views.compose.pollOptionSrOnly", { n: i + 1 }) }}</label>
              <input
                :id="`poll-opt-${i}`"
                v-model="pollOptionInputs[i]"
                type="text"
                maxlength="80"
                class="w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 focus:ring-2"
                :placeholder="$t('views.compose.pollOptionPlaceholder', { n: i + 1 })"
              />
            </div>
          </div>
          <button
            v-if="pollOptionInputs.length < 4"
            type="button"
            class="mt-2 text-xs font-medium text-lime-800 hover:underline"
            @click="addPollOptionField"
          >
            {{ $t("views.compose.pollAddOption") }}
          </button>
          <div class="mt-3">
            <label class="text-xs text-neutral-600" for="poll-dur">{{ $t("views.compose.pollDurationLabel") }}</label>
            <select
              id="poll-dur"
              v-model.number="pollDurationHours"
              class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 focus:ring-2"
            >
              <option :value="1">{{ $t("views.compose.pollDuration1h") }}</option>
              <option :value="6">{{ $t("views.compose.pollDuration6h") }}</option>
              <option :value="24">{{ $t("views.compose.pollDuration24h") }}</option>
              <option :value="72">{{ $t("views.compose.pollDuration72h") }}</option>
              <option :value="168">{{ $t("views.compose.pollDuration168h") }}</option>
            </select>
          </div>
        </div>
        <div
          v-if="composerScheduleOpen"
          id="composer-schedule-panel"
          class="mt-3 rounded-xl border border-violet-200/90 bg-violet-50/60 px-3 py-3 text-sm"
        >
          <label class="text-xs font-medium text-neutral-700" for="schedule-at">{{ $t("views.compose.schedulePanelLabel") }}</label>
          <input
            id="schedule-at"
            v-model="scheduleLocal"
            type="datetime-local"
            class="mt-1 w-full max-w-xs rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-violet-500 focus:ring-2"
          />
          <p class="mt-2 text-xs text-neutral-600">{{ $t("views.compose.scheduleHint") }}</p>
        </div>
        <div
          v-if="composerVisibilityOpen"
          id="composer-visibility-panel"
          class="mt-3 rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-3 text-sm"
        >
          <label class="text-xs font-medium text-neutral-700" for="composer-visibility">{{ $t("views.compose.visibilitySelectLabel") }}</label>
          <select
            id="composer-visibility"
            v-model="composerVisibility"
            class="mt-1 w-full max-w-xs rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 focus:ring-2"
          >
            <option v-for="option in visibilityOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
          <p class="mt-2 text-xs text-neutral-600">
            {{ composerVisibilityMeta(composerVisibility).description }}
          </p>
        </div>
        <div v-if="composerNsfwOpen || composerPasswordOpen || composerMembershipOpen" class="mt-3 space-y-3">
          <div
            v-if="composerNsfwOpen"
            id="composer-nsfw-panel"
            class="rounded-xl border border-amber-200/90 bg-amber-50/70 px-3 py-3 text-sm"
          >
            <label class="inline-flex cursor-pointer items-center gap-2 text-neutral-800">
              <input v-model="isNsfw" type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-amber-600 focus:ring-amber-500" />
              <span>{{ $t("views.compose.nsfwPostCheckbox") }}</span>
            </label>
            <p class="mt-2 text-xs text-amber-900/75">{{ $t("views.compose.nsfwPostHint") }}</p>
          </div>
          <div
            v-if="composerPasswordOpen"
            id="composer-password-panel"
            class="rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-3 text-sm"
          >
            <p class="mb-2 text-xs font-medium text-neutral-600">{{ $t("views.compose.passwordPanelIntro") }}</p>
            <div class="grid gap-2 sm:grid-cols-2">
              <div>
                <label class="mb-0.5 block text-xs text-neutral-500" for="view-pw">{{ $t("views.compose.passwordFieldLabel") }}</label>
                <input
                  id="view-pw"
                  v-model="viewPassword"
                  type="password"
                  autocomplete="new-password"
                  maxlength="72"
                  :placeholder="$t('views.compose.passwordPlaceholderOpen')"
                  class="w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 focus:ring-2"
                />
              </div>
              <div>
                <label class="mb-0.5 block text-xs text-neutral-500" for="view-pw2">{{ $t("views.compose.passwordConfirmLabel") }}</label>
                <input
                  id="view-pw2"
                  v-model="viewPasswordConfirm"
                  type="password"
                  autocomplete="new-password"
                  maxlength="72"
                  :placeholder="$t('views.compose.passwordConfirmPlaceholder')"
                  class="w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 focus:ring-2"
                />
              </div>
            </div>
            <div class="mt-3 rounded-xl border border-neutral-200 bg-white px-3 py-3">
              <p class="text-xs font-medium text-neutral-600">{{ $t("views.compose.protectTargetsHeading") }}</p>
              <div class="mt-2 flex flex-wrap gap-2">
                <button
                  type="button"
                  class="rounded-full border px-3 py-1.5 text-xs font-medium"
                  :class="composerProtectAll ? 'border-lime-500 bg-lime-50 text-lime-800' : 'border-neutral-200 text-neutral-700 hover:bg-neutral-50'"
                  @click="toggleComposerProtectAll"
                >
                  {{ $t("views.compose.protectAll") }}
                </button>
                <button
                  type="button"
                  class="rounded-full border px-3 py-1.5 text-xs font-medium"
                  :class="composerProtectText ? 'border-lime-500 bg-lime-50 text-lime-800' : 'border-neutral-200 text-neutral-700 hover:bg-neutral-50'"
                  @click="toggleComposerProtectText"
                >
                  {{ $t("views.compose.protectTextPart") }}
                </button>
                <button
                  type="button"
                  class="rounded-full border px-3 py-1.5 text-xs font-medium"
                  :class="composerProtectMedia ? 'border-lime-500 bg-lime-50 text-lime-800' : 'border-neutral-200 text-neutral-700 hover:bg-neutral-50'"
                  @click="toggleComposerProtectMedia"
                >
                  {{ $t("views.compose.protectMediaOnly") }}
                </button>
              </div>
              <p class="mt-2 text-xs text-neutral-500">
                {{ $t("views.compose.protectTargetsHint", { all: $t("views.compose.protectAll") }) }}
              </p>
              <div v-if="composerProtectText && !composerProtectAll" class="mt-3 rounded-xl border border-dashed border-neutral-200 bg-neutral-50 px-3 py-3">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <p class="text-xs font-medium text-neutral-700">{{ $t("views.compose.protectRangesTitle") }}</p>
                  <button
                    type="button"
                    class="rounded-full border border-neutral-200 px-3 py-1 text-xs font-medium text-neutral-700 hover:bg-white"
                    @click="addComposerSelectedTextRange"
                  >
                    {{ $t("views.compose.addTextRange") }}
                  </button>
                </div>
                <p class="mt-2 text-xs text-neutral-500">
                  {{ $t("views.compose.addTextRangeHintComposer") }}
                </p>
                <ul v-if="composerTextRanges.length" class="mt-3 space-y-2">
                  <li
                    v-for="(rg, idx) in composerTextRanges"
                    :key="`${rg.start}-${rg.end}-${idx}`"
                    class="flex items-start justify-between gap-3 rounded-lg border border-neutral-200 bg-white px-3 py-2"
                  >
                    <div class="min-w-0">
                      <p class="truncate text-sm text-neutral-800">{{ composerTextRangePreview(rg) }}</p>
                      <p class="text-xs text-neutral-500">{{ $t("views.compose.charRange", { start: rg.start, end: rg.end }) }}</p>
                    </div>
                    <button
                      type="button"
                      class="shrink-0 text-xs font-medium text-red-600 hover:text-red-700"
                      @click="removeComposerTextRange(idx)"
                    >
                      {{ $t("views.compose.remove") }}
                    </button>
                  </li>
                </ul>
              </div>
            </div>
          </div>
          <div
            v-if="composerMembershipOpen"
            id="feed-composer-membership-panel"
            class="rounded-xl border border-sky-200/90 bg-sky-50/70 px-3 py-3 text-sm"
          >
            <p class="text-xs font-medium text-sky-950">{{ $t("views.compose.membershipTitle") }}</p>
            <p class="mt-1 text-xs text-sky-900/80">{{ $t("views.compose.membershipHint") }}</p>
            <div class="mt-3 flex flex-wrap gap-2">
              <button
                v-if="patreonAvailable"
                type="button"
                class="rounded-full border px-3 py-1.5 text-xs font-medium"
                :class="membershipProvider === 'patreon' ? 'border-sky-500 bg-white text-sky-900' : 'border-sky-200 text-sky-800 hover:bg-white'"
                @click="membershipProvider = 'patreon'"
              >
                Patreon
              </button>
              <button
                type="button"
                class="rounded-full border px-3 py-1.5 text-xs font-medium"
                :class="membershipProvider === 'gumroad' ? 'border-sky-500 bg-white text-sky-900' : 'border-sky-200 text-sky-800 hover:bg-white'"
                @click="membershipProvider = 'gumroad'"
              >
                Gumroad
              </button>
            </div>
            <div v-if="membershipProvider === 'patreon' && !patreonConnected" class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center">
              <button
                type="button"
                :disabled="patreonConnectBusy"
                class="inline-flex w-fit rounded-full bg-sky-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-sky-700 disabled:opacity-50"
                @click="connectPatreonFromFeed"
              >
                {{
                  patreonConnectBusy
                    ? $t("views.settings.fanclubPatreon.connecting")
                    : $t("views.settings.fanclubPatreon.connect")
                }}
              </button>
              <RouterLink :to="patreonSettingsHref" class="text-xs font-medium text-sky-800 underline">{{
                $t("views.compose.membershipGoSettings")
              }}</RouterLink>
            </div>
            <template v-else-if="membershipProvider === 'patreon'">
              <label class="mt-3 flex cursor-pointer items-center gap-2 text-sm text-sky-950">
                <input v-model="membershipUsePatreon" type="checkbox" class="h-4 w-4 rounded border-sky-300 text-sky-600" />
                <span>{{ $t("views.compose.membershipOpen") }}</span>
              </label>
              <div v-if="membershipUsePatreon" class="mt-3 space-y-2">
                <div v-if="patreonCampaigns.length" class="grid gap-2 sm:grid-cols-2">
                  <div>
                    <label class="mb-0.5 block text-xs text-sky-900/80" for="feed-mem-camp">{{ $t("views.compose.membershipPickCampaign") }}</label>
                    <select
                      id="feed-mem-camp"
                      v-model="membershipCampaignId"
                      class="w-full rounded-xl border border-sky-200 bg-white px-2 py-2 text-sm text-neutral-900"
                    >
                      <option value="">{{ "—" }}</option>
                      <option v-for="c in patreonCampaigns" :key="c.id" :value="c.id">{{ c.title || c.id }}</option>
                    </select>
                  </div>
                  <div>
                    <label class="mb-0.5 block text-xs text-sky-900/80" for="feed-mem-tier">{{ $t("views.compose.membershipPickTier") }}</label>
                    <select
                      id="feed-mem-tier"
                      v-model="membershipTierId"
                      :disabled="!membershipCampaignId"
                      class="w-full rounded-xl border border-sky-200 bg-white px-2 py-2 text-sm text-neutral-900 disabled:opacity-50"
                    >
                      <option value="">{{ "—" }}</option>
                      <option v-for="tier in membershipTierOptions" :key="tier.id" :value="tier.id">{{
                        tier.name || tier.id
                      }}</option>
                    </select>
                  </div>
                </div>
                <div v-else class="grid gap-2 sm:grid-cols-2">
                  <div>
                    <label class="mb-0.5 block text-xs text-sky-900/80" for="feed-mem-cid">{{ $t("views.compose.membershipCampaign") }}</label>
                    <input
                      id="feed-mem-cid"
                      v-model="membershipCampaignId"
                      type="text"
                      autocomplete="off"
                      class="w-full rounded-xl border border-sky-200 bg-white px-2 py-2 text-sm text-neutral-900"
                    />
                  </div>
                  <div>
                    <label class="mb-0.5 block text-xs text-sky-900/80" for="feed-mem-tid">{{ $t("views.compose.membershipTier") }}</label>
                    <input
                      id="feed-mem-tid"
                      v-model="membershipTierId"
                      type="text"
                      autocomplete="off"
                      class="w-full rounded-xl border border-sky-200 bg-white px-2 py-2 text-sm text-neutral-900"
                    />
                  </div>
                </div>
              </div>
            </template>
            <template v-else>
              <label class="mt-3 flex cursor-pointer items-center gap-2 text-sm text-sky-950">
                <input v-model="membershipUsePatreon" type="checkbox" class="h-4 w-4 rounded border-sky-300 text-sky-600" />
                <span>{{ $t("views.compose.gumroadMembershipOpen") }}</span>
              </label>
              <div v-if="membershipUsePatreon" class="mt-3">
                <label class="mb-0.5 block text-xs text-sky-900/80" for="feed-gumroad-product-id">{{
                  $t("views.compose.gumroadProductId")
                }}</label>
                <input
                  id="feed-gumroad-product-id"
                  v-model="membershipCampaignId"
                  type="text"
                  autocomplete="off"
                  class="w-full rounded-xl border border-sky-200 bg-white px-2 py-2 text-sm text-neutral-900"
                  :placeholder="$t('views.compose.gumroadProductIdPlaceholder')"
                />
              </div>
            </template>
          </div>
        </div>
        <div v-if="previewUrls.length" class="mt-3 space-y-3">
          <div v-if="attachmentKind === 'video'" class="relative">
            <GlipzVideoPlayer :src="previewUrls[0]!" />
            <button
              type="button"
              class="absolute right-2 top-2 z-10 flex h-7 w-7 items-center justify-center rounded-full bg-black/60 text-sm font-bold text-white hover:bg-black/80"
              :disabled="busy"
              :aria-label="$t('views.compose.removeImageAria', { n: 1 })"
              @click="removeImage(0)"
            >
              ×
            </button>
          </div>
          <div v-else-if="attachmentKind === 'audio'" class="relative">
            <GlipzAudioPlayer :src="previewUrls[0]!" />
            <button
              type="button"
              class="absolute right-3 top-3 z-10 flex h-7 w-7 items-center justify-center rounded-full bg-black/50 text-sm font-bold text-white hover:bg-black/70"
              :disabled="busy"
              :aria-label="$t('views.compose.removeImageAria', { n: 1 })"
              @click="removeImage(0)"
            >
              ×
            </button>
          </div>
          <div v-else class="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <div
              v-for="(url, i) in previewUrls"
              :key="`${url}-${i}`"
              class="relative aspect-square overflow-hidden rounded-xl border border-neutral-200 bg-neutral-100"
            >
              <img :src="url" :alt="$t('views.compose.selectedImageAlt', { n: i + 1 })" class="h-full w-full object-cover" />
              <button
                type="button"
                class="absolute right-1 top-1 flex h-7 w-7 items-center justify-center rounded-full bg-black/60 text-sm font-bold text-white hover:bg-black/80"
                :disabled="busy"
                :aria-label="$t('views.compose.removeImageAria', { n: i + 1 })"
                @click="removeImage(i)"
              >
                ×
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <p v-if="actionToast" class="border-b border-lime-100 bg-lime-50 px-4 py-2 text-center text-sm text-lime-900">
      {{ actionToast }}
    </p>
    <p v-if="err" class="border-b border-neutral-200 px-4 py-3 text-sm text-red-600">{{ err }}</p>

    <p v-if="!items.length && !err" class="border-b border-neutral-200 px-4 py-16 text-center text-neutral-500">
      {{
        feedScope === "following"
          ? $t("views.feed.emptyFollowing")
          : feedScope === "recommended"
            ? $t("views.feed.emptyRecommended")
            : $t("views.feed.emptyAll")
      }}
    </p>

    <PostTimeline
      :items="items"
      :thread-replies-by-root="threadRepliesByRoot"
      :action-busy="actionBusy"
      :viewer-email="myEmail"
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
  </PullToRefresh>
  <RouterLink
    to="/compose"
    class="fixed bottom-[calc(5.5rem+env(safe-area-inset-bottom))] right-4 z-20 flex h-14 w-14 items-center justify-center rounded-full bg-lime-600 text-white shadow-lg shadow-lime-900/20 transition lg:hidden"
    :class="isNearFeedBottom ? 'opacity-10 hover:opacity-25' : 'opacity-100 hover:bg-lime-700'"
    :aria-label="$t('views.feed.fabComposeAria')"
  >
    <Icon name="pencil" class="h-6 w-6" />
  </RouterLink>
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
                  :alt="$t('views.feed.lightboxImageAlt', { n: li + 1 })"
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

  <RepostModal
    v-model:open="repostModalOpen"
    :post="repostTarget"
    :submitting="!!actionBusy && actionBusy.startsWith('rp-')"
    @plain="confirmRepostPlain"
    @with-comment="confirmRepostWithComment"
  />
</template>
