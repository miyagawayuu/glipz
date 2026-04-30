<script setup lang="ts">
import Cropper from "cropperjs";
import "cropperjs/dist/cropper.css";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRoute, useRouter } from "vue-router";
import Icon from "../components/Icon.vue";
import { clearTokens, getAccessToken } from "../auth";
import UserBadges from "../components/UserBadges.vue";
import { formatUpdatedAt } from "../i18n";
import { api, uploadMediaFile } from "../lib/api";
import { fetchPostThreadReplies, mapFeedItem } from "../lib/feedStream";
import PostTimeline from "../components/PostTimeline.vue";
import RepostModal from "../components/RepostModal.vue";
import { addTimelineReaction, removeTimelineReaction, toggleTimelineBookmark, toggleTimelineRepost } from "../lib/federationActions";
import type { TimelinePost } from "../types/timeline";
import { avatarInitials, fullHandleAt, postDetailPath } from "../lib/feedDisplay";
import { buildComposerReplyQuery, composeRoutePath } from "../lib/postComposer";
import { bumpMeHub } from "../meHub";
import { createDMThread, inviteDMPeer } from "../lib/dm";
import { safeHttpURL, safeMediaURL } from "../lib/redirect";
import { isSafeProfileImageFile, SAFE_PROFILE_IMAGE_ACCEPT } from "../lib/composerMedia";

let cropperInstance: InstanceType<typeof Cropper> | null = null;

type Profile = {
  handle: string;
  display_name: string;
  badges?: string[];
  /** Saved display name returned only for the owner. When empty, the UI falls back to the email-derived name. */
  display_name_raw?: string;
  bio: string;
  /** External site URLs, limited to five http/https entries. */
  profile_urls?: string[];
  avatar_url: string | null;
  header_url: string | null;
  pinned_post_id?: string | null;
  is_me: boolean;
  follower_count?: number;
  following_count?: number;
  followed_by_me?: boolean;
  follows_you?: boolean;
  email?: string;
  avatar_object_key?: string;
  header_object_key?: string;
};

const { t } = useI18n();

function safeHeaderStyle(raw: unknown): Record<string, string> {
  const url = safeHttpURL(raw);
  return url ? { backgroundImage: `url("${url}")`, backgroundSize: "cover", backgroundPosition: "center" } : {};
}

function safeProfileImageURL(raw: unknown): string {
  return safeHttpURL(raw);
}
const route = useRoute();
const router = useRouter();

const handleParam = computed(() =>
  String(route.params.handle ?? "")
    .replace(/^@/, "")
    .trim(),
);

const profile = ref<Profile | null>(null);
const posts = ref<TimelinePost[]>([]);
const threadRepliesByRoot = ref<Record<string, TimelinePost[]>>({});
type ProfilePostTab = "posts" | "replies" | "media";
const profilePostTab = ref<ProfilePostTab>("posts");
const replyPosts = ref<TimelinePost[]>([]);
const replyPostsLoaded = ref(false);
const replyPostsBusy = ref(false);
const replyPostsErr = ref("");

type MediaTileRow = { post_id: string; media_type: string; preview_url: string; locked: boolean };
const mediaTiles = ref<MediaTileRow[]>([]);
const mediaTilesLoaded = ref(false);
const mediaTilesBusy = ref(false);
const mediaTilesErr = ref("");
/** Logged-in email, used for menus on the viewer's own posts. */
const myEmail = ref<string | null>(null);
const profileHeaderInApp = ref(Boolean(getAccessToken()));
const err = ref("");
const editBio = ref("");
const editProfileUrls = ref<string[]>([""]);
const editDisplayName = ref("");
const editHandle = ref("");
const editIsBot = ref(false);
const editIsAI = ref(false);
const avatarKey = ref("");
const headerKey = ref("");
const saving = ref(false);
const uploadBusy = ref(false);
const followBusy = ref(false);
const dmOpenBusy = ref(false);
const profileEditModalOpen = ref(false);
const profileModalErr = ref("");

/** Image cropper state. Set to null to close it. */
const cropKind = ref<"avatar" | "header" | null>(null);
const cropSrc = ref("");
const cropKey = ref(0);
const cropImageRef = ref<HTMLImageElement | null>(null);

const actionBusy = ref<string | null>(null);
const repostModalOpen = ref(false);
const repostTarget = ref<TimelinePost | null>(null);
const actionToast = ref("");
let toastTimer: ReturnType<typeof setTimeout> | null = null;

type LightboxState = { urls: string[]; index: number };
const lightbox = ref<LightboxState | null>(null);
let lightboxTouchStartX = 0;

function showToast(msg: string) {
  if (toastTimer) clearTimeout(toastTimer);
  actionToast.value = msg;
  toastTimer = setTimeout(() => {
    actionToast.value = "";
    toastTimer = null;
  }, 2200);
}

function patchPost(id: string, patch: Partial<TimelinePost>) {
  posts.value = posts.value.map((x) => (x.id === id ? { ...x, ...patch } : x));
  replyPosts.value = replyPosts.value.map((x) => (x.id === id ? { ...x, ...patch } : x));
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

function sortProfilePosts(list: TimelinePost[]): TimelinePost[] {
  const pinnedId = profile.value?.pinned_post_id ?? "";
  return [...list]
    .map((it) => ({ ...it, is_pinned_to_profile: pinnedId ? it.id === pinnedId : Boolean(it.is_pinned_to_profile) }))
    .sort((a, b) => {
      if (a.is_pinned_to_profile !== b.is_pinned_to_profile) return a.is_pinned_to_profile ? -1 : 1;
      return 0;
    });
}

async function refreshThreadForRoot(rootId: string) {
  const token = getAccessToken();
  const list = await fetchPostThreadReplies(rootId, token);
  const tr = { ...threadRepliesByRoot.value };
  if (list.length) tr[rootId] = list;
  else delete tr[rootId];
  threadRepliesByRoot.value = tr;
}

async function loadThreadsForProfile() {
  const token = getAccessToken();
  const withReplies = posts.value.filter((x) => x.reply_count > 0);
  const next: Record<string, TimelinePost[]> = {};
  await Promise.all(
    withReplies.map(async (it) => {
      const list = await fetchPostThreadReplies(it.id, token);
      if (list.length) next[it.id] = list;
    }),
  );
  threadRepliesByRoot.value = next;
}

async function removePost(id: string) {
  const affectedRoots = Object.entries(threadRepliesByRoot.value)
    .filter(([, list]) => list.some((x) => x.id === id))
    .map(([rootId]) => rootId);
  posts.value = posts.value.filter((x) => x.id !== id);
  replyPosts.value = replyPosts.value.filter((x) => x.id !== id);
  const tr = { ...threadRepliesByRoot.value };
  delete tr[id];
  threadRepliesByRoot.value = tr;
  for (const rid of affectedRoots) {
    if (rid !== id && posts.value.some((x) => x.id === rid)) {
      await refreshThreadForRoot(rid);
    }
  }
}

async function loadMeEmail() {
  const token = getAccessToken();
  profileHeaderInApp.value = Boolean(token);
  if (!token) {
    myEmail.value = null;
    return;
  }
  try {
    const u = await api<{ email: string }>("/api/v1/me", { method: "GET", token });
    myEmail.value = u.email;
  } catch (e: unknown) {
    // Tokens can remain in localStorage after expiring, which would desync the UI's logged-in state.
    // Clear them as soon as /me fails so the UI consistently falls back to logged-out behavior.
    const msg = e instanceof Error ? e.message : "";
    if (msg === "unauthorized") {
      clearTokens();
      profileHeaderInApp.value = false;
    }
    myEmail.value = null;
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

watch(lightbox, (v) => {
  if (v) {
    document.body.style.overflow = "hidden";
    window.addEventListener("keydown", onLightboxKeydown);
  } else {
    document.body.style.overflow = "";
    window.removeEventListener("keydown", onLightboxKeydown);
  }
});

function onCropKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") {
    e.preventDefault();
    closeCropModal();
  }
}

function onProfileEditKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") {
    e.preventDefault();
    closeProfileEditModal();
  }
}

watch(
  () => Boolean(cropKind.value && cropSrc.value),
  (open) => {
    if (open) window.addEventListener("keydown", onCropKeydown);
    else window.removeEventListener("keydown", onCropKeydown);
  },
);

watch(profileEditModalOpen, (open) => {
  if (open) window.addEventListener("keydown", onProfileEditKeydown);
  else window.removeEventListener("keydown", onProfileEditKeydown);
});

function openProfileEditModal() {
  const p = profile.value;
  if (!p?.is_me) return;
  profileModalErr.value = "";
  editBio.value = p.bio ?? "";
  syncEditProfileUrlsFromProfile();
  editDisplayName.value = p.display_name_raw ?? "";
  editHandle.value = p.handle ?? "";
  editIsBot.value = Array.isArray(p.badges) && p.badges.includes("bot");
  editIsAI.value = Array.isArray(p.badges) && p.badges.includes("ai");
  profileEditModalOpen.value = true;
}

function closeProfileEditModal() {
  profileEditModalOpen.value = false;
  profileModalErr.value = "";
  const p = profile.value;
  if (!p?.is_me) return;
  editBio.value = p.bio ?? "";
  syncEditProfileUrlsFromProfile();
  editDisplayName.value = p.display_name_raw ?? "";
  editHandle.value = p.handle ?? "";
  editIsBot.value = Array.isArray(p.badges) && p.badges.includes("bot");
  editIsAI.value = Array.isArray(p.badges) && p.badges.includes("ai");
}

function setProfileSaveError(msg: string) {
  if (profileEditModalOpen.value) profileModalErr.value = msg;
  else err.value = msg;
}

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onLightboxKeydown);
  window.removeEventListener("keydown", onCropKeydown);
  window.removeEventListener("keydown", onProfileEditKeydown);
  document.body.style.overflow = "";
  if (toastTimer) clearTimeout(toastTimer);
  closeCropModal();
  closeProfileEditModal();
});

async function loadAll() {
  const h = handleParam.value;
  if (!h) {
    err.value = t("views.userProfile.errors.userNotFound");
    return;
  }
  const token = getAccessToken();
  const authOpt = token ? { token } : {};
  err.value = "";
  try {
    replyPostsLoaded.value = false;
    replyPosts.value = [];
    replyPostsErr.value = "";
    mediaTilesLoaded.value = false;
    mediaTiles.value = [];
    mediaTilesErr.value = "";
    profilePostTab.value = "posts";
    const enc = encodeURIComponent(h);
    const p = await api<Profile>(`/api/v1/users/by-handle/${enc}`, { method: "GET", ...authOpt });
    profile.value = {
      ...p,
      profile_urls: Array.isArray(p.profile_urls) ? p.profile_urls : [],
      follower_count: p.follower_count ?? 0,
      following_count: p.following_count ?? 0,
      followed_by_me: Boolean(p.followed_by_me),
      follows_you: Boolean(p.follows_you),
    };
    editBio.value = p.bio ?? "";
    syncEditProfileUrlsFromProfile();
    avatarKey.value = p.avatar_object_key ?? "";
    headerKey.value = p.header_object_key ?? "";
    if (p.is_me) {
      editDisplayName.value = p.display_name_raw ?? "";
      editHandle.value = p.handle ?? "";
      editIsBot.value = Array.isArray(p.badges) && p.badges.includes("bot");
      editIsAI.value = Array.isArray(p.badges) && p.badges.includes("ai");
    }

    const res = await api<{ items: TimelinePost[] }>(`/api/v1/users/by-handle/${enc}/posts`, {
      method: "GET",
      ...authOpt,
    });
    posts.value = sortProfilePosts(res.items.map((x) => mapFeedItem(x as Parameters<typeof mapFeedItem>[0])));
    await loadThreadsForProfile();
  } catch (e: unknown) {
    profile.value = null;
    posts.value = [];
    threadRepliesByRoot.value = {};
    replyPosts.value = [];
    replyPostsLoaded.value = false;
    replyPostsErr.value = "";
    mediaTiles.value = [];
    mediaTilesLoaded.value = false;
    mediaTilesErr.value = "";
    err.value = e instanceof Error ? e.message : t("views.userProfile.errors.loadFailed");
  }
}

async function loadReplyPosts() {
  if (replyPostsLoaded.value) return;
  const h = handleParam.value;
  if (!h) return;
  const token = getAccessToken();
  const authOpt = token ? { token } : {};
  replyPostsBusy.value = true;
  replyPostsErr.value = "";
  try {
    const enc = encodeURIComponent(h);
    const res = await api<{ items: TimelinePost[] }>(`/api/v1/users/by-handle/${enc}/replies`, {
      method: "GET",
      ...authOpt,
    });
    replyPosts.value = res.items.map((x) => mapFeedItem(x as Parameters<typeof mapFeedItem>[0]));
    replyPostsLoaded.value = true;
  } catch (e: unknown) {
    replyPostsErr.value = e instanceof Error ? e.message : t("views.userProfile.errors.loadFailed");
  } finally {
    replyPostsBusy.value = false;
  }
}

async function loadMediaTiles() {
  if (mediaTilesLoaded.value) return;
  const h = handleParam.value;
  if (!h) return;
  const token = getAccessToken();
  const authOpt = token ? { token } : {};
  mediaTilesBusy.value = true;
  mediaTilesErr.value = "";
  try {
    const enc = encodeURIComponent(h);
    const res = await api<{ tiles: MediaTileRow[] }>(`/api/v1/users/by-handle/${enc}/post-media-tiles`, {
      method: "GET",
      ...authOpt,
    });
    mediaTiles.value = res.tiles ?? [];
    mediaTilesLoaded.value = true;
  } catch (e: unknown) {
    mediaTilesErr.value = e instanceof Error ? e.message : t("views.userProfile.errors.loadFailed");
  } finally {
    mediaTilesBusy.value = false;
  }
}

watch(profilePostTab, (t) => {
  if (t === "replies") void loadReplyPosts();
  if (t === "media") void loadMediaTiles();
});

function normalizeHandleInput(raw: string): string {
  return raw.trim().toLowerCase().replace(/^@+/, "");
}

function validateHandleClient(h: string): string | null {
  if (!h) return t("auth.register.errors.handleRequired");
  if (h.length > 30) return t("auth.register.errors.handleTooLong");
  if (!/^[a-z0-9_]+$/.test(h)) return t("auth.register.errors.handleChars");
  return null;
}

function profileSaveErrorMessage(code: string): string {
  const m: Record<string, string> = {
    handle_taken: t("auth.register.errors.handleTaken"),
    invalid_handle: t("auth.register.errors.invalidHandle"),
    reserved_handle: t("auth.register.errors.reservedHandle"),
    display_name_too_long: t("views.userProfile.errors.displayNameTooLong"),
    bio_too_long: t("views.userProfile.errors.bioTooLong"),
    invalid_profile_url: t("views.userProfile.errors.invalidProfileUrl"),
    profile_urls_too_many: t("views.userProfile.errors.profileUrlsTooMany"),
    profile_url_too_long: t("views.userProfile.errors.profileUrlTooLong"),
  };
  return m[code] ?? code;
}

async function saveProfile() {
  const token = getAccessToken();
  if (!token || !profile.value?.is_me) return;
  const handleNorm = normalizeHandleInput(editHandle.value);
  const vh = validateHandleClient(handleNorm);
  profileModalErr.value = "";
  err.value = "";
  if (vh) {
    setProfileSaveError(vh);
    return;
  }
  if ([...editDisplayName.value.trim()].length > 50) {
    setProfileSaveError(t("views.userProfile.errors.displayNameTooLong"));
    return;
  }
  saving.value = true;
  const prevRouteHandle = handleParam.value;
  const profileUrlsPayload = editProfileUrls.value.map((x) => x.trim()).filter(Boolean);
  try {
    await api("/api/v1/me/profile", {
      method: "PATCH",
      token,
      json: {
        bio: editBio.value,
        display_name: editDisplayName.value.trim(),
        handle: handleNorm,
        avatar_object_key: avatarKey.value,
        header_object_key: headerKey.value,
        profile_urls: profileUrlsPayload,
        is_bot: editIsBot.value,
        is_ai: editIsAI.value,
      },
    });
    showToast(t("views.userProfile.savedToast"));
    if (handleNorm !== prevRouteHandle) {
      await router.replace(`/@${handleNorm}`);
    }
    await loadAll();
    bumpMeHub();
    closeProfileEditModal();
  } catch (e: unknown) {
    const raw = e instanceof Error ? e.message : "";
    setProfileSaveError(profileSaveErrorMessage(raw) || t("views.userProfile.errors.saveFailed"));
  } finally {
    saving.value = false;
  }
}

async function uploadProfileImage(kind: "avatar" | "header", file: File) {
  const token = getAccessToken();
  if (!token || !profile.value?.is_me) return;
  uploadBusy.value = true;
  err.value = "";
  try {
    const up = await uploadMediaFile(token, file);
    if (kind === "avatar") {
      avatarKey.value = up.object_key;
    } else {
      headerKey.value = up.object_key;
    }
    await saveProfile();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.userProfile.errors.uploadFailed");
  } finally {
    uploadBusy.value = false;
  }
}

function closeCropModal() {
  cropperInstance?.destroy();
  cropperInstance = null;
  if (cropSrc.value) {
    URL.revokeObjectURL(cropSrc.value);
    cropSrc.value = "";
  }
  cropKind.value = null;
}

function openCropFromFile(kind: "avatar" | "header", file: File) {
  if (!isSafeProfileImageFile(file)) return;
  cropperInstance?.destroy();
  cropperInstance = null;
  if (cropSrc.value) URL.revokeObjectURL(cropSrc.value);
  cropKind.value = kind;
  cropKey.value += 1;
  cropSrc.value = URL.createObjectURL(file);
}

function onPickAvatarFile(e: Event) {
  const input = e.target as HTMLInputElement;
  const f = input.files?.[0];
  input.value = "";
  if (f) openCropFromFile("avatar", f);
}

function onPickHeaderFile(e: Event) {
  const input = e.target as HTMLInputElement;
  const f = input.files?.[0];
  input.value = "";
  if (f) openCropFromFile("header", f);
}

function onCropImageLoad() {
  void nextTick(() => {
    cropperInstance?.destroy();
    cropperInstance = null;
    const el = cropImageRef.value;
    if (!el || !cropKind.value) return;
    const ratio = cropKind.value === "avatar" ? 1 : 3;
    cropperInstance = new Cropper(el, {
      aspectRatio: ratio,
      viewMode: 1,
      dragMode: "move",
      autoCropArea: 0.9,
      responsive: true,
      restore: false,
      guides: true,
      center: true,
      highlight: true,
      cropBoxMovable: true,
      cropBoxResizable: true,
    });
  });
}

async function confirmCrop() {
  if (!cropperInstance || !cropKind.value) return;
  const kind = cropKind.value;
  const outW = kind === "avatar" ? 512 : 1500;
  const outH = kind === "avatar" ? 512 : 500;
  const canvas = cropperInstance.getCroppedCanvas({
    width: outW,
    height: outH,
    imageSmoothingEnabled: true,
    imageSmoothingQuality: "high",
  });
  if (!canvas) {
    closeCropModal();
    return;
  }
  closeCropModal();
  const blob = await new Promise<Blob | null>((resolve) => {
    canvas.toBlob((b) => resolve(b), "image/jpeg", 0.92);
  });
  if (!blob) return;
  const file = new File([blob], kind === "avatar" ? "avatar.jpg" : "header.jpg", { type: "image/jpeg" });
  await uploadProfileImage(kind, file);
}

function applyReactionPost(updated: TimelinePost) {
  patchPost(updated.id, {
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
    showToast(t("views.userProfile.toasts.reactionFailed"));
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
  } catch {
    showToast(t("views.userProfile.toasts.bookmarkFailed"));
  } finally {
    actionBusy.value = null;
  }
}

async function toggleProfilePin(it: TimelinePost) {
  const token = getAccessToken();
  const p = profile.value;
  if (!token || !p?.is_me || actionBusy.value === `pin-${it.id}`) return;
  actionBusy.value = `pin-${it.id}`;
  try {
    const nextPinned = !it.is_pinned_to_profile;
    await api(`/api/v1/posts/${encodeURIComponent(it.id)}/profile-pin`, {
      method: nextPinned ? "PUT" : "DELETE",
      token,
    });
    profile.value = { ...p, pinned_post_id: nextPinned ? it.id : null };
    posts.value = sortProfilePosts(posts.value);
    showToast(nextPinned ? t("views.userProfile.toasts.pinSaved") : t("views.userProfile.toasts.pinRemoved"));
  } catch {
    showToast(t("views.userProfile.toasts.pinFailed"));
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
      patchPost(it.id, { reposted_by_me: res.reposted, repost_count: res.repost_count });
    } catch {
      showToast(t("views.userProfile.toasts.repostUpdateFailed"));
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
    patchPost(it.id, { reposted_by_me: res.reposted, repost_count: res.repost_count });
    repostModalOpen.value = false;
    repostTarget.value = null;
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : "";
    showToast(msg === "repost_comment_too_long" ? t("views.userProfile.toasts.repostCommentTooLong") : t("views.userProfile.toasts.repostFailed"));
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
    patchPost(it.id, { reposted_by_me: res.reposted, repost_count: res.repost_count });
    repostModalOpen.value = false;
    repostTarget.value = null;
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : "";
    showToast(msg === "repost_comment_too_long" ? t("views.userProfile.toasts.repostCommentTooLong") : t("views.userProfile.toasts.repostFailed"));
  } finally {
    actionBusy.value = null;
  }
}

function onReply(it: TimelinePost) {
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
  void router.push({ path: composePath, query: replyQuery });
}

async function sharePost(it: TimelinePost) {
  const resolved = router.resolve({ path: postDetailPath(it.id) });
  const url = new URL(resolved.href, window.location.origin).href;
  if (navigator.share) {
    try {
      await navigator.share({ title: "Glipz", text: it.caption?.slice(0, 80) ?? t("views.userProfile.shareFallbackText"), url });
      return;
    } catch (e: unknown) {
      if (e instanceof DOMException && e.name === "AbortError") return;
    }
  }
  try {
    await navigator.clipboard.writeText(url);
    showToast(t("views.userProfile.toasts.linkCopied"));
  } catch {
    showToast(t("views.userProfile.toasts.shareFailed"));
  }
}

const profileHandleAt = computed(() => (profile.value ? fullHandleAt(profile.value.handle) : ""));

const profileUrlsForDisplay = computed(() =>
  (profile.value?.profile_urls ?? []).map((x) => String(x).trim()).filter(Boolean),
);

/** Marks external favicon fetch failures, keyed by URL string. */
const profileLinkFaviconBroken = ref<Record<string, boolean>>({});

watch(
  () => (profile.value?.profile_urls ?? []).join("\u0001"),
  () => {
    profileLinkFaviconBroken.value = {};
  },
);

function profileFaviconSrc(href: string): string {
  try {
    const host = new URL(href).hostname;
    if (!host) return "";
    return `https://www.google.com/s2/favicons?domain=${encodeURIComponent(host)}&sz=48`;
  } catch {
    return "";
  }
}

function onProfileLinkFaviconError(href: string) {
  profileLinkFaviconBroken.value = { ...profileLinkFaviconBroken.value, [href]: true };
}

function shortUrlLabel(href: string): string {
  try {
    const u = new URL(href);
    const host = u.hostname;
    const path = u.pathname + u.search;
    if (path && path !== "/") {
      const rest = path.length > 56 ? `${path.slice(0, 53)}…` : path;
      return `${host}${rest}`;
    }
    return host || href;
  } catch {
    return href;
  }
}

function addProfileUrlField() {
  if (editProfileUrls.value.length >= 5) return;
  editProfileUrls.value = [...editProfileUrls.value, ""];
}

function syncEditProfileUrlsFromProfile() {
  const p = profile.value;
  if (!p?.is_me) return;
  const u = (p.profile_urls ?? []).map((x) => String(x).trim()).filter(Boolean);
  editProfileUrls.value = u.length ? [...u] : [""];
}
/** Profiles are viewable without login, so use this to branch follow-related UI. */
const viewerAuthed = computed(() => Boolean(myEmail.value));

async function toggleFollow() {
  const p = profile.value;
  const token = getAccessToken();
  const h = handleParam.value;
  if (!p || p.is_me || !token || !h || followBusy.value) return;
  followBusy.value = true;
  try {
    const enc = encodeURIComponent(h);
    const res = await api<{ following: boolean; follower_count: number }>(
      `/api/v1/users/by-handle/${enc}/follow`,
      { method: "POST", token },
    );
    profile.value = {
      ...p,
      followed_by_me: res.following,
      follower_count: res.follower_count,
    };
  } catch {
    showToast(t("views.userProfile.toasts.followFailed"));
  } finally {
    followBusy.value = false;
  }
}

async function openDMFromProfile() {
  const p = profile.value;
  const token = getAccessToken();
  const h = p?.handle;
  if (!p || p.is_me || !token || !h || dmOpenBusy.value) return;
  dmOpenBusy.value = true;
  try {
    try {
      const thread = await createDMThread(h);
      await router.push({ path: `/messages/${thread.id}` });
      return;
    } catch (e) {
      const code = e instanceof Error ? e.message : "";
      if (code === "peer_identity_required") {
        const st = await inviteDMPeer(h);
        if (st === "peer_ready") {
          const thread = await createDMThread(h);
          await router.push({ path: `/messages/${thread.id}` });
          return;
        }
        showToast(
          st === "invited_auto" ? t("views.userProfile.toasts.dmInviteSentAuto") : t("views.userProfile.toasts.dmInviteSent"),
        );
        return;
      }
      if (code === "identity_required") {
        await router.push({ path: "/messages", query: { with: h } });
        showToast(t("views.userProfile.toasts.dmIdentityRequired"));
        return;
      }
      showToast(t("views.userProfile.toasts.dmOpenFailed"));
    }
  } catch {
    showToast(t("views.userProfile.toasts.dmOpenFailed"));
  } finally {
    dmOpenBusy.value = false;
  }
}

onMounted(() => {
  void loadMeEmail();
  void loadAll();
});
watch(handleParam, () => void loadAll());
</script>

<template>
  <Teleport v-if="profileHeaderInApp" to="#app-view-header-slot-desktop">
    <div class="flex h-14 items-center gap-3">
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="$t('views.userProfile.backAria')"
        @click="router.push('/feed')"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-1.5">
          <h1 class="truncate text-lg font-bold leading-tight text-neutral-900">
            {{ profile ? profile.display_name : "…" }}
          </h1>
          <UserBadges v-if="profile" :badges="profile.badges" size="xs" />
        </div>
        <p class="truncate text-sm text-neutral-500">{{ profile ? profileHandleAt : "" }}</p>
      </div>
    </div>
  </Teleport>
  <Teleport v-if="profileHeaderInApp" to="#app-view-header-slot-mobile">
    <div class="flex h-14 items-center gap-3 px-4">
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="$t('views.userProfile.backAria')"
        @click="router.push('/feed')"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-1.5">
          <h1 class="truncate text-lg font-bold leading-tight text-neutral-900">
            {{ profile ? profile.display_name : "…" }}
          </h1>
          <UserBadges v-if="profile" :badges="profile.badges" size="xs" />
        </div>
        <p class="truncate text-sm text-neutral-500">{{ profile ? profileHandleAt : "" }}</p>
      </div>
    </div>
  </Teleport>
  <div class="min-h-0 h-full w-full min-w-0 text-neutral-900">
    <header
      v-if="!profileHeaderInApp"
      class="sticky top-0 z-10 flex h-14 items-center gap-3 border-b border-neutral-200 bg-white/90 px-4 backdrop-blur supports-[backdrop-filter]:bg-white/70"
    >
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="$t('views.userProfile.backAria')"
        @click="router.push(viewerAuthed ? '/feed' : '/')"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-1.5">
          <h1 class="truncate text-lg font-bold leading-tight text-neutral-900">
            {{ profile ? profile.display_name : "…" }}
          </h1>
          <UserBadges v-if="profile" :badges="profile.badges" size="xs" />
        </div>
        <p class="truncate text-sm text-neutral-500">{{ profile ? profileHandleAt : "" }}</p>
      </div>
    </header>

    <p v-if="actionToast" class="border-b border-lime-100 bg-lime-50 px-4 py-2 text-center text-sm text-lime-900">
      {{ actionToast }}
    </p>
    <p v-if="err" class="border-b border-neutral-200 px-4 py-3 text-sm text-red-600">{{ err }}</p>

    <template v-if="profile">
      <div class="relative">
        <div
          class="group relative h-36 w-full overflow-hidden bg-gradient-to-br from-lime-200 via-lime-100 to-neutral-200 sm:h-44"
          :class="profile.is_me ? 'cursor-pointer' : ''"
          :style="safeHeaderStyle(profile.header_url)"
        >
          <template v-if="profile.is_me">
            <input
              id="profile-header-file"
              type="file"
              :accept="SAFE_PROFILE_IMAGE_ACCEPT"
              class="sr-only"
              :disabled="uploadBusy || saving"
              @change="onPickHeaderFile"
            />
            <label
              for="profile-header-file"
              class="absolute inset-0 flex cursor-pointer items-center justify-center bg-black/0 transition-colors group-hover:bg-black/35"
              :aria-label="$t('views.userProfile.changeHeaderAria')"
            >
              <span
                class="inline-flex h-11 w-11 items-center justify-center rounded-full bg-black/50 text-white opacity-0 shadow-lg ring-1 ring-white/25 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
              >
                <Icon name="camera" class="h-5 w-5" />
              </span>
            </label>
          </template>
        </div>
        <div class="relative -mt-12 flex flex-col gap-3 px-4 pb-2">
          <div class="flex w-full items-end gap-3">
            <div
              class="group relative flex h-24 w-24 shrink-0 items-center justify-center overflow-hidden rounded-full border-4 border-white bg-neutral-200 text-xl font-bold text-neutral-700 shadow-sm"
              :class="profile.is_me ? 'cursor-pointer' : ''"
            >
              <img
                v-if="safeProfileImageURL(profile.avatar_url)"
                :src="safeProfileImageURL(profile.avatar_url)"
                alt=""
                referrerpolicy="no-referrer"
                class="h-full w-full object-cover"
              />
              <span v-else>{{ avatarInitials(profile.email ?? profile.display_name) }}</span>
              <template v-if="profile.is_me">
                <input
                  id="profile-avatar-file"
                  type="file"
                  :accept="SAFE_PROFILE_IMAGE_ACCEPT"
                  class="sr-only"
                  :disabled="uploadBusy || saving"
                  @change="onPickAvatarFile"
                />
                <label
                  for="profile-avatar-file"
                  class="absolute inset-0 flex cursor-pointer items-center justify-center rounded-full bg-black/0 transition-colors group-hover:bg-black/35"
                  :aria-label="$t('views.userProfile.changeAvatarAria')"
                >
                  <span
                    class="inline-flex h-10 w-10 items-center justify-center rounded-full bg-black/50 text-white opacity-0 shadow-lg ring-1 ring-white/25 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
                  >
                    <Icon name="camera" class="h-5 w-5" />
                  </span>
                </label>
              </template>
            </div>
            <div class="ml-auto flex shrink-0 items-center gap-2">
              <button
                v-if="!profile.is_me && viewerAuthed"
                type="button"
                class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-neutral-200 bg-white text-lime-700 transition-colors hover:bg-neutral-50 focus-visible:outline focus-visible:ring-2 focus-visible:ring-lime-400 disabled:opacity-50 dark:border-neutral-200 dark:bg-neutral-900 dark:text-lime-400 dark:hover:bg-neutral-800"
                :title="$t('views.userProfile.message')"
                :disabled="dmOpenBusy"
                @click="openDMFromProfile"
              >
                <Icon name="message" class="h-5 w-5" />
                <span class="sr-only">{{ $t("views.userProfile.message") }}</span>
              </button>
              <button
                v-if="!profile.is_me && viewerAuthed"
                type="button"
                class="shrink-0 rounded-full border px-4 py-1.5 text-sm font-semibold transition-colors disabled:opacity-50"
                :class="
                  profile.followed_by_me
                    ? 'border-neutral-200 bg-white text-neutral-800 hover:bg-neutral-50'
                    : 'border-transparent bg-neutral-900 text-white hover:bg-neutral-800'
                "
                :disabled="followBusy || dmOpenBusy"
                @click="toggleFollow"
              >
                {{ profile.followed_by_me ? $t("views.userProfile.following") : $t("views.userProfile.follow") }}
              </button>
              <RouterLink
                v-if="!profile.is_me && !viewerAuthed"
                :to="{ path: '/login', query: { next: route.fullPath } }"
                class="shrink-0 rounded-full border border-neutral-900 bg-neutral-900 px-4 py-1.5 text-sm font-semibold text-white hover:bg-neutral-800"
              >
                {{ $t("views.userProfile.loginToFollow") }}
              </RouterLink>
            </div>
          </div>
          <div>
            <p class="mt-1 text-sm text-neutral-600">
              <RouterLink
                :to="`/@${encodeURIComponent(profile.handle)}/following`"
                class="rounded-md px-1 -mx-1 hover:bg-neutral-100 focus-visible:outline focus-visible:ring-2 focus-visible:ring-lime-400"
              >
                <span class="font-medium tabular-nums text-neutral-800">{{ profile.following_count ?? 0 }}</span>
                {{ $t("views.userProfile.followingLabel") }}
              </RouterLink>
              <span class="mx-1.5 text-neutral-300">·</span>
              <RouterLink
                :to="`/@${encodeURIComponent(profile.handle)}/followers`"
                class="rounded-md px-1 -mx-1 hover:bg-neutral-100 focus-visible:outline focus-visible:ring-2 focus-visible:ring-lime-400"
              >
                <span class="font-medium tabular-nums text-neutral-800">{{ profile.follower_count ?? 0 }}</span>
                {{ $t("views.userProfile.followersLabel") }}
              </RouterLink>
            </p>
            <p v-if="viewerAuthed && profile.follows_you && !profile.is_me" class="mt-1 text-xs text-lime-700">
              {{ $t("views.userProfile.followsYou") }}
            </p>
          </div>
          <template v-if="!profile.is_me">
            <p v-if="profile.bio" class="whitespace-pre-wrap text-[15px] text-neutral-800">
              {{ profile.bio }}
            </p>
            <p v-else class="text-sm text-neutral-400">{{ $t("views.userProfile.bioEmpty") }}</p>
            <div v-if="profileUrlsForDisplay.length" class="mt-2 flex flex-col gap-2">
              <p class="text-xs font-medium text-neutral-500">{{ $t("views.userProfile.externalLinksHeading") }}</p>
              <a
                v-for="u in profileUrlsForDisplay"
                :key="u"
                :href="u"
                target="_blank"
                rel="noopener noreferrer"
                class="flex min-w-0 items-center gap-2 rounded-md py-0.5 text-sm text-lime-700 hover:underline"
              >
                <span
                  class="flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded bg-neutral-100 ring-1 ring-neutral-200/80 dark:bg-neutral-800 dark:ring-neutral-600"
                >
                  <img
                    v-if="!profileLinkFaviconBroken[u] && profileFaviconSrc(u)"
                    :src="profileFaviconSrc(u)"
                    alt=""
                    width="20"
                    height="20"
                    class="h-5 w-5 object-contain"
                    loading="lazy"
                    referrerpolicy="no-referrer"
                    @error="onProfileLinkFaviconError(u)"
                  />
                  <span v-else class="text-[11px] leading-none text-neutral-500 dark:text-neutral-400" aria-hidden="true">&#x1F310;</span>
                </span>
                <span class="min-w-0 break-all">{{ shortUrlLabel(u) }}</span>
              </a>
            </div>
          </template>
          <div v-else class="flex flex-col gap-2">
            <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div class="min-w-0 flex-1">
                <p v-if="profile.bio" class="whitespace-pre-wrap text-[15px] text-neutral-800">{{ profile.bio }}</p>
                <p v-else class="text-sm text-neutral-400">{{ $t("views.userProfile.bioEmpty") }}</p>
              </div>
              <button
                type="button"
                class="shrink-0 self-start rounded-full border border-neutral-200 bg-white px-4 py-1.5 text-sm font-semibold text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
                :disabled="uploadBusy || saving"
                @click="openProfileEditModal"
              >
                {{ $t("views.userProfile.editProfile") }}
              </button>
            </div>
            <div v-if="profileUrlsForDisplay.length" class="flex flex-col gap-2 pt-1">
              <p class="text-xs font-medium text-neutral-500">{{ $t("views.userProfile.externalLinksHeading") }}</p>
              <a
                v-for="u in profileUrlsForDisplay"
                :key="u"
                :href="u"
                target="_blank"
                rel="noopener noreferrer"
                class="flex min-w-0 items-center gap-2 rounded-md py-0.5 text-sm text-lime-700 hover:underline"
              >
                <span
                  class="flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded bg-neutral-100 ring-1 ring-neutral-200/80 dark:bg-neutral-800 dark:ring-neutral-600"
                >
                  <img
                    v-if="!profileLinkFaviconBroken[u] && profileFaviconSrc(u)"
                    :src="profileFaviconSrc(u)"
                    alt=""
                    width="20"
                    height="20"
                    class="h-5 w-5 object-contain"
                    loading="lazy"
                    referrerpolicy="no-referrer"
                    @error="onProfileLinkFaviconError(u)"
                  />
                  <span v-else class="text-[11px] leading-none text-neutral-500 dark:text-neutral-400" aria-hidden="true">&#x1F310;</span>
                </span>
                <span class="min-w-0 break-all">{{ shortUrlLabel(u) }}</span>
              </a>
            </div>
          </div>
        </div>
      </div>

      <div class="sticky top-0 z-[5] border-b border-neutral-200 bg-white/95 px-2 pt-1 backdrop-blur supports-[backdrop-filter]:bg-white/80">
        <div class="profile-tabs-scroll flex flex-nowrap gap-1 overflow-x-auto">
          <button
            type="button"
            class="relative shrink-0 -mb-px border-b-2 px-3 py-2.5 text-sm font-semibold transition-colors"
            :class="
              profilePostTab === 'posts'
                ? 'border-lime-600 text-neutral-900'
                : 'border-transparent text-neutral-500 hover:text-neutral-800'
            "
            @click="profilePostTab = 'posts'"
          >
            {{ $t("views.userProfile.tabPosts") }}
          </button>
          <button
            type="button"
            class="relative shrink-0 -mb-px border-b-2 px-3 py-2.5 text-sm font-semibold transition-colors"
            :class="
              profilePostTab === 'replies'
                ? 'border-lime-600 text-neutral-900'
                : 'border-transparent text-neutral-500 hover:text-neutral-800'
            "
            @click="profilePostTab = 'replies'"
          >
            {{ $t("views.userProfile.tabReplies") }}
          </button>
          <button
            type="button"
            class="relative shrink-0 -mb-px border-b-2 px-3 py-2.5 text-sm font-semibold transition-colors"
            :class="
              profilePostTab === 'media'
                ? 'border-lime-600 text-neutral-900'
                : 'border-transparent text-neutral-500 hover:text-neutral-800'
            "
            @click="profilePostTab = 'media'"
          >
            {{ $t("views.userProfile.tabMedia") }}
          </button>
        </div>
      </div>
      <template v-if="profilePostTab === 'posts'">
        <p v-if="!posts.length" class="border-b border-neutral-200 px-4 py-10 text-center text-sm text-neutral-500">
          {{ $t("views.userProfile.emptyPosts") }}
        </p>
        <PostTimeline
          v-else
          :items="posts"
          :thread-replies-by-root="threadRepliesByRoot"
          :action-busy="actionBusy"
          :viewer-email="myEmail"
          show-federated-reply-action
          show-federated-repost-action
          show-profile-pin-action
          @reply="onReply"
          @toggle-reaction="toggleReaction"
          @toggle-bookmark="toggleBookmark"
          @toggle-profile-pin="toggleProfilePin"
          @toggle-repost="onToggleRepost"
          @share="sharePost"
          @open-lightbox="openLightbox"
          @patch-item="({ id, patch }) => patchPost(id, patch)"
          @remove-post="removePost"
        />
      </template>
      <template v-else-if="profilePostTab === 'replies'">
        <p v-if="replyPostsBusy" class="border-b border-neutral-200 px-4 py-10 text-center text-sm text-neutral-500">
          {{ $t("app.loading") }}
        </p>
        <p v-else-if="replyPostsErr" class="border-b border-neutral-200 px-4 py-8 text-center text-sm text-red-600">
          {{ replyPostsErr }}
        </p>
        <p v-else-if="!replyPosts.length" class="border-b border-neutral-200 px-4 py-10 text-center text-sm text-neutral-500">
          {{ $t("views.userProfile.emptyReplies") }}
        </p>
        <PostTimeline
          v-else
          :items="replyPosts"
          :action-busy="actionBusy"
          :viewer-email="myEmail"
          show-reply-parent-link
          show-federated-reply-action
          show-federated-repost-action
          @reply="onReply"
          @toggle-reaction="toggleReaction"
          @toggle-bookmark="toggleBookmark"
          @toggle-repost="onToggleRepost"
          @share="sharePost"
          @open-lightbox="openLightbox"
          @patch-item="({ id, patch }) => patchPost(id, patch)"
          @remove-post="removePost"
        />
      </template>
      <template v-else-if="profilePostTab === 'media'">
        <p v-if="mediaTilesBusy" class="border-b border-neutral-200 px-4 py-10 text-center text-sm text-neutral-500">
          {{ $t("app.loading") }}
        </p>
        <p v-else-if="mediaTilesErr" class="border-b border-neutral-200 px-4 py-8 text-center text-sm text-red-600">
          {{ mediaTilesErr }}
        </p>
        <p v-else-if="!mediaTiles.length" class="border-b border-neutral-200 px-4 py-10 text-center text-sm text-neutral-500">
          {{ $t("views.userProfile.emptyMedia") }}
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
    </template>
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
        <p v-if="lightbox.urls.length > 1" class="pb-3 text-center text-xs text-white/60 sm:hidden">{{ $t("views.feed.lightboxSwipeHint") }}</p>
      </div>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="cropKind && cropSrc"
      class="fixed inset-0 z-[110] flex items-start justify-center overflow-y-auto bg-black/60 p-4 pt-10 sm:pt-16"
      role="dialog"
      aria-modal="true"
      :aria-label="cropKind === 'avatar' ? $t('views.userProfile.cropAvatarAria') : $t('views.userProfile.cropHeaderAria')"
      @click.self="closeCropModal"
    >
      <div
        class="relative w-full max-w-2xl rounded-2xl border border-neutral-200 bg-white p-4 shadow-xl dark:border-neutral-200 dark:bg-neutral-900"
      >
        <h2 class="text-lg font-bold text-neutral-900 dark:text-neutral-100">{{ $t("views.userProfile.cropTitle") }}</h2>
        <p class="mt-1 text-sm text-neutral-500">
          {{ cropKind === "avatar" ? $t("views.userProfile.cropHintAvatar") : $t("views.userProfile.cropHintHeader") }}
        </p>
        <div class="mt-3 max-h-[min(65vh,520px)] w-full overflow-hidden rounded-lg bg-neutral-100 dark:bg-neutral-800">
          <img
            ref="cropImageRef"
            :key="cropKey"
            :src="cropSrc"
            alt=""
            class="block max-h-[min(65vh,520px)] w-full"
            @load="onCropImageLoad"
          />
        </div>
        <div class="mt-4 flex flex-wrap items-center justify-end gap-2">
          <button
            type="button"
            class="rounded-full border border-neutral-200 px-4 py-2 text-sm font-medium text-neutral-700 hover:bg-neutral-50 dark:border-neutral-200 dark:text-neutral-200 dark:hover:bg-neutral-800"
            :disabled="uploadBusy"
            @click="closeCropModal"
          >
            {{ $t("views.userProfile.cancel") }}
          </button>
          <button
            type="button"
            class="rounded-full bg-lime-500 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-600 disabled:opacity-50"
            :disabled="uploadBusy"
            @click="confirmCrop"
          >
            {{ uploadBusy ? $t("views.userProfile.cropUploading") : $t("views.userProfile.cropApply") }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="profileEditModalOpen && profile?.is_me"
      class="fixed inset-0 z-[115] flex items-start justify-center overflow-y-auto bg-black/60 p-4 pt-10 sm:items-center sm:pt-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="profile-edit-title"
      @click.self="closeProfileEditModal"
    >
      <div
        class="relative w-full max-w-md rounded-2xl border border-neutral-200 bg-white p-4 shadow-xl dark:border-neutral-200 dark:bg-neutral-900"
      >
        <div class="flex items-start justify-between gap-2">
          <h2 id="profile-edit-title" class="text-lg font-bold text-neutral-900 dark:text-neutral-100">
            {{ $t("views.userProfile.editModalTitle") }}
          </h2>
          <button
            type="button"
            class="rounded-full p-1.5 text-neutral-500 hover:bg-neutral-100 dark:hover:bg-neutral-800"
            :aria-label="$t('views.userProfile.closeAria')"
            :disabled="saving"
            @click="closeProfileEditModal"
          >
            <Icon name="close" class="h-5 w-5" :stroke-width="2" />
          </button>
        </div>
        <p v-if="profileModalErr" class="mt-2 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-200">
          {{ profileModalErr }}
        </p>
        <div class="mt-4 space-y-3">
          <div>
            <label class="block text-xs font-medium text-neutral-600 dark:text-neutral-400">{{ $t("views.userProfile.displayNameLabel") }}</label>
            <input
              v-model="editDisplayName"
              type="text"
              maxlength="50"
              class="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 focus:border-lime-400 focus:outline-none focus:ring-1 focus:ring-lime-400 dark:border-neutral-200 dark:bg-neutral-950 dark:text-neutral-100"
              :placeholder="$t('views.userProfile.displayNamePlaceholder')"
              autocomplete="nickname"
            />
            <p class="mt-1 text-xs text-neutral-500">{{ $t("views.userProfile.displayNameHint") }}</p>
          </div>
          <div>
            <label class="block text-xs font-medium text-neutral-600 dark:text-neutral-400">{{ $t("views.userProfile.handleLabel") }}</label>
            <div
              class="mt-1 flex items-center gap-1 rounded-lg border border-neutral-200 bg-white px-3 py-2 focus-within:border-lime-400 focus-within:ring-1 focus-within:ring-lime-400 dark:border-neutral-200 dark:bg-neutral-950"
            >
              <span class="shrink-0 text-sm text-neutral-500">@</span>
              <input
                v-model="editHandle"
                type="text"
                maxlength="30"
                class="min-w-0 flex-1 border-0 bg-transparent p-0 text-sm text-neutral-900 outline-none dark:text-neutral-100"
                :placeholder="$t('views.userProfile.handlePlaceholder')"
                spellcheck="false"
                autocapitalize="off"
                autocomplete="username"
              />
            </div>
            <p class="mt-1 text-xs text-neutral-500">
              {{ $t("views.userProfile.handleHint") }}
            </p>
          </div>
          <div>
            <label class="block text-xs font-medium text-neutral-600 dark:text-neutral-400">{{ $t("views.userProfile.bioLabel") }}</label>
            <textarea
              v-model="editBio"
              rows="4"
              maxlength="500"
              class="mt-1 w-full resize-none rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 focus:border-lime-400 focus:outline-none focus:ring-1 focus:ring-lime-400 dark:border-neutral-200 dark:bg-neutral-950 dark:text-neutral-100"
              :placeholder="$t('views.userProfile.bioPlaceholder')"
            />
          </div>
          <div class="space-y-2">
            <label class="flex items-start gap-3 rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-800 dark:border-neutral-200 dark:bg-neutral-950 dark:text-neutral-100">
              <input
                v-model="editIsBot"
                type="checkbox"
                class="mt-0.5 h-4 w-4 rounded border-neutral-300 text-lime-600 focus:ring-lime-500"
              />
              <span>
                <span class="block font-medium">{{ $t("views.userProfile.botCheckboxLabel") }}</span>
                <span class="mt-0.5 block text-xs text-neutral-500 dark:text-neutral-400">{{ $t("views.userProfile.botCheckboxHint") }}</span>
              </span>
            </label>
            <label class="flex items-start gap-3 rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-800 dark:border-neutral-200 dark:bg-neutral-950 dark:text-neutral-100">
              <input
                v-model="editIsAI"
                type="checkbox"
                class="mt-0.5 h-4 w-4 rounded border-neutral-300 text-lime-600 focus:ring-lime-500"
              />
              <span>
                <span class="block font-medium">{{ $t("views.userProfile.aiCheckboxLabel") }}</span>
                <span class="mt-0.5 block text-xs text-neutral-500 dark:text-neutral-400">{{ $t("views.userProfile.aiCheckboxHint") }}</span>
              </span>
            </label>
          </div>
          <div>
            <label class="block text-xs font-medium text-neutral-600 dark:text-neutral-400">{{
              $t("views.userProfile.externalUrlsLabel")
            }}</label>
            <p class="mt-1 text-xs text-neutral-500 dark:text-neutral-400">{{ $t("views.userProfile.externalUrlsHint") }}</p>
            <div class="mt-2 space-y-2">
              <div v-for="(_, i) in editProfileUrls" :key="i" class="flex items-center gap-2">
                <input
                  v-model="editProfileUrls[i]"
                  type="text"
                  inputmode="url"
                  class="min-w-0 flex-1 rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 focus:border-lime-400 focus:outline-none focus:ring-1 focus:ring-lime-400 dark:border-neutral-200 dark:bg-neutral-950 dark:text-neutral-100"
                  :placeholder="$t('views.userProfile.externalUrlPlaceholder')"
                  spellcheck="false"
                  autocapitalize="off"
                  autocomplete="url"
                />
                <button
                  v-if="editProfileUrls.length < 5 && i === editProfileUrls.length - 1"
                  type="button"
                  class="shrink-0 rounded-full border border-neutral-200 bg-white px-3 py-2 text-base font-semibold leading-none text-neutral-800 hover:bg-neutral-50 disabled:opacity-50 dark:border-neutral-200 dark:bg-neutral-900 dark:text-neutral-100 dark:hover:bg-neutral-800"
                  :disabled="saving"
                  :aria-label="$t('views.userProfile.addExternalUrlAria')"
                  @click="addProfileUrlField"
                >
                  +
                </button>
              </div>
            </div>
          </div>
        </div>
        <div class="mt-5 flex flex-wrap justify-end gap-2">
          <button
            type="button"
            class="rounded-full border border-neutral-200 px-4 py-2 text-sm font-medium text-neutral-700 hover:bg-neutral-50 disabled:opacity-50 dark:border-neutral-200 dark:text-neutral-200 dark:hover:bg-neutral-800"
            :disabled="saving"
            @click="closeProfileEditModal"
          >
            {{ $t("views.userProfile.cancel") }}
          </button>
          <button
            type="button"
            class="rounded-full bg-lime-500 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-600 disabled:opacity-50"
            :disabled="saving || uploadBusy"
            @click="saveProfile"
          >
            {{ saving ? $t("views.userProfile.saving") : $t("views.userProfile.save") }}
          </button>
        </div>
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

<style scoped>
/* Allow horizontal tab scrolling while keeping scrollbars hidden. */
.profile-tabs-scroll {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.profile-tabs-scroll::-webkit-scrollbar {
  display: none;
  width: 0;
  height: 0;
}
</style>
