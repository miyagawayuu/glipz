import { computed, nextTick, onBeforeUnmount, ref, watch, type Ref } from "vue";
import { getAccessToken } from "../auth";
import { api, uploadMediaFile } from "../lib/api";
import {
  composerAttachmentLabel,
  inferPostMediaType,
  MAX_COMPOSER_IMAGE_SLOTS,
  mergePickedComposerFiles,
} from "../lib/composerMedia";
import { usePatreonComposer } from "./usePatreonComposer";
import {
  buildViewPasswordScope,
  codeUnitOffsetToRuneIndex,
  normalizeViewPasswordRanges,
  sliceRunes,
} from "../lib/viewPassword";
import type { TimelinePost, ViewPasswordTextRange } from "../types/timeline";

export type PostComposerMode = "normal" | "community";
export type ComposerVisibility = "public" | "logged_in" | "followers" | "private";
export type PostComposerReplyTarget = {
  id: string;
  user_email: string;
  user_handle: string;
  is_federated?: boolean;
  remote_object_url?: string;
};

type Translate = (key: string, params?: Record<string, unknown>) => string;

export function usePostComposerForm(opts: {
  mode: Ref<PostComposerMode>;
  communityId: Ref<string | null | undefined>;
  patreonEnabled: Ref<boolean>;
  t: Translate;
  onSubmitted?: () => void | Promise<void>;
  eventDetail?: () => Record<string, unknown>;
}) {
  const err = ref("");
  const busy = ref(false);
  const caption = ref("");
  const composerCaptionEl = ref<HTMLTextAreaElement | null>(null);
  const selectedFiles = ref<File[]>([]);
  const previewUrls = ref<string[]>([]);
  const replyingTo = ref<PostComposerReplyTarget | null>(null);
  const isNsfw = ref(false);
  const composerVisibility = ref<ComposerVisibility>("public");
  const viewPassword = ref("");
  const viewPasswordConfirm = ref("");
  const composerProtectText = ref(false);
  const composerProtectMedia = ref(false);
  const composerProtectAll = ref(false);
  const composerTextRanges = ref<ViewPasswordTextRange[]>([]);
  const composerNsfwOpen = ref(false);
  const composerPasswordOpen = ref(false);
  const composerPollOpen = ref(false);
  const pollOptionInputs = ref(["", ""]);
  const pollDurationHours = ref(24);
  const scheduleLocal = ref("");
  const composerScheduleOpen = ref(false);
  const composerVisibilityOpen = ref(false);

  const isCommunityMode = computed(() => opts.mode.value === "community");
  const attachmentKind = computed(() =>
    selectedFiles.value.length ? inferPostMediaType(selectedFiles.value) : "none",
  );
  const attachmentPickerDisabled = computed(
    () =>
      busy.value ||
      attachmentKind.value === "video" ||
      attachmentKind.value === "audio" ||
      selectedFiles.value.length >= MAX_COMPOSER_IMAGE_SLOTS,
  );

  const {
    patreonAvailable,
    patreonConnected,
    patreonCampaigns,
    composerMembershipOpen,
    membershipUsePatreon,
    membershipCampaignId,
    membershipTierId,
    patreonConnectBusy,
    membershipTierOptions,
    loadPatreon,
    resetPatreonComposerState,
    validateMembershipForSubmit,
    applyMembershipToBody,
    connectPatreonOAuth,
  } = usePatreonComposer({
    viewPassword,
    viewPasswordConfirm,
    composerPasswordOpen,
    patreonEnabled: opts.patreonEnabled,
  });

  const fanclubComposerEnabled = computed(() => !isCommunityMode.value && patreonAvailable.value);

  watch(
    selectedFiles,
    (files) => {
      previewUrls.value.forEach((u) => URL.revokeObjectURL(u));
      previewUrls.value = files.map((f) => URL.createObjectURL(f));
    },
    { deep: true },
  );

  watch(isCommunityMode, (community) => {
    if (!community) return;
    composerPasswordOpen.value = false;
    composerScheduleOpen.value = false;
    composerVisibilityOpen.value = false;
    viewPassword.value = "";
    viewPasswordConfirm.value = "";
    composerProtectText.value = false;
    composerProtectMedia.value = false;
    composerProtectAll.value = false;
    composerTextRanges.value = [];
    scheduleLocal.value = "";
    composerVisibility.value = "public";
    resetPatreonComposerState();
  });

  onBeforeUnmount(() => {
    previewUrls.value.forEach((u) => URL.revokeObjectURL(u));
    previewUrls.value = [];
  });

  function hasComposerContent(): boolean {
    if (selectedFiles.value.length > 0) return true;
    if (caption.value.trim().length > 0) return true;
    if (composerPollOpen.value) {
      const pollOpts = pollOptionInputs.value.map((s) => s.trim()).filter(Boolean);
      if (pollOpts.length >= 2) return true;
    }
    return false;
  }

  function onFilesSelect(e: Event) {
    const input = e.target as HTMLInputElement;
    const incoming = Array.from(input.files ?? []);
    input.value = "";
    if (!incoming.length) return;

    const { files, replacedKind, partialImageDrop, excludedImages } = mergePickedComposerFiles(
      selectedFiles.value,
      incoming,
    );
    selectedFiles.value = files;
    if (replacedKind) {
      err.value = opts.t("views.compose.errors.attachmentsReplaced");
    } else if (partialImageDrop) {
      err.value = opts.t("views.compose.errors.tooManyImages", { count: excludedImages });
    } else {
      err.value = "";
    }
  }

  function removeImage(i: number) {
    selectedFiles.value = selectedFiles.value.filter((_, j) => j !== i);
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
      err.value = opts.t("views.compose.errors.selectTextRange");
      return;
    }
    const startOffset = el.selectionStart ?? 0;
    const endOffset = el.selectionEnd ?? startOffset;
    if (startOffset === endOffset) {
      err.value = opts.t("views.compose.errors.selectTextRange");
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
    return text || opts.t("views.compose.whitespaceOnly");
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
      el.setSelectionRange(pos, pos);
      el.focus();
    });
  }

  function addPollOptionField() {
    if (pollOptionInputs.value.length >= 4) return;
    pollOptionInputs.value = [...pollOptionInputs.value, ""];
  }

  function startReply(it: TimelinePost | PostComposerReplyTarget) {
    replyingTo.value = {
      id: it.id,
      user_email: it.user_email,
      user_handle: it.user_handle ?? "",
      is_federated: Boolean(it.is_federated),
      remote_object_url: it.remote_object_url,
    };
    void nextTick(() => composerCaptionEl.value?.focus());
  }

  function cancelReply() {
    replyingTo.value = null;
  }

  async function connectPatreon(returnTo: string) {
    err.value = "";
    const r = await connectPatreonOAuth(returnTo);
    if (r.error) {
      err.value = r.error === "patreon_oauth_failed" ? opts.t("views.compose.errors.postFailed") : r.error;
    }
  }

  function resetComposer() {
    caption.value = "";
    selectedFiles.value = [];
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
  }

  async function submitPost() {
    if (!hasComposerContent()) {
      err.value = opts.t("views.compose.errors.composerNeedsContent");
      return;
    }
    const token = getAccessToken();
    if (!token || busy.value) return;

    const pw = isCommunityMode.value ? "" : viewPassword.value.trim();
    const pw2 = isCommunityMode.value ? "" : viewPasswordConfirm.value.trim();
    const pwScope = composerViewPasswordScope();
    if (!isCommunityMode.value) {
      const memErr = validateMembershipForSubmit(pw, pw2, opts.t);
      if (memErr) {
        err.value = memErr;
        return;
      }
      if (pw || pw2) {
        if (pw !== pw2) {
          err.value = opts.t("views.compose.errors.viewPasswordMismatch");
          return;
        }
        if (pw.length < 4 || pw.length > 72) {
          err.value = opts.t("views.compose.errors.viewPasswordLength");
          return;
        }
        if (pwScope === 0) {
          err.value = opts.t("views.compose.errors.viewPasswordScopeRequired");
          return;
        }
        if (composerProtectText.value && !composerProtectAll.value && composerTextRanges.value.length === 0) {
          err.value = opts.t("views.compose.errors.viewPasswordRangesRequired");
          return;
        }
      }
    }

    const pollOpts = pollOptionInputs.value.map((s) => s.trim()).filter(Boolean);
    if (composerPollOpen.value && pollOpts.length < 2) {
      err.value = opts.t("views.compose.errors.pollMinOptions");
      return;
    }

    let visibleIso: string | undefined;
    if (!isCommunityMode.value && scheduleLocal.value.trim()) {
      const d = new Date(scheduleLocal.value);
      if (Number.isNaN(d.getTime())) {
        err.value = opts.t("views.compose.errors.scheduleInvalid");
        return;
      }
      visibleIso = d.toISOString();
    }

    if (composerPollOpen.value && pollOpts.length >= 2) {
      const baseMs = visibleIso ? new Date(visibleIso).getTime() : Date.now();
      const ends = new Date(baseMs + pollDurationHours.value * 3600000);
      if (Number.isNaN(ends.getTime())) {
        err.value = opts.t("views.compose.errors.pollInvalid");
        return;
      }
    }

    busy.value = true;
    err.value = "";
    const capText = caption.value;
    try {
      const objectKeys: string[] = [];
      for (const file of selectedFiles.value) {
        const up = await uploadMediaFile(token, file);
        objectKeys.push(up.object_key);
      }
      const body: Record<string, unknown> = {
        caption: capText,
        media_type: objectKeys.length ? inferPostMediaType(selectedFiles.value) : "none",
        object_keys: objectKeys,
        is_nsfw: isNsfw.value,
        visibility: isCommunityMode.value ? "public" : composerVisibility.value,
      };
      if (isCommunityMode.value) {
        if (!opts.communityId.value) {
          err.value = opts.t("components.sidebarCompose.communityMissing");
          return;
        }
        body.community_id = opts.communityId.value;
      }
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
      if (!isCommunityMode.value && pw) {
        body.view_password = pw;
        body.view_password_scope = pwScope;
        if (composerProtectText.value && !composerProtectAll.value) {
          body.view_password_text_ranges = composerTextRanges.value;
        }
      }
      if (!isCommunityMode.value) {
        applyMembershipToBody(body);
      }
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
      resetComposer();
      await opts.onSubmitted?.();
      window.dispatchEvent(new CustomEvent("glipz:post-created", { detail: opts.eventDetail?.() ?? { mode: opts.mode.value } }));
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : opts.t("views.compose.errors.postFailed");
    } finally {
      busy.value = false;
    }
  }

  return {
    err,
    busy,
    caption,
    composerCaptionEl,
    selectedFiles,
    previewUrls,
    replyingTo,
    isNsfw,
    composerVisibility,
    viewPassword,
    viewPasswordConfirm,
    composerProtectText,
    composerProtectMedia,
    composerProtectAll,
    composerTextRanges,
    composerNsfwOpen,
    composerPasswordOpen,
    composerPollOpen,
    pollOptionInputs,
    pollDurationHours,
    scheduleLocal,
    composerScheduleOpen,
    composerVisibilityOpen,
    isCommunityMode,
    attachmentKind,
    attachmentPickerDisabled,
    patreonAvailable,
    patreonConnected,
    patreonCampaigns,
    composerMembershipOpen,
    membershipUsePatreon,
    membershipCampaignId,
    membershipTierId,
    patreonConnectBusy,
    membershipTierOptions,
    fanclubComposerEnabled,
    loadPatreon,
    onFilesSelect,
    removeImage,
    composerViewPasswordScope,
    toggleComposerProtectAll,
    toggleComposerProtectText,
    toggleComposerProtectMedia,
    addComposerSelectedTextRange,
    removeComposerTextRange,
    composerTextRangePreview,
    insertComposerEmoji,
    addPollOptionField,
    startReply,
    cancelReply,
    connectPatreon,
    resetComposer,
    submitPost,
    composerAttachmentLabel,
  };
}
