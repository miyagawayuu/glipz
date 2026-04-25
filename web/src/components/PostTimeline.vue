<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, reactive, ref, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import EmojiInline from "./EmojiInline.vue";
import Icon from "./Icon.vue";
import GlipzAudioPlayer from "./GlipzAudioPlayer.vue";
import GlipzVideoPlayer from "./GlipzVideoPlayer.vue";
import PostRichText from "./PostRichText.vue";
import PostThreadEmbed from "./PostThreadEmbed.vue";
import UserBadges from "./UserBadges.vue";
import { buildReplyTree, depthIndentPxByPostId, flatRepliesDFS } from "../lib/threadTree";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";
import { customEmojiMap, ensureCustomEmojiCatalog, pickerCustomEmojisForHandle, unicodeReactionPickerCategories } from "../lib/customEmojis";
import { blockFederationUser, muteFederationUser } from "../lib/federationPrivacy";
import { requestGumroadEntitlement } from "../lib/fanclubGumroad";
import { requestPatreonEntitlement, requestPatreonEntitlementFederated } from "../lib/fanclubPatreon";
import { unlockTimelinePost, voteTimelinePoll } from "../lib/federationActions";
import type { TimelinePoll, TimelinePost } from "../types/timeline";
import {
  avatarInitials,
  displayName as displayNameFromEmail,
  federatedRemoteProfilePath,
  formatActionCount,
  formatAbsoluteDateTime,
  formatPostTime,
  fullHandleAt,
  gridSlots,
  postPublishedAtISO,
  postDetailPath,
  handleAt,
  mediaIndexFromGridSlot,
  profilePath,
  timelineDisplayName,
} from "../lib/feedDisplay";
import {
  buildViewPasswordScope,
  codeUnitOffsetToRuneIndex,
  normalizeViewPasswordRanges,
  sliceRunes,
  VIEW_PASSWORD_SCOPE_ALL,
  scopeProtectsMedia,
  scopeProtectsText,
  viewPasswordScopeSummary,
  type ViewPasswordScopeLabels,
} from "../lib/viewPassword";

const props = withDefaults(
  defineProps<{
    items: TimelinePost[];
    actionBusy: string | null;
    /** Logged-in user. Falls back to App-level `me` when omitted. */
    viewerEmail?: string | null;
    /** Admin flag. Falls back to App-level `me` when omitted. */
    viewerIsAdmin?: boolean | null;
    /** Hides links to the post detail page, used when already on that page. */
    hidePostDetailLink?: boolean;
    /** Maps root IDs to flat reply lists that will be treeified via reply_to_post_id. */
    threadRepliesByRoot?: Record<string, TimelinePost[]> | null;
    /** When false, child-thread containers are hidden for embedded use cases. */
    embedThreadReplies?: boolean;
    /** Maps embedded-thread post IDs to left-indent pixel values. */
    threadArticleIndentByPostId?: Record<string, number> | null;
    /** Shows parent-post links in reply timelines when reply_to_post_id is present. */
    showReplyParentLink?: boolean;
    /** Shows the original post link only on detail pages. */
    showRemoteObjectLink?: boolean;
    /** Shows the reply action even for federated posts. */
    showFederatedReplyAction?: boolean;
    /** Shows the repost action even for federated posts. */
    showFederatedRepostAction?: boolean;
  }>(),
  {
    viewerIsAdmin: null,
    hidePostDetailLink: false,
    threadRepliesByRoot: null,
    embedThreadReplies: true,
    threadArticleIndentByPostId: null,
    showReplyParentLink: false,
    showRemoteObjectLink: false,
    showFederatedReplyAction: false,
    showFederatedRepostAction: false,
  },
);

const { t } = useI18n();

const viewPasswordLabels = computed<ViewPasswordScopeLabels>(() => ({
  all: t("components.postTimeline.viewPasswordScope.all"),
  text: t("components.postTimeline.viewPasswordScope.text"),
  media: t("components.postTimeline.viewPasswordScope.media"),
  sep: t("components.postTimeline.viewPasswordScope.sep"),
}));

const appMe = inject<Ref<{ email: string; handle?: string; is_site_admin?: boolean } | null> | null>("appMe", null);
const effectiveViewerEmail = computed(() =>
  props.viewerEmail !== undefined ? props.viewerEmail : (appMe?.value?.email ?? null),
);
const effectiveViewerIsAdmin = computed(() =>
  props.viewerIsAdmin !== null ? Boolean(props.viewerIsAdmin) : Boolean(appMe?.value?.is_site_admin),
);

const threadBlocks = computed(() => {
  const map: Record<string, { flatOrdered: TimelinePost[]; indents: Record<string, number> }> = {};
  if (!props.embedThreadReplies) return map;
  const roots = props.threadRepliesByRoot;
  if (!roots) return map;
  for (const it of props.items) {
    const flat = roots[it.id];
    if (!flat?.length) continue;
    const tree = buildReplyTree(it.id, flat);
    map[it.id] = {
      flatOrdered: flatRepliesDFS(tree),
      indents: depthIndentPxByPostId(tree, 12),
    };
  }
  return map;
});

function threadArticleIndentClass(it: TimelinePost): string {
  const px = props.threadArticleIndentByPostId?.[it.id];
  if (px == null || px <= 0) return "";
  return "border-l-2 border-neutral-200 pl-3";
}

function threadArticleIndentStyle(it: TimelinePost): Record<string, string> | undefined {
  const px = props.threadArticleIndentByPostId?.[it.id];
  if (px == null || px <= 0) return undefined;
  return { marginLeft: `${px}px` };
}

/** Accordion state for per-root reply threads. Closed by default. */
const threadAccordionOpen = reactive<Record<string, boolean>>({});

function isThreadAccordionOpen(rootId: string): boolean {
  return Boolean(threadAccordionOpen[rootId]);
}

function toggleThreadAccordion(rootId: string) {
  threadAccordionOpen[rootId] = !threadAccordionOpen[rootId];
}

const emit = defineEmits<{
  reply: [it: TimelinePost];
  toggleReaction: [it: TimelinePost, emoji: string];
  toggleBookmark: [it: TimelinePost];
  toggleRepost: [it: TimelinePost];
  share: [it: TimelinePost];
  openLightbox: [urls: string[], index: number];
  patchItem: [payload: { id: string; patch: Partial<TimelinePost> }];
  removePost: [id: string];
}>();

function isOwnPost(it: TimelinePost) {
  return Boolean(effectiveViewerEmail.value && it.user_email === effectiveViewerEmail.value);
}

const openMenuId = ref<string | null>(null);
const federationPrivacyBusyId = ref<string | null>(null);
const openReactionPickerId = ref<string | null>(null);
const emojiCatalog = customEmojiMap();
const standardReactionCategories = unicodeReactionPickerCategories();
const customReactionOptions = computed(() => pickerCustomEmojisForHandle(appMe?.value?.handle));

function reactionCategoryLabel(slug: string): string {
  return t(`components.postTimeline.reactionCategories.${slug}`);
}
function closePostMenu() {
  openMenuId.value = null;
  openReactionPickerId.value = null;
}
function togglePostMenu(id: string) {
  openMenuId.value = openMenuId.value === id ? null : id;
}

function toggleReactionPicker(id: string) {
  openMenuId.value = null;
  openReactionPickerId.value = openReactionPickerId.value === id ? null : id;
}

function selectReaction(it: TimelinePost, emoji: string) {
  openReactionPickerId.value = null;
  emit("toggleReaction", it, emoji);
}

function hasVisibleReactions(it: TimelinePost): boolean {
  return Array.isArray(it.reactions) && it.reactions.length > 0;
}

onMounted(() => {
  document.addEventListener("click", closePostMenu);
  void ensureCustomEmojiCatalog(getAccessToken());
});
onBeforeUnmount(() => {
  document.removeEventListener("click", closePostMenu);
});

const editTarget = ref<TimelinePost | null>(null);
const editCaption = ref("");
const editCaptionEl = ref<HTMLTextAreaElement | null>(null);
const editIsNsfw = ref(false);
const editVisibility = ref<"public" | "logged_in" | "followers" | "private">("public");
const editClearPassword = ref(false);
const editPassword = ref("");
const editPasswordConfirm = ref("");
const editProtectText = ref(false);
const editProtectMedia = ref(false);
const editProtectAll = ref(false);
const editTextRanges = ref<{ start: number; end: number }[]>([]);
const editBusy = ref(false);
const editErr = ref("");
const reportTarget = ref<TimelinePost | null>(null);
const reportReason = ref("");
const reportBusy = ref(false);
const reportErr = ref("");

const nsfwRevealedIds = ref(new Set<string>());
const ageGatePostId = ref<string | null>(null);
const unlockPwd = reactive<Record<string, string>>({});
const unlockGumroadLicense = reactive<Record<string, string>>({});
const unlockErr = reactive<Record<string, string>>({});
const unlockBusy = ref<string | null>(null);
const pollBusy = ref<string | null>(null);

/** Falls back to initials when an avatar image URL fails to load. */
const avatarLoadFailed = reactive<Record<string, boolean>>({});
function onAvatarLoadError(id: string) {
  avatarLoadFailed[id] = true;
}

function feedRowKey(it: TimelinePost): string {
  const k = it.feed_entry_id?.trim();
  return k && k.length > 0 ? k : it.id;
}

function formatRepostedAt(iso: string): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return "";
  return formatAbsoluteDateTime(t, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

function hasCommentedRepost(it: TimelinePost): boolean {
  return Boolean(it.repost?.comment?.trim());
}

function repostCommentText(it: TimelinePost): string {
  return it.repost?.comment?.trim() ?? "";
}

function rowActorEmail(it: TimelinePost): string {
  if (hasCommentedRepost(it) && it.repost?.user_email) return it.repost.user_email;
  return it.user_email;
}

function rowActorAvatarUrl(it: TimelinePost): string | undefined {
  if (hasCommentedRepost(it) && it.repost?.user_avatar_url) return it.repost.user_avatar_url;
  return it.user_avatar_url;
}

function rowActorDisplayName(it: TimelinePost): string {
  if (hasCommentedRepost(it) && it.repost) {
    return it.repost.user_display_name?.trim() || displayNameFromEmail(it.repost.user_email);
  }
  return timelineDisplayName(it);
}

function rowActorBadges(it: TimelinePost): string[] {
  if (hasCommentedRepost(it) && it.repost?.user_badges) return it.repost.user_badges;
  return it.user_badges ?? [];
}

function rowActorHandleLabel(it: TimelinePost): string {
  if (hasCommentedRepost(it) && it.repost) {
    return fullHandleAt(it.repost.user_handle || it.repost.user_email.split("@")[0] || "user");
  }
  return handleAt(it);
}

function rowActorProfileRoute(it: TimelinePost): string | null {
  if (hasCommentedRepost(it) && it.repost) {
    return profilePath({ user_handle: it.repost.user_handle, user_email: it.repost.user_email });
  }
  return it.is_federated ? federatedRemoteProfilePath(it) : profilePath(it);
}

function rowPublishedAtISO(it: TimelinePost): string {
  if (hasCommentedRepost(it) && it.repost?.reposted_at) return it.repost.reposted_at;
  return postPublishedAtISO(it);
}

function rowPublishedAtLabel(it: TimelinePost): string {
  if (hasCommentedRepost(it) && it.repost?.reposted_at) return formatRepostedAt(it.repost.reposted_at);
  return formatPostTime(it);
}

function mainCaptionText(it: TimelinePost): string {
  if (hasCommentedRepost(it)) return repostCommentText(it);
  return it.caption;
}

function originalPostProfileRoute(it: TimelinePost): string | null {
  return it.is_federated ? federatedRemoteProfilePath(it) : profilePath(it);
}

function openEdit(it: TimelinePost) {
  openMenuId.value = null;
  editTarget.value = it;
  editCaption.value = it.caption ?? "";
  editIsNsfw.value = Boolean(it.is_nsfw);
  editVisibility.value =
    it.visibility === "logged_in" || it.visibility === "followers" || it.visibility === "private"
      ? it.visibility
      : "public";
  editClearPassword.value = false;
  editPassword.value = "";
  editPasswordConfirm.value = "";
  editProtectAll.value = (it.view_password_scope ?? 0) === VIEW_PASSWORD_SCOPE_ALL;
  editProtectText.value = Boolean(it.view_password_scope && scopeProtectsText(it.view_password_scope) && (it.view_password_scope ?? 0) !== VIEW_PASSWORD_SCOPE_ALL);
  editProtectMedia.value = Boolean(it.view_password_scope && scopeProtectsMedia(it.view_password_scope) && (it.view_password_scope ?? 0) !== VIEW_PASSWORD_SCOPE_ALL);
  editTextRanges.value = (it.view_password_text_ranges ?? []).map((rg) => ({ start: rg.start, end: rg.end }));
  editErr.value = "";
}

function closeEditModal() {
  editTarget.value = null;
  editErr.value = "";
}

function editViewPasswordScope(): number {
  return buildViewPasswordScope({
    protectAll: editProtectAll.value,
    protectText: editProtectText.value,
    protectMedia: editProtectMedia.value,
  });
}

function toggleEditProtectAll() {
  editProtectAll.value = !editProtectAll.value;
  if (editProtectAll.value) {
    editProtectText.value = false;
    editProtectMedia.value = false;
    editTextRanges.value = [];
  }
}

function toggleEditProtectText() {
  editProtectText.value = !editProtectText.value;
  if (editProtectText.value) {
    editProtectAll.value = false;
  } else {
    editTextRanges.value = [];
  }
}

function toggleEditProtectMedia() {
  editProtectMedia.value = !editProtectMedia.value;
  if (editProtectMedia.value) {
    editProtectAll.value = false;
  }
}

function addEditSelectedTextRange() {
  const el = editCaptionEl.value;
  if (!el) {
    editErr.value = t("views.compose.errors.selectTextFromCaption");
    return;
  }
  const startOffset = Math.min(el.selectionStart ?? 0, el.selectionEnd ?? 0);
  const endOffset = Math.max(el.selectionStart ?? 0, el.selectionEnd ?? 0);
  if (startOffset === endOffset) {
    editErr.value = t("views.compose.errors.selectTextFromCaption");
    return;
  }
  editProtectText.value = true;
  editProtectAll.value = false;
  editTextRanges.value = normalizeViewPasswordRanges([
    ...editTextRanges.value,
    {
      start: codeUnitOffsetToRuneIndex(editCaption.value, startOffset),
      end: codeUnitOffsetToRuneIndex(editCaption.value, endOffset),
    },
  ]);
  editErr.value = "";
}

function removeEditTextRange(idx: number) {
  editTextRanges.value = editTextRanges.value.filter((_, i) => i !== idx);
}

function editTextRangePreview(rg: { start: number; end: number }): string {
  const text = sliceRunes(editCaption.value, rg.start, rg.end).trim();
  return text || t("views.compose.whitespaceOnly");
}

function lockedSummary(it: TimelinePost): string {
  return (
    viewPasswordScopeSummary(it.view_password_scope ?? 0, viewPasswordLabels.value) || viewPasswordLabels.value.all
  );
}

function visibilityLabel(it: TimelinePost): string {
  if (it.visibility === "logged_in") return t("views.compose.visibility.loggedIn.label");
  if (it.visibility === "followers") return t("views.compose.visibility.followers.label");
  if (it.visibility === "private") return t("views.compose.visibility.private.label");
  return t("views.compose.visibility.public.label");
}

function canRepost(it: TimelinePost): boolean {
  return (it.visibility ?? "public") === "public";
}

function canReportPost(it: TimelinePost): boolean {
  return Boolean(effectiveViewerEmail.value) && !isOwnPost(it);
}

function federatedPrivacyTargetAcct(it: TimelinePost): string | null {
  if (!it.is_federated) return null;
  const a = it.user_handle?.trim().toLowerCase() ?? "";
  if (!a.includes("@")) return null;
  return a;
}

function canFederationPrivacyMenu(it: TimelinePost): boolean {
  return Boolean(effectiveViewerEmail.value) && Boolean(federatedPrivacyTargetAcct(it)) && !isOwnPost(it);
}

function canAdminDeletePost(it: TimelinePost): boolean {
  return effectiveViewerIsAdmin.value && !it.is_federated && !isOwnPost(it);
}

function canAdminSuspendAuthor(it: TimelinePost): boolean {
  return effectiveViewerIsAdmin.value && !it.is_federated && !isOwnPost(it);
}

function canShowPostMenu(it: TimelinePost): boolean {
  return (
    isOwnPost(it) ||
    canReportPost(it) ||
    canAdminDeletePost(it) ||
    canAdminSuspendAuthor(it) ||
    canFederationPrivacyMenu(it)
  );
}

function moderationErrorMessage(e: unknown, fallback: string): string {
  const msg = e instanceof Error ? e.message : "";
  if (msg === "cannot_report_own_post") return t("components.postTimeline.moderation.cannotReportOwnPost");
  if (msg === "report_reason_required") return t("components.postTimeline.moderation.reportReasonRequired");
  if (msg === "report_reason_too_long") return t("components.postTimeline.moderation.reportReasonTooLong");
  if (msg === "cannot_suspend_self") return t("components.postTimeline.moderation.cannotSuspendSelf");
  if (msg === "cannot_suspend_site_admin") return t("components.postTimeline.moderation.cannotSuspendSiteAdmin");
  if (msg === "account_suspended") return t("components.postTimeline.moderation.accountSuspended");
  if (msg === "forbidden") return t("components.postTimeline.moderation.forbidden");
  if (msg === "not_found") return t("components.postTimeline.moderation.notFound");
  return fallback;
}

async function submitEdit() {
  const it = editTarget.value;
  if (!it) return;
  const token = getAccessToken();
  if (!token) return;

  if (!editClearPassword.value) {
    const p = editPassword.value.trim();
    const p2 = editPasswordConfirm.value.trim();
    if (p || p2) {
      if (p !== p2) {
        editErr.value = t("views.compose.errors.editPasswordMismatch");
        return;
      }
      if (p.length < 4 || p.length > 72) {
        editErr.value = t("views.compose.errors.viewPasswordLength");
        return;
      }
    }
    const scope = editViewPasswordScope();
    if ((it.has_view_password || p || p2) && scope === 0) {
      editErr.value = t("views.compose.errors.editProtectScopeRequired");
      return;
    }
    if (editProtectText.value && !editProtectAll.value && editTextRanges.value.length === 0) {
      editErr.value = t("views.compose.errors.editProtectRangesRequired");
      return;
    }
  }

  const body: Record<string, unknown> = {
    caption: editCaption.value,
    is_nsfw: editIsNsfw.value,
    visibility: editVisibility.value,
    clear_view_password: editClearPassword.value,
  };
  if (!editClearPassword.value) {
    const p = editPassword.value.trim();
    if (p) {
      body.view_password = p;
    }
    body.view_password_scope = editViewPasswordScope();
    if (editProtectText.value && !editProtectAll.value) {
      body.view_password_text_ranges = editTextRanges.value;
    }
  }

  editBusy.value = true;
  editErr.value = "";
  try {
    const res = await api<{ item: Record<string, unknown> }>(`/api/v1/posts/${it.id}`, {
      method: "PATCH",
      token,
      json: body,
    });
    const raw = res.item;
    emit("patchItem", {
      id: it.id,
      patch: {
        caption: String(raw.caption ?? ""),
        media_type: String(raw.media_type ?? it.media_type),
        media_urls: Array.isArray(raw.media_urls) ? (raw.media_urls as string[]) : [],
        is_nsfw: Boolean(raw.is_nsfw),
        visibility:
          raw.visibility === "logged_in" || raw.visibility === "followers" || raw.visibility === "private"
            ? raw.visibility
            : "public",
        has_view_password: Boolean(raw.has_view_password),
        view_password_scope: typeof raw.view_password_scope === "number" ? raw.view_password_scope : 0,
        view_password_text_ranges: Array.isArray(raw.view_password_text_ranges)
          ? raw.view_password_text_ranges
              .map((rg) => {
                if (!rg || typeof rg !== "object") return null;
                const row = rg as { start?: number; end?: number };
                return typeof row.start === "number" && typeof row.end === "number"
                  ? { start: row.start, end: row.end }
                  : null;
              })
              .filter((rg): rg is { start: number; end: number } => rg != null)
          : [],
        content_locked: Boolean(raw.content_locked),
        text_locked: Boolean(raw.text_locked),
        media_locked: Boolean(raw.media_locked),
      },
    });
    if (!editIsNsfw.value) {
      const next = new Set(nsfwRevealedIds.value);
      next.delete(it.id);
      nsfwRevealedIds.value = next;
    }
    closeEditModal();
  } catch (e: unknown) {
    editErr.value = e instanceof Error ? e.message : t("views.compose.errors.updateFailed");
  } finally {
    editBusy.value = false;
  }
}

async function requestDeletePost(it: TimelinePost) {
  openMenuId.value = null;
  const confirmMessage = isOwnPost(it)
    ? t("components.postTimeline.moderation.deleteOwnConfirm")
    : t("components.postTimeline.moderation.deleteAdminConfirm");
  if (!window.confirm(confirmMessage)) {
    return;
  }
  const token = getAccessToken();
  if (!token) return;
  try {
    await api(`/api/v1/posts/${it.id}`, { method: "DELETE", token });
    emit("removePost", it.id);
  } catch (e: unknown) {
    window.alert(moderationErrorMessage(e, t("components.postTimeline.moderation.deleteFailed")));
  }
}

async function requestFederationMute(it: TimelinePost) {
  openMenuId.value = null;
  const acct = federatedPrivacyTargetAcct(it);
  if (!acct) return;
  const token = getAccessToken();
  if (!token) return;
  if (!window.confirm(t("components.postTimeline.federationPrivacy.muteConfirm"))) return;
  federationPrivacyBusyId.value = it.id;
  try {
    await muteFederationUser(token, acct);
    window.alert(t("components.postTimeline.federationPrivacy.doneMute"));
    emit("removePost", it.id);
  } catch (e: unknown) {
    window.alert(e instanceof Error ? e.message : t("components.postTimeline.federationPrivacy.failed"));
  } finally {
    federationPrivacyBusyId.value = null;
  }
}

async function requestFederationBlock(it: TimelinePost) {
  openMenuId.value = null;
  const acct = federatedPrivacyTargetAcct(it);
  if (!acct) return;
  const token = getAccessToken();
  if (!token) return;
  if (!window.confirm(t("components.postTimeline.federationPrivacy.blockConfirm"))) return;
  federationPrivacyBusyId.value = it.id;
  try {
    await blockFederationUser(token, acct);
    window.alert(t("components.postTimeline.federationPrivacy.doneBlock"));
    emit("removePost", it.id);
  } catch (e: unknown) {
    window.alert(e instanceof Error ? e.message : t("components.postTimeline.federationPrivacy.failed"));
  } finally {
    federationPrivacyBusyId.value = null;
  }
}

async function requestReportPost(it: TimelinePost) {
  openMenuId.value = null;
  reportTarget.value = it;
  reportReason.value = "";
  reportErr.value = "";
}

function closeReportModal() {
  if (reportBusy.value) return;
  reportTarget.value = null;
  reportReason.value = "";
  reportErr.value = "";
}

async function submitReport() {
  const it = reportTarget.value;
  if (!it) return;
  const token = getAccessToken();
  if (!token) return;
  const reason = reportReason.value.trim();
  if (!reason) {
    reportErr.value = t("components.postTimeline.moderation.reportReasonRequired");
    return;
  }
  reportBusy.value = true;
  reportErr.value = "";
  const base = it.is_federated
    ? `/api/v1/federation/posts/${encodeURIComponent(it.id.replace(/^federated:/, ""))}/report`
    : `/api/v1/posts/${encodeURIComponent(it.id)}/report`;
  const path = it.is_federated && it.remote_object_url
    ? `${base}?${new URLSearchParams({ object_url: it.remote_object_url }).toString()}`
    : base;
  try {
    await api(path, { method: "POST", token, json: { reason } });
    closeReportModal();
    window.alert(t("components.postTimeline.moderation.reportAccepted"));
  } catch (e: unknown) {
    reportErr.value = moderationErrorMessage(e, t("components.postTimeline.moderation.reportFailed"));
  } finally {
    reportBusy.value = false;
  }
}

async function requestSuspendAuthor(it: TimelinePost) {
  openMenuId.value = null;
  if (!window.confirm(t("components.postTimeline.moderation.suspendConfirm"))) {
    return;
  }
  const token = getAccessToken();
  if (!token) return;
  try {
    await api(`/api/v1/admin/posts/${encodeURIComponent(it.id)}/suspend-author`, { method: "POST", token });
    window.alert(t("components.postTimeline.moderation.suspendDone"));
  } catch (e: unknown) {
    window.alert(moderationErrorMessage(e, t("components.postTimeline.moderation.suspendFailed")));
  }
}

function nsfwRevealed(id: string) {
  return nsfwRevealedIds.value.has(id);
}

function confirmAgeGate() {
  const id = ageGatePostId.value;
  if (!id) return;
  const next = new Set(nsfwRevealedIds.value);
  next.add(id);
  nsfwRevealedIds.value = next;
  ageGatePostId.value = null;
}

function cancelAgeGate() {
  ageGatePostId.value = null;
}

function openAgeGate(postId: string) {
  ageGatePostId.value = postId;
}

function mediaBlockedByNsfw(it: TimelinePost) {
  return Boolean(it.is_nsfw) && !nsfwRevealed(it.id);
}

function isPostVisibleForPoll(it: TimelinePost): boolean {
  if (!it.visible_at) return true;
  return new Date(it.visible_at).getTime() <= Date.now();
}

function formatPollEnds(iso: string): string {
  return formatAbsoluteDateTime(iso) || iso;
}

function pollPercent(poll: TimelinePoll, votes: number): number {
  const totalVotes = poll.total_votes || 0;
  if (totalVotes <= 0) return 0;
  return Math.round((votes / totalVotes) * 1000) / 10;
}

async function votePoll(it: TimelinePost, optionId: string) {
  if (!it.poll || it.poll.closed || it.poll.my_option_id || !isPostVisibleForPoll(it)) return;
  const token = getAccessToken();
  if (!token) return;
  pollBusy.value = it.id;
  try {
    const mapped = await voteTimelinePoll(token, it, optionId);
    if (mapped.poll) {
      emit("patchItem", {
        id: it.id,
        patch: {
          poll: mapped.poll,
          reactions: mapped.reactions,
          like_count: mapped.like_count,
          liked_by_me: mapped.liked_by_me,
        },
      });
    }
  } catch (e: unknown) {
    window.alert(e instanceof Error ? e.message : t("components.postTimeline.voteFailed"));
  } finally {
    pollBusy.value = null;
  }
}

async function submitUnlock(it: TimelinePost) {
  const token = getAccessToken();
  if (!token || unlockBusy.value === it.id) return;
  const pwd = (unlockPwd[it.id] ?? "").trim();
  unlockBusy.value = it.id;
  unlockErr[it.id] = "";
  try {
    let entitlementJwt: string | undefined;
    const provider = (it.membership_provider || "").toLowerCase();
    if (it.has_membership_lock && provider === "patreon") {
      try {
        entitlementJwt = it.is_federated
          ? await requestPatreonEntitlementFederated(token, it.id, it.remote_object_url)
          : await requestPatreonEntitlement(token, it.id);
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : "";
        unlockErr[it.id] =
          msg === "patreon_not_connected"
            ? t("components.postTimeline.unlock.patreonNotConnected")
            : msg === "not_entitled"
              ? t("components.postTimeline.unlock.notEntitled")
              : msg === "patreon_api_error" || msg.startsWith("patreon_")
                ? t("components.postTimeline.unlock.patreonApiError")
                : msg || t("components.postTimeline.unlock.unlockFailed");
        return;
      }
    }
    if (it.has_membership_lock && provider === "gumroad") {
      if (it.is_federated) {
        unlockErr[it.id] = t("components.postTimeline.unlock.federatedMembershipUnsupported");
        return;
      }
      const licenseKey = (unlockGumroadLicense[it.id] ?? "").trim();
      if (!licenseKey) {
        unlockErr[it.id] = t("components.postTimeline.unlock.gumroadLicenseRequired");
        return;
      }
      try {
        entitlementJwt = await requestGumroadEntitlement(token, it.id, licenseKey);
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : "";
        unlockErr[it.id] =
          msg === "not_entitled"
            ? t("components.postTimeline.unlock.notEntitled")
            : msg === "gumroad_license_required"
              ? t("components.postTimeline.unlock.gumroadLicenseRequired")
              : msg === "gumroad_api_error" || msg.startsWith("gumroad_")
                ? t("components.postTimeline.unlock.gumroadApiError")
                : msg || t("components.postTimeline.unlock.unlockFailed");
        return;
      }
    }
    const res = await unlockTimelinePost(token, it, { password: pwd, entitlement_jwt: entitlementJwt });
    emit("patchItem", {
      id: it.id,
      patch: {
        caption: res.caption,
        media_type: res.media_type,
        media_urls: res.media_urls ?? [],
        is_nsfw: Boolean(res.is_nsfw),
        has_view_password: Boolean(res.has_view_password),
        view_password_scope: typeof res.view_password_scope === "number" ? res.view_password_scope : 0,
        view_password_text_ranges: Array.isArray(res.view_password_text_ranges) ? res.view_password_text_ranges : [],
        content_locked: Boolean(res.content_locked),
        text_locked: Boolean(res.text_locked),
        media_locked: Boolean(res.media_locked),
      },
    });
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : t("components.postTimeline.unlock.unlockFailed");
    unlockErr[it.id] =
      msg === "wrong_password"
        ? t("components.postTimeline.unlock.wrongPassword")
        : msg === "no_password"
          ? t("components.postTimeline.unlock.noPassword")
          : msg === "invalid_unlock"
            ? t("components.postTimeline.unlock.passwordRequired")
            : msg === "federation_patreon_entitlement_unsupported" || msg === "federation_membership_entitlement_unsupported"
              ? t("components.postTimeline.unlock.federatedMembershipUnsupported")
              : msg === "untrusted_instance"
                ? t("components.postTimeline.unlock.untrustedInstance")
                : msg;
  } finally {
    unlockBusy.value = null;
  }
}
</script>

<template>
  <div>
    <Teleport to="body">
      <div
        v-if="ageGatePostId"
        class="fixed inset-0 z-[120] flex items-center justify-center p-4"
        role="dialog"
        aria-modal="true"
        aria-labelledby="age-gate-title"
      >
        <div class="absolute inset-0 bg-black/50" aria-hidden="true" @click="cancelAgeGate" />
        <div class="relative z-10 w-full max-w-md rounded-2xl border border-neutral-200 bg-white p-6 shadow-xl">
          <h2 id="age-gate-title" class="text-lg font-bold text-neutral-900">{{ $t("components.postTimeline.ageGate.title") }}</h2>
          <p class="mt-2 text-sm leading-relaxed text-neutral-600">
            {{ $t("components.postTimeline.ageGate.body") }}
          </p>
          <div class="mt-6 flex flex-wrap justify-end gap-2">
            <button
              type="button"
              class="rounded-full border border-neutral-200 px-4 py-2 text-sm font-medium text-neutral-700 hover:bg-neutral-50"
              @click="cancelAgeGate"
            >
              {{ $t("views.compose.cancel") }}
            </button>
            <button
              type="button"
              class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700"
              @click="confirmAgeGate"
            >
              {{ $t("components.postTimeline.ageGate.confirm") }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <div
      v-for="it in items"
      :id="`post-${feedRowKey(it)}`"
      :key="feedRowKey(it)"
      class="border-b border-neutral-200"
    >
      <div
        v-if="it.repost && !hasCommentedRepost(it)"
        class="flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-neutral-200 bg-neutral-50/90 px-4 py-2 text-xs text-neutral-600"
      >
        <Icon name="repost" class="h-4 w-4 shrink-0 text-lime-600" />
        <RouterLink
          v-if="it.is_federated && federatedRemoteProfilePath(it)"
          :to="federatedRemoteProfilePath(it)!"
          class="font-medium text-neutral-800 hover:text-lime-700 hover:underline"
          @click.stop
        >
          {{ it.repost.user_display_name?.trim() || timelineDisplayName(it) }}
        </RouterLink>
        <RouterLink
          v-if="!(it.is_federated && federatedRemoteProfilePath(it)) && it.repost.user_handle"
          :to="`/@${it.repost.user_handle}`"
          class="font-medium text-neutral-800 hover:text-lime-700 hover:underline"
          @click.stop
        >
          {{ it.repost.user_display_name?.trim() || fullHandleAt(it.repost.user_handle) }}
        </RouterLink>
        <UserBadges :badges="it.repost.user_badges" size="xs" />
        <span>{{ $t("components.postTimeline.repostedBySuffix") }}</span>
        <time class="text-neutral-500" :datetime="it.repost.reposted_at">{{ formatRepostedAt(it.repost.reposted_at) }}</time>
      </div>
      <div :class="it.repost && !hasCommentedRepost(it) ? 'px-3 pb-2 pt-1' : ''">
        <article
          class="flex cursor-default gap-3 px-4 py-3 transition-colors hover:bg-neutral-50/90"
          :class="[threadArticleIndentClass(it), it.repost && !hasCommentedRepost(it) ? 'rounded-xl border border-neutral-200 bg-white shadow-sm' : '']"
          :style="threadArticleIndentStyle(it)"
        >
      <RouterLink
        v-if="rowActorProfileRoute(it)"
        :to="rowActorProfileRoute(it)!"
        class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-xs font-bold text-neutral-700 hover:ring-2 hover:ring-lime-300"
        :aria-label="$t('components.postTimeline.profileAria', { name: rowActorDisplayName(it) })"
      >
        <img
          v-if="rowActorAvatarUrl(it) && !avatarLoadFailed[feedRowKey(it)]"
          :src="rowActorAvatarUrl(it)"
          alt=""
          class="h-full w-full object-cover"
          @error="onAvatarLoadError(feedRowKey(it))"
        />
        <span v-else>{{ avatarInitials(rowActorEmail(it)) }}</span>
      </RouterLink>
      <div
        v-else
        class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-xs font-bold text-neutral-700"
        :aria-label="`${rowActorDisplayName(it)}`"
      >
        <img
          v-if="rowActorAvatarUrl(it) && !avatarLoadFailed[feedRowKey(it)]"
          :src="rowActorAvatarUrl(it)"
          alt=""
          class="h-full w-full object-cover"
          @error="onAvatarLoadError(feedRowKey(it))"
        />
        <span v-else>{{ avatarInitials(rowActorEmail(it)) }}</span>
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5 leading-tight">
              <span class="flex flex-wrap items-center gap-1.5">
                <RouterLink
                  v-if="rowActorProfileRoute(it)"
                  :to="rowActorProfileRoute(it)!"
                  class="truncate font-bold text-neutral-900 hover:text-lime-700 hover:underline"
                >
                  {{ rowActorDisplayName(it) }}
                </RouterLink>
                <span v-else class="truncate font-bold text-neutral-900">
                  {{ rowActorDisplayName(it) }}
                </span>
                <UserBadges :badges="rowActorBadges(it)" size="xs" />
              </span>
              <RouterLink
                v-if="rowActorProfileRoute(it)"
                :to="rowActorProfileRoute(it)!"
                class="truncate text-sm text-neutral-500 hover:text-lime-700"
              >
                {{ rowActorHandleLabel(it) }}
              </RouterLink>
              <span v-else class="truncate text-sm text-neutral-500">{{ rowActorHandleLabel(it) }}</span>
              <span class="text-sm text-neutral-400">·</span>
              <time
                class="shrink-0 text-sm text-neutral-500"
                :datetime="rowPublishedAtISO(it)"
                :title="
                  rowPublishedAtISO(it)
                    ? formatAbsoluteDateTime(rowPublishedAtISO(it))
                    : undefined
                "
              >
                {{ rowPublishedAtLabel(it) }}
              </time>
              <span
                v-if="hasCommentedRepost(it)"
                class="shrink-0 rounded bg-lime-100 px-1.5 py-0.5 text-[10px] font-semibold text-lime-900"
              >
                {{ $t("components.postTimeline.repostBadge") }}
              </span>
              <span
                v-if="!hasCommentedRepost(it) && it.is_federated && it.federated_boost"
                class="shrink-0 rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold text-amber-900"
              >
                {{ $t("components.postTimeline.boostBadge") }}
              </span>
              <span
                v-if="!it.is_federated"
                class="shrink-0 rounded bg-neutral-100 px-1.5 py-0.5 text-[10px] font-semibold text-neutral-700"
              >
                {{ visibilityLabel(it) }}
              </span>
              <template v-if="!hidePostDetailLink">
                <span class="text-sm text-neutral-400">·</span>
                <RouterLink
                  :to="postDetailPath(it.id)"
                  class="shrink-0 text-sm font-medium text-lime-700 hover:text-lime-800 hover:underline"
                  @click.stop
                >
                  {{ $t("components.postTimeline.postDetailLink") }}
                </RouterLink>
                <template v-if="showRemoteObjectLink && it.is_federated && it.remote_object_url">
                  <span class="text-sm text-neutral-400">·</span>
                  <a
                    :href="it.remote_object_url"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="shrink-0 text-sm font-medium text-violet-800 hover:text-violet-900 hover:underline"
                    @click.stop
                  >
                    {{ $t("components.postTimeline.originalPostLink") }}
                  </a>
                </template>
              </template>
            </div>
          </div>
          <div v-if="canShowPostMenu(it)" class="relative shrink-0">
            <button
              type="button"
              class="rounded-full p-1.5 text-neutral-500 hover:bg-neutral-200 hover:text-neutral-800"
              aria-haspopup="true"
              :aria-expanded="openMenuId === it.id"
              @click.stop="togglePostMenu(it.id)"
            >
              <span class="sr-only">{{ $t("components.postTimeline.postMenuSr") }}</span>
              <Icon name="ellipsis" filled class="h-5 w-5" />
            </button>
            <div
              v-if="openMenuId === it.id"
              class="absolute right-0 top-full z-[50] mt-1 min-w-[9rem] overflow-hidden rounded-xl border border-neutral-200 bg-white py-1 shadow-lg"
              role="menu"
              @click.stop
            >
              <button
                v-if="isOwnPost(it)"
                type="button"
                role="menuitem"
                class="block w-full px-4 py-2.5 text-left text-sm text-neutral-800 hover:bg-neutral-50"
                @click.stop="openEdit(it)"
              >
                {{ $t("components.postTimeline.editPost") }}
              </button>
              <button
                v-if="canReportPost(it)"
                type="button"
                role="menuitem"
                class="block w-full px-4 py-2.5 text-left text-sm text-neutral-800 hover:bg-neutral-50"
                @click.stop="requestReportPost(it)"
              >
                {{ $t("components.postTimeline.reportPost") }}
              </button>
              <button
                v-if="canFederationPrivacyMenu(it)"
                type="button"
                role="menuitem"
                class="block w-full px-4 py-2.5 text-left text-sm text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
                :disabled="federationPrivacyBusyId === it.id"
                @click.stop="requestFederationMute(it)"
              >
                {{ $t("components.postTimeline.federationPrivacy.mute") }}
              </button>
              <button
                v-if="canFederationPrivacyMenu(it)"
                type="button"
                role="menuitem"
                class="block w-full px-4 py-2.5 text-left text-sm text-red-700 hover:bg-red-50 disabled:opacity-50"
                :disabled="federationPrivacyBusyId === it.id"
                @click.stop="requestFederationBlock(it)"
              >
                {{ $t("components.postTimeline.federationPrivacy.block") }}
              </button>
              <button
                v-if="isOwnPost(it) || canAdminDeletePost(it)"
                type="button"
                role="menuitem"
                class="block w-full px-4 py-2.5 text-left text-sm text-red-600 hover:bg-red-50"
                @click.stop="requestDeletePost(it)"
              >
                {{ isOwnPost(it) ? $t("components.postTimeline.deletePost") : $t("components.postTimeline.deletePostAsAdmin") }}
              </button>
              <button
                v-if="canAdminSuspendAuthor(it)"
                type="button"
                role="menuitem"
                class="block w-full px-4 py-2.5 text-left text-sm text-red-600 hover:bg-red-50"
                @click.stop="requestSuspendAuthor(it)"
              >
                {{ $t("components.postTimeline.suspendAccount") }}
              </button>
            </div>
          </div>
        </div>

        <p v-if="showReplyParentLink && (it.reply_to_post_id || it.reply_to_object_url)" class="mt-2 text-xs text-neutral-500">
          <RouterLink
            v-if="it.reply_to_post_id"
            :to="postDetailPath(it.reply_to_post_id)"
            class="font-medium text-lime-700 hover:text-lime-800 hover:underline"
            @click.stop
          >
            {{ $t("components.postTimeline.viewReplyParent") }}
          </RouterLink>
          <a
            v-else-if="it.reply_to_object_url"
            :href="it.reply_to_object_url"
            target="_blank"
            rel="noopener noreferrer"
            class="font-medium text-lime-700 hover:text-lime-800 hover:underline"
            @click.stop
          >
            {{ $t("components.postTimeline.viewReplyParent") }}
          </a>
        </p>

        <div
          v-if="it.content_locked && it.has_view_password"
          class="mt-3 rounded-2xl border border-amber-200 bg-amber-50/90 p-4"
        >
          <p class="text-sm font-medium text-amber-950">{{ $t("components.postTimeline.passwordLockedTitle") }}</p>
          <p class="mt-1 text-xs text-amber-900/80">
            {{ $t("components.postTimeline.passwordLockedBody", { summary: lockedSummary(it) }) }}
          </p>
          <div class="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
            <label class="sr-only" :for="`unlock-${it.id}`">{{ $t("components.postTimeline.unlockPasswordSr") }}</label>
            <input
              :id="`unlock-${it.id}`"
              type="password"
              autocomplete="off"
              class="min-w-0 flex-1 rounded-xl border border-amber-300/80 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 focus:ring-2"
              :placeholder="$t('components.postTimeline.unlockPlaceholder')"
              :value="unlockPwd[it.id] ?? ''"
              @input="unlockPwd[it.id] = ($event.target as HTMLInputElement).value"
              @keydown.enter.prevent="submitUnlock(it)"
            />
            <button
              type="button"
              class="shrink-0 rounded-full bg-amber-600 px-4 py-2 text-sm font-semibold text-white hover:bg-amber-700 disabled:opacity-50"
              :disabled="unlockBusy === it.id"
              @click="submitUnlock(it)"
            >
              {{ unlockBusy === it.id ? $t("components.postTimeline.unlockSubmitting") : $t("components.postTimeline.unlockSubmit") }}
            </button>
          </div>
          <p v-if="unlockErr[it.id]" class="mt-2 text-xs text-red-600">{{ unlockErr[it.id] }}</p>
        </div>

        <div
          v-else-if="it.content_locked && it.has_membership_lock"
          class="mt-3 rounded-2xl border border-sky-200 bg-sky-50/90 p-4"
        >
          <p class="text-sm font-medium text-sky-950">{{ $t("components.postTimeline.membershipLockedTitle") }}</p>
          <p class="mt-1 text-xs text-sky-900/80">
            {{ $t("components.postTimeline.membershipLockedBody") }}
          </p>
          <div v-if="(it.membership_provider || '').toLowerCase() === 'gumroad'" class="mt-3">
            <label class="sr-only" :for="`gumroad-license-${it.id}`">{{
              $t("components.postTimeline.unlock.gumroadLicenseLabel")
            }}</label>
            <input
              :id="`gumroad-license-${it.id}`"
              type="text"
              autocomplete="off"
              class="w-full rounded-xl border border-sky-300/80 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-sky-500 focus:ring-2"
              :placeholder="$t('components.postTimeline.unlock.gumroadLicensePlaceholder')"
              :value="unlockGumroadLicense[it.id] ?? ''"
              @input="unlockGumroadLicense[it.id] = ($event.target as HTMLInputElement).value"
              @keydown.enter.prevent="submitUnlock(it)"
            />
          </div>
          <div class="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
            <button
              type="button"
              class="shrink-0 rounded-full bg-sky-600 px-4 py-2 text-sm font-semibold text-white hover:bg-sky-700 disabled:opacity-50"
              :disabled="unlockBusy === it.id"
              @click="submitUnlock(it)"
            >
              {{
                unlockBusy === it.id
                  ? $t("components.postTimeline.unlockSubmitting")
                  : $t("components.postTimeline.membershipUnlockSubmit")
              }}
            </button>
          </div>
          <p v-if="unlockErr[it.id]" class="mt-2 text-xs text-red-600">{{ unlockErr[it.id] }}</p>
        </div>

        <PostRichText v-if="mainCaptionText(it)" :text="mainCaptionText(it)" class="mt-1" />
        <div
          v-if="hasCommentedRepost(it)"
          class="mt-3 overflow-hidden rounded-2xl border border-neutral-200 bg-neutral-50/80"
        >
          <div class="flex gap-3 px-4 py-3">
            <RouterLink
              v-if="originalPostProfileRoute(it)"
              :to="originalPostProfileRoute(it)!"
              class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-[11px] font-bold text-neutral-700 hover:ring-2 hover:ring-lime-300"
              :aria-label="$t('components.postTimeline.profileAria', { name: timelineDisplayName(it) })"
            >
              <img
                v-if="it.user_avatar_url && !avatarLoadFailed[`quoted-${feedRowKey(it)}`]"
                :src="it.user_avatar_url"
                alt=""
                class="h-full w-full object-cover"
                @error="onAvatarLoadError(`quoted-${feedRowKey(it)}`)"
              />
              <span v-else>{{ avatarInitials(it.user_email) }}</span>
            </RouterLink>
            <div
              v-else
              class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-[11px] font-bold text-neutral-700"
              :aria-label="`${timelineDisplayName(it)}`"
            >
              <img
                v-if="it.user_avatar_url && !avatarLoadFailed[`quoted-${feedRowKey(it)}`]"
                :src="it.user_avatar_url"
                alt=""
                class="h-full w-full object-cover"
                @error="onAvatarLoadError(`quoted-${feedRowKey(it)}`)"
              />
              <span v-else>{{ avatarInitials(it.user_email) }}</span>
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5 leading-tight">
                <span class="flex flex-wrap items-center gap-1.5">
                  <RouterLink
                    v-if="originalPostProfileRoute(it)"
                    :to="originalPostProfileRoute(it)!"
                    class="truncate text-sm font-bold text-neutral-900 hover:text-lime-700 hover:underline"
                  >
                    {{ timelineDisplayName(it) }}
                  </RouterLink>
                  <span v-else class="truncate text-sm font-bold text-neutral-900">{{ timelineDisplayName(it) }}</span>
                  <UserBadges :badges="it.user_badges" size="xs" />
                </span>
                <RouterLink
                  v-if="originalPostProfileRoute(it)"
                  :to="originalPostProfileRoute(it)!"
                  class="truncate text-xs text-neutral-500 hover:text-lime-700"
                >
                  {{ handleAt(it) }}
                </RouterLink>
                <span v-else class="truncate text-xs text-neutral-500">{{ handleAt(it) }}</span>
                <span class="text-xs text-neutral-400">·</span>
                <time
                  class="text-xs text-neutral-500"
                  :datetime="postPublishedAtISO(it)"
                  :title="postPublishedAtISO(it) ? formatAbsoluteDateTime(postPublishedAtISO(it)) : undefined"
                >
                  {{ formatPostTime(it) }}
                </time>
              </div>
              <PostRichText v-if="it.caption" :text="it.caption" :card-limit="0" class="mt-1" />
              <div
                v-if="it.media_type === 'image' && it.media_urls?.length"
                class="mt-3 overflow-hidden rounded-2xl border border-neutral-200 bg-neutral-100"
                :class="it.media_urls.length === 1 ? '' : 'grid grid-cols-2 gap-0.5'"
              >
                <template v-for="(cell, gi) in gridSlots(it.media_urls)" :key="`quoted-media-${feedRowKey(it)}-${gi}`">
                  <img
                    v-if="cell"
                    :src="cell"
                    :alt="it.caption || $t('components.postTimeline.imageAltNumbered', { n: mediaIndexFromGridSlot(it.media_urls, gi) + 1 })"
                    class="w-full object-cover"
                    :class="it.media_urls.length === 1 ? 'max-h-64' : 'aspect-square min-h-[92px]'"
                    loading="lazy"
                  />
                  <div v-else class="aspect-square min-h-[92px] bg-neutral-50" aria-hidden="true" />
                </template>
              </div>
              <div
                v-else-if="it.media_type === 'video' && it.media_urls?.[0]"
                class="mt-3 flex items-center gap-2 rounded-2xl border border-neutral-200 bg-neutral-100 px-3 py-3 text-sm text-neutral-700"
              >
                <Icon name="lock" class="h-5 w-5 shrink-0 text-neutral-500" />
                <span>{{ $t("components.postTimeline.quotedVideoPost") }}</span>
              </div>
              <div
                v-else-if="it.media_type === 'audio' && it.media_urls?.[0]"
                class="mt-3 flex items-center gap-2 rounded-2xl border border-neutral-200 bg-neutral-100 px-3 py-3 text-sm text-neutral-700"
              >
                <Icon name="note" class="h-5 w-5 shrink-0 text-neutral-500" />
                <span>{{ $t("components.postTimeline.quotedAudioPost") }}</span>
              </div>
              <div
                v-else-if="it.poll"
                class="mt-3 rounded-2xl border border-neutral-200 bg-white px-3 py-3 text-sm text-neutral-700"
              >
                <p class="text-xs font-medium uppercase tracking-wide text-neutral-500">{{ $t("components.postTimeline.quotedPollPost") }}</p>
                <ul class="mt-2 space-y-1">
                  <li v-for="opt in it.poll.options.slice(0, 3)" :key="`quoted-poll-${it.id}-${opt.id}`" class="truncate">
                    {{ $t("components.postTimeline.quotedPollBullet", { label: opt.label }) }}
                  </li>
                </ul>
              </div>
              <p
                v-if="it.has_view_password && it.media_locked"
                class="mt-3 rounded-2xl border border-dashed border-amber-300 bg-amber-50/70 px-3 py-2 text-sm text-amber-900"
              >
                {{ $t("components.postTimeline.mediaPasswordProtected") }}
              </p>
              <RouterLink
                :to="postDetailPath(it.id)"
                class="mt-3 inline-flex items-center gap-1 text-sm font-medium text-lime-700 hover:text-lime-800 hover:underline"
              >
                {{ $t("components.postTimeline.viewOriginalPost") }}
              </RouterLink>
            </div>
          </div>
        </div>
        <div
          v-if="!hasCommentedRepost(it) && it.poll"
          class="mt-3 rounded-2xl border border-neutral-200 bg-neutral-50/80 px-3 py-3"
        >
          <div class="flex flex-wrap items-center justify-between gap-2 text-xs text-neutral-600">
            <span>{{ $t("components.postTimeline.pollHeading") }}</span>
            <span>{{ $t("components.postTimeline.pollEnds", { date: formatPollEnds(it.poll.ends_at) }) }}</span>
          </div>
          <p v-if="!isPostVisibleForPoll(it)" class="mt-2 text-xs text-amber-800">
            {{ $t("components.postTimeline.pollVoteAfterPublish") }}
          </p>
          <div v-for="opt in it.poll.options" :key="opt.id" class="mt-2">
            <button
              v-if="!it.poll.closed && !it.poll.my_option_id && isPostVisibleForPoll(it)"
              type="button"
              class="flex w-full items-center justify-between gap-2 rounded-xl border border-neutral-200 bg-white px-3 py-2 text-left text-sm text-neutral-900 hover:border-lime-400 hover:bg-lime-50/60 disabled:opacity-50"
              :disabled="pollBusy === it.id"
              @click="votePoll(it, opt.id)"
            >
              <span>{{ opt.label }}</span>
            </button>
            <div v-else class="space-y-1">
              <div class="flex justify-between text-xs text-neutral-700">
                <span :class="it.poll.my_option_id === opt.id ? 'font-semibold text-lime-800' : ''">{{
                  opt.label
                }}</span>
                <span class="tabular-nums">{{
                  $t("components.postTimeline.pollPercentVotes", { pct: pollPercent(it.poll, opt.votes), votes: opt.votes })
                }}</span>
              </div>
              <div class="h-2 overflow-hidden rounded-full bg-neutral-200">
                <div
                  class="h-2 rounded-full bg-lime-500 transition-[width] duration-300"
                  :style="{ width: `${pollPercent(it.poll, opt.votes)}%` }"
                />
              </div>
            </div>
          </div>
          <p v-if="it.poll.closed" class="mt-2 text-xs text-neutral-500">
            {{ $t("components.postTimeline.pollClosed", { total: it.poll.total_votes }) }}
          </p>
        </div>
        <div
          v-if="!hasCommentedRepost(it) && it.has_view_password && it.media_locked"
          class="mt-3 rounded-2xl border border-dashed border-amber-300 bg-amber-50/70 px-4 py-3 text-sm text-amber-900"
        >
          {{ $t("components.postTimeline.mediaPasswordProtected") }}
        </div>
        <div v-if="!hasCommentedRepost(it) && it.media_type === 'image' && it.media_urls?.length" class="mt-3">
            <div
              v-if="mediaBlockedByNsfw(it)"
              class="relative overflow-hidden rounded-2xl border border-neutral-800/40 bg-neutral-900"
            >
              <div
                class="grid gap-0.5"
                :class="it.media_urls.length === 1 ? 'grid-cols-1' : 'grid-cols-2'"
              >
                <div
                  v-for="(url, si) in it.media_urls"
                  :key="`nsfw-slot-${si}`"
                  class="relative min-h-[120px] overflow-hidden bg-neutral-800 sm:min-h-[160px]"
                  :class="it.media_urls.length === 1 ? 'max-h-[420px] sm:max-h-[60vh]' : 'aspect-square min-h-[100px] sm:min-h-[140px]'"
                  aria-hidden="true"
                >
                  <img
                    :src="url"
                    alt=""
                    class="h-full w-full scale-110 object-cover blur-2xl brightness-75"
                    loading="lazy"
                  />
                </div>
              </div>
              <button
                type="button"
                class="absolute inset-0 z-[1] flex flex-col items-center justify-center gap-2 bg-black/55 p-4 text-center text-white outline-none ring-offset-2 ring-offset-neutral-900 focus-visible:ring-2 focus-visible:ring-lime-400"
                @click="openAgeGate(it.id)"
              >
                <span class="text-xs font-semibold uppercase tracking-wide text-white/80">{{ $t("components.postTimeline.nsfwGridBadge") }}</span>
                <span class="text-sm font-medium">{{ $t("components.postTimeline.nsfwGridHint", { count: it.media_urls.length }) }}</span>
              </button>
            </div>
            <div
              v-else
              class="overflow-hidden rounded-2xl border border-neutral-200 bg-neutral-100"
              :class="it.media_urls.length === 1 ? '' : 'grid grid-cols-2 gap-0.5'"
            >
              <template v-for="(cell, gi) in gridSlots(it.media_urls)" :key="gi">
                <button
                  v-if="cell"
                  type="button"
                  :class="[
                    it.media_urls.length === 1
                      ? 'max-h-[510px] w-full overflow-hidden sm:max-h-[70vh]'
                      : 'relative aspect-square min-h-[100px] w-full overflow-hidden sm:min-h-[140px]',
                    'block cursor-zoom-in border-0 bg-transparent p-0 text-left outline-none ring-offset-2 ring-offset-white focus-visible:ring-2 focus-visible:ring-lime-500',
                  ]"
                  :aria-label="
                    $t('components.postTimeline.imageOpenLightboxAria', {
                      index: mediaIndexFromGridSlot(it.media_urls, gi) + 1,
                      total: it.media_urls.length,
                    })
                  "
                  @click="emit('openLightbox', it.media_urls, mediaIndexFromGridSlot(it.media_urls, gi))"
                >
                  <img
                    :src="cell"
                    :alt="it.caption || $t('components.postTimeline.imageAltNumbered', { n: mediaIndexFromGridSlot(it.media_urls, gi) + 1 })"
                    :class="
                      it.media_urls.length === 1
                        ? 'pointer-events-none max-h-[510px] w-full object-contain sm:max-h-[70vh]'
                        : 'pointer-events-none h-full w-full object-cover'
                    "
                    loading="lazy"
                  />
                </button>
                <div
                  v-else
                  class="aspect-square min-h-[100px] bg-neutral-50 sm:min-h-[140px]"
                  aria-hidden="true"
                />
              </template>
            </div>
        </div>
        <div
          v-else-if="!hasCommentedRepost(it) && it.media_type === 'video' && it.media_urls?.[0]"
          class="mt-3"
        >
          <button
            v-if="mediaBlockedByNsfw(it)"
            type="button"
            class="flex max-h-[510px] min-h-[200px] w-full flex-col items-center justify-center gap-2 overflow-hidden rounded-2xl border border-neutral-200 bg-neutral-900 p-6 text-center text-white outline-none ring-offset-2 ring-offset-white focus-visible:ring-2 focus-visible:ring-lime-500 sm:max-h-[70vh]"
            @click="openAgeGate(it.id)"
          >
            <span class="text-xs font-semibold uppercase tracking-wide text-white/70">{{ $t("components.postTimeline.nsfwVideoBadge") }}</span>
            <span class="text-sm font-medium">{{ $t("components.postTimeline.nsfwVideoHint") }}</span>
          </button>
          <GlipzVideoPlayer v-else :src="it.media_urls[0]" />
        </div>
        <div
          v-else-if="!hasCommentedRepost(it) && it.media_type === 'audio' && it.media_urls?.[0]"
          class="mt-3"
        >
          <button
            v-if="mediaBlockedByNsfw(it)"
            type="button"
            class="flex min-h-[120px] w-full flex-col items-center justify-center gap-2 rounded-2xl border border-neutral-200 bg-neutral-900 px-6 py-8 text-center text-white outline-none ring-offset-2 ring-offset-white focus-visible:ring-2 focus-visible:ring-lime-500 dark:ring-offset-neutral-900"
            @click="openAgeGate(it.id)"
          >
            <span class="text-xs font-semibold uppercase tracking-wide text-white/70">{{ $t("components.postTimeline.nsfwAudioBadge") }}</span>
            <span class="text-sm font-medium">{{ $t("components.postTimeline.nsfwAudioHint") }}</span>
          </button>
          <GlipzAudioPlayer v-else :src="it.media_urls[0]" />
        </div>

        <div v-if="hasVisibleReactions(it)" class="mt-3 flex flex-wrap items-center gap-2">
          <button
            v-for="reaction in it.reactions"
            :key="`${it.id}-${reaction.emoji}`"
            type="button"
            class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-sm transition"
            :class="reaction.reacted_by_me
              ? 'border-lime-300 bg-lime-50 text-lime-900'
              : 'border-neutral-200 bg-white text-neutral-700 hover:border-lime-200 hover:bg-lime-50/60'"
            :disabled="actionBusy === `rx-${it.id}`"
            :aria-label="$t('components.postTimeline.ariaReactionToggle', { emoji: reaction.emoji, count: reaction.count })"
            @click="selectReaction(it, reaction.emoji)"
          >
            <EmojiInline :token="reaction.emoji" size-class="text-xl" image-class="h-6 w-6" custom-image-class="h-6 w-auto max-w-14" />
            <span class="text-sm font-medium tabular-nums">{{ formatActionCount(reaction.count) }}</span>
          </button>
        </div>

        <div
          class="mt-3 flex w-full max-w-full flex-wrap items-center justify-between gap-x-1 gap-y-1 text-neutral-400 sm:gap-x-2"
        >
          <button
            v-if="!it.is_federated || showFederatedReplyAction"
            type="button"
            class="group inline-flex min-w-0 items-center gap-1 rounded-full py-1.5 pl-1.5 pr-1 hover:bg-lime-50 hover:text-lime-600 sm:pr-2"
            :aria-label="$t('components.postTimeline.ariaReply', { count: it.reply_count })"
            @click="emit('reply', it)"
          >
            <Icon name="reply" class="h-[18px] w-[18px] shrink-0" />
            <span v-if="it.reply_count > 0" class="min-w-[1ch] text-xs tabular-nums text-neutral-500 group-hover:text-lime-700">
              {{ formatActionCount(it.reply_count) }}
            </span>
          </button>
          <button
            v-if="!it.is_federated || showFederatedRepostAction"
            type="button"
            class="group inline-flex min-w-0 items-center gap-1 rounded-full py-1.5 pl-1.5 pr-1 hover:bg-lime-50 sm:pr-2"
            :class="[
              it.reposted_by_me ? 'text-lime-600' : 'hover:text-lime-600',
              !canRepost(it) && 'cursor-not-allowed opacity-40 hover:bg-transparent hover:text-neutral-400',
            ]"
            :aria-label="$t('components.postTimeline.ariaRepost', { count: it.repost_count })"
            :disabled="actionBusy === `rp-${it.id}` || !canRepost(it)"
            @click="emit('toggleRepost', it)"
          >
            <Icon name="repost" class="h-[18px] w-[18px] shrink-0" />
            <span v-if="it.repost_count > 0" class="min-w-[1ch] text-xs tabular-nums text-neutral-500 group-hover:text-lime-700">
              {{ formatActionCount(it.repost_count) }}
            </span>
          </button>
          <div class="relative">
            <button
              type="button"
              class="group inline-flex min-w-0 items-center gap-1 rounded-full py-1.5 pl-1.5 pr-1 hover:bg-neutral-100 sm:pr-2"
              :class="openReactionPickerId === it.id ? 'bg-neutral-100 text-neutral-800' : 'text-neutral-400 hover:text-neutral-700'"
              :aria-label="$t('components.postTimeline.ariaReactionPicker')"
              :disabled="actionBusy === `rx-${it.id}`"
              @click.stop="toggleReactionPicker(it.id)"
            >
              <span class="text-[18px] leading-none grayscale">{{ "🙂" }}</span>
            </button>
            <Transition
              enter-active-class="transition duration-200 ease-out"
              enter-from-class="translate-y-6 opacity-0 sm:translate-y-2"
              enter-to-class="translate-y-0 opacity-100"
              leave-active-class="transition duration-150 ease-in"
              leave-from-class="translate-y-0 opacity-100"
              leave-to-class="translate-y-6 opacity-0 sm:translate-y-2"
            >
              <div
                v-if="openReactionPickerId === it.id"
                class="fixed inset-x-0 bottom-0 z-[60] overflow-hidden rounded-t-3xl border-t border-neutral-200 bg-white px-3 pb-[calc(0.75rem+env(safe-area-inset-bottom,0px))] pt-2 shadow-2xl ring-1 ring-black/5 sm:absolute sm:bottom-full sm:left-0 sm:mb-2 sm:w-[20rem] sm:max-w-[min(24rem,calc(100vw-2rem))] sm:rounded-2xl sm:border sm:p-2 sm:shadow-lg"
                @click.stop
              >
                <div class="mb-2 flex justify-center sm:hidden" aria-hidden="true">
                  <span class="h-1.5 w-10 rounded-full bg-neutral-300"></span>
                </div>
                <div class="max-h-[min(32rem,calc(100vh-5rem-env(safe-area-inset-top,0px)))] space-y-3 overflow-y-auto pr-1 sm:max-h-[28rem]">
                  <div>
                    <p class="px-1 text-[11px] font-semibold uppercase tracking-wide text-neutral-500">
                      {{ $t("components.postTimeline.reactionPickerStandard") }}
                    </p>
                    <div class="mt-2 space-y-3">
                      <section
                        v-for="group in standardReactionCategories"
                        :key="group.slug"
                        class="space-y-1"
                      >
                        <p class="px-1 text-[11px] font-medium text-neutral-500">
                          {{ reactionCategoryLabel(group.slug) }}
                        </p>
                        <div class="flex flex-wrap gap-1">
                          <button
                            v-for="emoji in group.emojis"
                            :key="`${it.id}-${group.slug}-${emoji.slug}`"
                            type="button"
                            class="inline-flex h-10 w-10 items-center justify-center rounded-full text-lg transition hover:bg-lime-50"
                            :disabled="actionBusy === `rx-${it.id}`"
                            :aria-label="$t('components.postTimeline.ariaReactionSelect', { emoji: emoji.emoji })"
                            :title="emoji.name"
                            @click="selectReaction(it, emoji.emoji)"
                          >
                            <EmojiInline :token="emoji.emoji" size-class="text-xl" image-class="h-7 w-7" custom-image-class="h-7 w-auto max-w-14" />
                          </button>
                        </div>
                      </section>
                    </div>
                  </div>
                  <div v-if="!it.is_federated && (customReactionOptions.length || Object.keys(emojiCatalog).length)">
                    <p class="px-1 text-[11px] font-semibold uppercase tracking-wide text-neutral-500">
                      {{ $t("components.postTimeline.reactionPickerCustom") }}
                    </p>
                    <div v-if="customReactionOptions.length" class="mt-1 flex flex-wrap gap-1">
                      <button
                        v-for="emoji in customReactionOptions"
                        :key="`${it.id}-${emoji.id}`"
                        type="button"
                        class="inline-flex h-10 w-10 items-center justify-center rounded-full transition hover:bg-lime-50"
                        :disabled="actionBusy === `rx-${it.id}`"
                        :aria-label="$t('components.postTimeline.ariaReactionSelect', { emoji: emoji.shortcode })"
                        :title="emoji.shortcode"
                        @click="selectReaction(it, emoji.shortcode)"
                      >
                        <EmojiInline :token="emoji.shortcode" size-class="text-lg" image-class="h-6 w-6" custom-image-class="h-6 w-auto max-w-14" />
                      </button>
                    </div>
                    <p v-else class="mt-1 px-1 text-xs text-neutral-500">
                      {{ $t("components.postTimeline.reactionPickerCustomEmpty") }}
                    </p>
                  </div>
                </div>
              </div>
            </Transition>
          </div>
          <button
            type="button"
            class="group inline-flex items-center gap-1 rounded-full py-1.5 pl-1.5 pr-1 hover:bg-amber-50 sm:pr-2"
            :class="it.bookmarked_by_me ? 'text-amber-500' : 'hover:text-amber-500'"
            :aria-label="it.bookmarked_by_me ? $t('components.postTimeline.ariaBookmarkRemove') : $t('components.postTimeline.ariaBookmarkAdd')"
            :disabled="actionBusy === `bm-${it.id}`"
            @click="emit('toggleBookmark', it)"
          >
            <Icon name="bookmark" :filled="it.bookmarked_by_me" class="h-[18px] w-[18px] shrink-0" />
          </button>
          <button
            type="button"
            class="group inline-flex items-center gap-1 rounded-full p-1.5 hover:bg-lime-50 hover:text-lime-600"
            :aria-label="$t('components.postTimeline.ariaShare')"
            @click="emit('share', it)"
          >
            <Icon name="share" class="h-[18px] w-[18px]" />
          </button>
        </div>
      </div>
    </article>
      </div>
      <div
        v-if="embedThreadReplies && threadBlocks[it.id]"
        class="border-t border-neutral-200 bg-neutral-100/85"
      >
        <button
          type="button"
          class="flex w-full items-center justify-between gap-2 px-3 py-2.5 text-left text-sm font-medium text-neutral-700 hover:bg-neutral-200/50"
          :aria-expanded="isThreadAccordionOpen(it.id)"
          :aria-controls="`thread-panel-${it.id}`"
          :id="`thread-toggle-${it.id}`"
          @click.stop="toggleThreadAccordion(it.id)"
        >
          <span>
            {{
              isThreadAccordionOpen(it.id)
                ? $t("components.postTimeline.threadHideReplies")
                : $t("components.postTimeline.threadShowReplies", { count: threadBlocks[it.id].flatOrdered.length })
            }}
          </span>
          <Icon
            name="chevronDown"
            class="h-4 w-4 shrink-0 text-neutral-500 transition-transform duration-200"
            :class="isThreadAccordionOpen(it.id) ? 'rotate-180' : ''"
          />
        </button>
        <div
          v-show="isThreadAccordionOpen(it.id)"
          :id="`thread-panel-${it.id}`"
          class="border-t border-neutral-200 px-1 pb-2 pt-1"
          role="region"
          :aria-labelledby="`thread-toggle-${it.id}`"
        >
          <PostThreadEmbed
            :items="threadBlocks[it.id].flatOrdered"
            :thread-article-indent-by-post-id="threadBlocks[it.id].indents"
            :action-busy="actionBusy"
            :viewer-email="effectiveViewerEmail"
            :viewer-is-admin="effectiveViewerIsAdmin"
            hide-post-detail-link
            @reply="emit('reply', $event)"
            @toggle-reaction="(it, emoji) => emit('toggleReaction', it, emoji)"
            @toggle-bookmark="emit('toggleBookmark', $event)"
            @toggle-repost="emit('toggleRepost', $event)"
            @share="emit('share', $event)"
            @open-lightbox="(...args) => emit('openLightbox', ...args)"
            @patch-item="emit('patchItem', $event)"
            @remove-post="emit('removePost', $event)"
          />
        </div>
      </div>
    </div>
  </div>

  <Teleport to="body">
    <div
      v-if="reportTarget"
      class="fixed inset-0 z-[125] flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="report-post-title"
    >
      <div class="absolute inset-0 bg-black/50" aria-hidden="true" @click="closeReportModal" />
      <div class="relative z-10 w-full max-w-lg rounded-2xl border border-neutral-200 bg-white p-5 shadow-xl">
        <h2 id="report-post-title" class="text-lg font-bold text-neutral-900">{{ $t("components.postTimeline.reportModal.title") }}</h2>
        <p class="mt-2 text-sm leading-relaxed text-neutral-600">
          {{ $t("components.postTimeline.reportModal.description") }}
        </p>
        <label class="mt-4 block text-sm font-medium text-neutral-700" for="report-reason">{{ $t("components.postTimeline.reportModal.reasonLabel") }}</label>
        <textarea
          id="report-reason"
          v-model="reportReason"
          rows="5"
          maxlength="1000"
          class="mt-1 w-full resize-y rounded-xl border border-neutral-200 px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 focus:ring-2"
          :placeholder="$t('components.postTimeline.reportModal.placeholder')"
        />
        <div class="mt-1 flex justify-between gap-3 text-xs text-neutral-500">
          <span>{{ $t("components.postTimeline.reportModal.hint") }}</span>
          <span>{{ reportReason.trim().length }}/1000</span>
        </div>
        <p v-if="reportErr" class="mt-3 text-sm text-red-600">{{ reportErr }}</p>
        <div class="mt-5 flex flex-wrap justify-end gap-2">
          <button
            type="button"
            class="rounded-full border border-neutral-200 px-4 py-2 text-sm font-medium text-neutral-700 hover:bg-neutral-50"
            :disabled="reportBusy"
            @click="closeReportModal"
          >
            {{ $t("components.postTimeline.reportModal.cancel") }}
          </button>
          <button
            type="button"
            class="rounded-full bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700 disabled:opacity-50"
            :disabled="reportBusy"
            @click="submitReport"
          >
            {{ reportBusy ? $t("components.postTimeline.reportModal.submitting") : $t("components.postTimeline.reportModal.submit") }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="editTarget"
      class="fixed inset-0 z-[130] flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="edit-post-title"
    >
      <div class="absolute inset-0 bg-black/50" aria-hidden="true" @click="closeEditModal" />
      <div class="relative z-10 w-full max-w-lg rounded-2xl border border-neutral-200 bg-white p-5 shadow-xl">
        <h2 id="edit-post-title" class="text-lg font-bold text-neutral-900">{{ $t("components.postTimeline.editModal.title") }}</h2>
        <label class="mt-3 block text-sm font-medium text-neutral-600" for="edit-caption">{{ $t("views.compose.caption") }}</label>
        <textarea
          ref="editCaptionEl"
          id="edit-caption"
          v-model="editCaption"
          rows="4"
          class="mt-1 w-full resize-none rounded-xl border border-neutral-200 px-3 py-2 text-[15px] text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        />
        <label class="mt-3 inline-flex cursor-pointer items-center gap-2 text-sm text-neutral-800">
          <input v-model="editIsNsfw" type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-amber-600 focus:ring-amber-500" />
          {{ $t("components.postTimeline.editModal.nsfwLabel") }}
        </label>
        <div class="mt-3">
          <label class="mb-1 block text-xs font-medium text-neutral-600" for="edit-visibility">{{ $t("views.compose.visibilitySelectLabel") }}</label>
          <select
            id="edit-visibility"
            v-model="editVisibility"
            class="w-full rounded-xl border border-neutral-200 px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 focus:ring-2"
          >
            <option value="public">{{ $t("views.compose.visibility.public.label") }}</option>
            <option value="logged_in">{{ $t("views.compose.visibility.loggedIn.label") }}</option>
            <option value="followers">{{ $t("views.compose.visibility.followers.label") }}</option>
            <option value="private">{{ $t("views.compose.visibility.private.label") }}</option>
          </select>
        </div>
        <div class="mt-4 border-t border-neutral-200 pt-3">
          <p class="text-xs font-medium text-neutral-600">{{ $t("components.postTimeline.editModal.passwordSection") }}</p>
          <label class="mt-2 inline-flex cursor-pointer items-center gap-2 text-sm text-neutral-700">
            <input v-model="editClearPassword" type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-lime-600" />
            {{ $t("components.postTimeline.editModal.clearPassword") }}
          </label>
          <div v-if="!editClearPassword" class="mt-3 space-y-3">
            <div class="grid gap-2 sm:grid-cols-2">
              <div>
                <label class="mb-0.5 block text-xs text-neutral-500" for="edit-pw">{{ $t("components.postTimeline.editModal.newPasswordLabel") }}</label>
                <input
                  id="edit-pw"
                  v-model="editPassword"
                  type="password"
                  autocomplete="new-password"
                  maxlength="72"
                  class="w-full rounded-xl border border-neutral-200 px-3 py-2 text-sm outline-none ring-lime-500 focus:ring-2"
                />
              </div>
              <div>
                <label class="mb-0.5 block text-xs text-neutral-500" for="edit-pw2">{{ $t("components.postTimeline.editModal.confirmLabel") }}</label>
                <input
                  id="edit-pw2"
                  v-model="editPasswordConfirm"
                  type="password"
                  autocomplete="new-password"
                  maxlength="72"
                  class="w-full rounded-xl border border-neutral-200 px-3 py-2 text-sm outline-none ring-lime-500 focus:ring-2"
                />
              </div>
            </div>
            <div class="rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-3">
              <p class="text-xs font-medium text-neutral-600">{{ $t("views.compose.protectTargetsHeading") }}</p>
              <div class="mt-2 flex flex-wrap gap-2">
                <button
                  type="button"
                  class="rounded-full border px-3 py-1.5 text-xs font-medium"
                  :class="editProtectAll ? 'border-lime-500 bg-lime-50 text-lime-800' : 'border-neutral-200 text-neutral-700 hover:bg-white'"
                  @click="toggleEditProtectAll"
                >
                  {{ $t("views.compose.protectAll") }}
                </button>
                <button
                  type="button"
                  class="rounded-full border px-3 py-1.5 text-xs font-medium"
                  :class="editProtectText ? 'border-lime-500 bg-lime-50 text-lime-800' : 'border-neutral-200 text-neutral-700 hover:bg-white'"
                  @click="toggleEditProtectText"
                >
                  {{ $t("views.compose.protectTextPart") }}
                </button>
                <button
                  type="button"
                  class="rounded-full border px-3 py-1.5 text-xs font-medium"
                  :class="editProtectMedia ? 'border-lime-500 bg-lime-50 text-lime-800' : 'border-neutral-200 text-neutral-700 hover:bg-white'"
                  @click="toggleEditProtectMedia"
                >
                  {{ $t("views.compose.protectMediaOnly") }}
                </button>
              </div>
              <div v-if="editProtectText && !editProtectAll" class="mt-3 rounded-xl border border-dashed border-neutral-200 bg-white px-3 py-3">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <p class="text-xs font-medium text-neutral-700">{{ $t("views.compose.protectRangesTitle") }}</p>
                  <button
                    type="button"
                    class="rounded-full border border-neutral-200 px-3 py-1 text-xs font-medium text-neutral-700 hover:bg-neutral-50"
                    @click="addEditSelectedTextRange"
                  >
                    {{ $t("views.compose.addTextRange") }}
                  </button>
                </div>
                <p class="mt-2 text-xs text-neutral-500">{{ $t("views.compose.addTextRangeHintEdit") }}</p>
                <ul v-if="editTextRanges.length" class="mt-3 space-y-2">
                  <li
                    v-for="(rg, idx) in editTextRanges"
                    :key="`${rg.start}-${rg.end}-${idx}`"
                    class="flex items-start justify-between gap-3 rounded-lg border border-neutral-200 bg-neutral-50 px-3 py-2"
                  >
                    <div class="min-w-0">
                      <p class="truncate text-sm text-neutral-800">{{ editTextRangePreview(rg) }}</p>
                      <p class="text-xs text-neutral-500">{{ $t("views.compose.charRange", { start: rg.start, end: rg.end }) }}</p>
                    </div>
                    <button
                      type="button"
                      class="shrink-0 text-xs font-medium text-red-600 hover:text-red-700"
                      @click="removeEditTextRange(idx)"
                    >
                      {{ $t("views.compose.remove") }}
                    </button>
                  </li>
                </ul>
              </div>
            </div>
          </div>
        </div>
        <p v-if="editErr" class="mt-2 text-sm text-red-600">{{ editErr }}</p>
        <div class="mt-5 flex flex-wrap justify-end gap-2">
          <button
            type="button"
            class="rounded-full border border-neutral-200 px-4 py-2 text-sm font-medium text-neutral-700 hover:bg-neutral-50"
            :disabled="editBusy"
            @click="closeEditModal"
          >
            {{ $t("components.postTimeline.editModal.cancel") }}
          </button>
          <button
            type="button"
            class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
            :disabled="editBusy"
            @click="submitEdit"
          >
            {{ editBusy ? $t("components.postTimeline.editModal.saving") : $t("components.postTimeline.editModal.save") }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
