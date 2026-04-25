<script setup lang="ts">
import { computed, nextTick, onActivated, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import ComposerEmojiPicker from "../components/ComposerEmojiPicker.vue";
import GlipzAudioPlayer from "../components/GlipzAudioPlayer.vue";
import GlipzVideoPlayer from "../components/GlipzVideoPlayer.vue";
import Icon from "../components/Icon.vue";
import { api, uploadMediaFile } from "../lib/api";
import {
  composerAttachmentLabel,
  inferPostMediaType,
  MAX_COMPOSER_IMAGE_SLOTS,
  mergePickedComposerFiles,
} from "../lib/composerMedia";
import { patreonSettingsPath, usePatreonComposer } from "../composables/usePatreonComposer";
import { usePaymentComposer } from "../composables/usePaymentComposer";
import { listPayPalPlans, type PayPalPlanRow } from "../lib/paymentPayPal";
import { avatarInitials, handleAt } from "../lib/feedDisplay";
import { parseComposerReplyQuery, type ComposerReplyTarget } from "../lib/postComposer";
import {
  buildViewPasswordScope,
  codeUnitOffsetToRuneIndex,
  normalizeViewPasswordRanges,
  sliceRunes,
} from "../lib/viewPassword";
import type { ViewPasswordTextRange } from "../types/timeline";

const MAX_IMAGES = MAX_COMPOSER_IMAGE_SLOTS;

type ComposerVisibility = "public" | "logged_in" | "followers" | "private";

const { t } = useI18n();
const visibilityOptions = computed<Array<{ value: ComposerVisibility; label: string; description: string }>>(() => [
  { value: "public", label: t("views.compose.visibility.public.label"), description: t("views.compose.visibility.public.description") },
  { value: "logged_in", label: t("views.compose.visibility.loggedIn.label"), description: t("views.compose.visibility.loggedIn.description") },
  { value: "followers", label: t("views.compose.visibility.followers.label"), description: t("views.compose.visibility.followers.description") },
  { value: "private", label: t("views.compose.visibility.private.label"), description: t("views.compose.visibility.private.description") },
]);

const router = useRouter();
const route = useRoute();

const err = ref("");
const busy = ref(false);
const myEmail = ref<string | null>(null);
const myHandle = ref<string | null>(null);
const myAvatarUrl = ref<string | null>(null);
const composerAvatarImgFailed = ref(false);
const replyingTo = ref<ComposerReplyTarget | null>(null);
const caption = ref("");
const composerCaptionEl = ref<HTMLTextAreaElement | null>(null);
const selectedImages = ref<File[]>([]);
const previewUrls = ref<string[]>([]);
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

const {
  composerPaymentOpen,
  paymentUsePayPal,
  payPalPlanId,
  resetPaymentComposerState,
  validatePaymentForSubmit,
  applyPaymentToBody,
} = usePaymentComposer({ viewPassword, viewPasswordConfirm, composerPasswordOpen, membershipUsePatreon });

const patreonSettingsHref = patreonSettingsPath;
const paypalPlans = ref<PayPalPlanRow[]>([]);
const paypalPlansLoading = ref(false);
const activePayPalPlans = computed(() => paypalPlans.value.filter((plan) => plan.active !== false));

async function loadPayPalPlans(token: string) {
  paypalPlansLoading.value = true;
  try {
    paypalPlans.value = await listPayPalPlans(token);
  } catch {
    paypalPlans.value = [];
  } finally {
    paypalPlansLoading.value = false;
  }
}

function hasComposerContent(): boolean {
  if (selectedImages.value.length > 0) return true;
  if (caption.value.trim().length > 0) return true;
  if (composerPollOpen.value) {
    const pollOpts = pollOptionInputs.value.map((s) => s.trim()).filter(Boolean);
    if (pollOpts.length >= 2) return true;
  }
  return false;
}

async function connectPatreonFromCompose() {
  err.value = "";
  const r = await connectPatreonOAuth("/compose");
  if (r.error) {
    err.value = r.error === "patreon_oauth_failed" ? t("views.compose.errors.postFailed") : r.error;
  }
}

async function loadMe() {
  const token = getAccessToken();
  if (!token) {
    await router.replace({ path: "/login", query: { next: route.fullPath } });
    return;
  }
  try {
    const u = await api<{ email: string; handle?: string; avatar_url?: string | null }>("/api/v1/me", { method: "GET", token });
    myEmail.value = u.email;
    myHandle.value = typeof u.handle === "string" ? u.handle : null;
    myAvatarUrl.value = u.avatar_url && String(u.avatar_url).trim() !== "" ? String(u.avatar_url) : null;
    composerAvatarImgFailed.value = false;
    void loadPatreon(token);
    void loadPayPalPlans(token);
  } catch {
    myEmail.value = null;
    myHandle.value = null;
    myAvatarUrl.value = null;
    composerAvatarImgFailed.value = false;
  }
}

function syncReplyingToFromRoute() {
  replyingTo.value = parseComposerReplyQuery(route.query);
}

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

function resetComposer() {
  caption.value = "";
  selectedImages.value = [];
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
  resetPaymentComposerState();
}

async function submitPost() {
  if (!hasComposerContent()) {
    err.value = t("views.compose.errors.composerNeedsContent");
    return;
  }
  const token = getAccessToken();
  if (!token) {
    await router.replace({ path: "/login", query: { next: route.fullPath } });
    return;
  }

  const pw = viewPassword.value.trim();
  const pw2 = viewPasswordConfirm.value.trim();
  const pwScope = composerViewPasswordScope();
  const memErr = validateMembershipForSubmit(pw, pw2, t);
  if (memErr) {
    err.value = memErr;
    return;
  }
  const payErr = validatePaymentForSubmit(t);
  if (payErr) {
    err.value = payErr;
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
  try {
    const objectKeys: string[] = [];
    for (const file of selectedImages.value) {
      const up = await uploadMediaFile(token, file);
      objectKeys.push(up.object_key);
    }
    const body: Record<string, unknown> = {
      caption: caption.value,
      media_type: objectKeys.length ? inferPostMediaType(selectedImages.value) : "none",
      object_keys: objectKeys,
      is_nsfw: isNsfw.value,
      visibility: composerVisibility.value,
    };
    if (visibleIso) body.visible_at = visibleIso;
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
    applyPaymentToBody(body);
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
    replyingTo.value = null;
    await router.push("/feed");
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.compose.errors.postFailed");
  } finally {
    busy.value = false;
  }
}

function addPollOptionField() {
  if (pollOptionInputs.value.length >= 4) return;
  pollOptionInputs.value = [...pollOptionInputs.value, ""];
}

async function cancelReply() {
  replyingTo.value = null;
  await router.replace("/compose");
}

watch(
  selectedImages,
  (files) => {
    previewUrls.value.forEach((u) => URL.revokeObjectURL(u));
    previewUrls.value = files.map((f) => URL.createObjectURL(f));
  },
  { deep: true },
);

watch(
  () => route.fullPath,
  () => {
    syncReplyingToFromRoute();
  },
  { immediate: true },
);

onMounted(() => {
  void loadMe();
});

onActivated(() => {
  const tok = getAccessToken();
  if (tok) void loadPatreon(tok);
});

onBeforeUnmount(() => {
  previewUrls.value.forEach((u) => URL.revokeObjectURL(u));
});
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="flex h-12 items-center gap-3">
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="$t('views.compose.back')"
        @click="router.push('/feed')"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <h1 class="text-lg font-bold">{{ $t("views.compose.title") }}</h1>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="flex h-12 items-center gap-3 px-4">
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="$t('views.compose.back')"
        @click="router.push('/feed')"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <h1 class="text-lg font-bold">{{ $t("views.compose.title") }}</h1>
    </div>
  </Teleport>

  <div class="mx-auto w-full max-w-[680px]">
    <div class="flex gap-3 px-4 py-4">
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
          rows="6"
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
              aria-controls="composer-membership-panel"
              @click="composerMembershipOpen = !composerMembershipOpen"
            >
              <span class="sr-only">{{ $t("views.compose.membershipOpen") }}</span>
              <Icon name="user" class="h-5 w-5" />
            </button>
            <button
              type="button"
              class="rounded-full p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800"
              :class="(paymentUsePayPal || composerPaymentOpen) && 'bg-emerald-50 text-emerald-800'"
              :title="composerPaymentOpen ? $t('views.compose.paymentClose') : $t('views.compose.paymentTitle')"
              :aria-expanded="composerPaymentOpen"
              aria-controls="composer-payment-panel"
              @click="composerPaymentOpen = !composerPaymentOpen"
            >
              <span class="sr-only">{{ $t("views.compose.paymentOpen") }}</span>
              <Icon name="wallet" class="h-5 w-5" />
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
        <div v-if="composerPollOpen" class="mt-3 rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-3 text-sm">
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
          class="mt-3 rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-3 text-sm"
        >
          <label class="text-xs font-medium text-neutral-700" for="schedule-at">{{ $t("views.compose.schedulePanelLabel") }}</label>
          <input
            id="schedule-at"
            v-model="scheduleLocal"
            type="datetime-local"
            class="mt-1 w-full max-w-xs rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 focus:ring-2"
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
        <div
          v-if="composerNsfwOpen || composerPasswordOpen || composerMembershipOpen || composerPaymentOpen"
          class="mt-3 space-y-3"
        >
          <div
            v-if="composerNsfwOpen"
            id="composer-nsfw-panel"
            class="rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-3 text-sm"
          >
            <label class="inline-flex cursor-pointer items-center gap-2 text-neutral-800">
              <input v-model="isNsfw" type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-lime-600 focus:ring-lime-500" />
              <span>{{ $t("views.compose.nsfwPostCheckbox") }}</span>
            </label>
            <p class="mt-2 text-xs text-neutral-600">{{ $t("views.compose.nsfwPostHint") }}</p>
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
            id="composer-membership-panel"
            class="rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-3 text-sm"
          >
            <p class="text-xs font-medium text-neutral-700">{{ $t("views.compose.membershipTitle") }}</p>
            <p class="mt-1 text-xs text-neutral-600">{{ $t("views.compose.membershipHint") }}</p>
            <div class="mt-3 flex flex-wrap gap-2">
              <button
                v-if="patreonAvailable"
                type="button"
                class="rounded-full border px-3 py-1.5 text-xs font-medium"
                :class="membershipProvider === 'patreon' ? 'border-lime-500 bg-lime-50 text-lime-800' : 'border-neutral-200 text-neutral-700 hover:bg-neutral-50'"
                @click="membershipProvider = 'patreon'"
              >
                Patreon
              </button>
              <button
                type="button"
                class="rounded-full border px-3 py-1.5 text-xs font-medium"
                :class="membershipProvider === 'gumroad' ? 'border-lime-500 bg-lime-50 text-lime-800' : 'border-neutral-200 text-neutral-700 hover:bg-neutral-50'"
                @click="membershipProvider = 'gumroad'"
              >
                Gumroad
              </button>
            </div>
            <div v-if="membershipProvider === 'patreon' && !patreonConnected" class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center">
              <button
                type="button"
                :disabled="patreonConnectBusy"
                class="inline-flex w-fit rounded-full bg-lime-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
                @click="connectPatreonFromCompose"
              >
                {{
                  patreonConnectBusy
                    ? $t("views.settings.fanclubPatreon.connecting")
                    : $t("views.settings.fanclubPatreon.connect")
                }}
              </button>
              <RouterLink :to="patreonSettingsHref" class="text-xs font-medium text-lime-800 underline">{{
                $t("views.compose.membershipGoSettings")
              }}</RouterLink>
            </div>
            <template v-else-if="membershipProvider === 'patreon'">
              <label class="mt-3 flex cursor-pointer items-center gap-2 text-sm text-neutral-800">
                <input v-model="membershipUsePatreon" type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-lime-600" />
                <span>{{ $t("views.compose.membershipOpen") }}</span>
              </label>
              <div v-if="membershipUsePatreon" class="mt-3 space-y-2">
                <div v-if="patreonCampaigns.length" class="grid gap-2 sm:grid-cols-2">
                  <div>
                    <label class="mb-0.5 block text-xs text-neutral-500" for="mem-camp">{{ $t("views.compose.membershipPickCampaign") }}</label>
                    <select
                      id="mem-camp"
                      v-model="membershipCampaignId"
                      class="w-full rounded-xl border border-neutral-200 bg-white px-2 py-2 text-sm text-neutral-900"
                    >
                      <option value="">{{ "—" }}</option>
                      <option v-for="c in patreonCampaigns" :key="c.id" :value="c.id">{{ c.title || c.id }}</option>
                    </select>
                  </div>
                  <div>
                    <label class="mb-0.5 block text-xs text-neutral-500" for="mem-tier">{{ $t("views.compose.membershipPickTier") }}</label>
                    <select
                      id="mem-tier"
                      v-model="membershipTierId"
                      :disabled="!membershipCampaignId"
                      class="w-full rounded-xl border border-neutral-200 bg-white px-2 py-2 text-sm text-neutral-900 disabled:opacity-50"
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
                    <label class="mb-0.5 block text-xs text-neutral-500" for="mem-cid">{{ $t("views.compose.membershipCampaign") }}</label>
                    <input
                      id="mem-cid"
                      v-model="membershipCampaignId"
                      type="text"
                      autocomplete="off"
                      class="w-full rounded-xl border border-neutral-200 bg-white px-2 py-2 text-sm text-neutral-900"
                    />
                  </div>
                  <div>
                    <label class="mb-0.5 block text-xs text-neutral-500" for="mem-tid">{{ $t("views.compose.membershipTier") }}</label>
                    <input
                      id="mem-tid"
                      v-model="membershipTierId"
                      type="text"
                      autocomplete="off"
                      class="w-full rounded-xl border border-neutral-200 bg-white px-2 py-2 text-sm text-neutral-900"
                    />
                  </div>
                </div>
              </div>
            </template>
            <template v-else>
              <label class="mt-3 flex cursor-pointer items-center gap-2 text-sm text-neutral-800">
                <input v-model="membershipUsePatreon" type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-lime-600" />
                <span>{{ $t("views.compose.gumroadMembershipOpen") }}</span>
              </label>
              <div v-if="membershipUsePatreon" class="mt-3">
                <label class="mb-0.5 block text-xs text-neutral-500" for="gumroad-product-id">{{
                  $t("views.compose.gumroadProductId")
                }}</label>
                <input
                  id="gumroad-product-id"
                  v-model="membershipCampaignId"
                  type="text"
                  autocomplete="off"
                  class="w-full rounded-xl border border-neutral-200 bg-white px-2 py-2 text-sm text-neutral-900"
                  :placeholder="$t('views.compose.gumroadProductIdPlaceholder')"
                />
              </div>
            </template>
          </div>
          <div
            v-if="composerPaymentOpen"
            id="composer-payment-panel"
            class="rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-3 text-sm"
          >
            <p class="text-xs font-medium text-neutral-700">{{ $t("views.compose.paymentTitle") }}</p>
            <p class="mt-1 text-xs text-neutral-600">{{ $t("views.compose.paymentHint") }}</p>
            <label class="mt-3 flex cursor-pointer items-center gap-2 text-sm text-neutral-800">
              <input v-model="paymentUsePayPal" type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-lime-600" />
              <span>{{ $t("views.compose.paypalPaymentOpen") }}</span>
            </label>
            <div v-if="paymentUsePayPal" class="mt-3">
              <label class="mb-0.5 block text-xs text-neutral-500" for="paypal-plan-id">{{
                $t("views.compose.paypalPlanId")
              }}</label>
              <select
                v-if="activePayPalPlans.length > 0"
                id="paypal-plan-id"
                v-model="payPalPlanId"
                class="w-full rounded-xl border border-neutral-200 bg-white px-2 py-2 text-sm text-neutral-900"
              >
                <option value="">{{ $t("views.compose.paypalPlanSelectPlaceholder") }}</option>
                <option v-for="plan in activePayPalPlans" :key="plan.id" :value="plan.plan_id">
                  {{ plan.label || plan.plan_id }}
                </option>
              </select>
              <input
                v-else
                id="paypal-plan-id"
                v-model="payPalPlanId"
                type="text"
                autocomplete="off"
                class="w-full rounded-xl border border-neutral-200 bg-white px-2 py-2 text-sm text-neutral-900"
                :placeholder="$t('views.compose.paypalPlanIdPlaceholder')"
              />
              <p class="mt-2 text-xs text-neutral-600">
                {{
                  activePayPalPlans.length > 0
                    ? $t("views.compose.paypalPlanSelectHint")
                    : paypalPlansLoading
                      ? $t("app.loadingShort")
                      : $t("views.compose.paypalPlanHint")
                }}
                <RouterLink v-if="!paypalPlansLoading && activePayPalPlans.length === 0" to="/settings" class="font-medium text-lime-800 underline">
                  {{ $t("views.compose.paypalPlanSettingsLink") }}
                </RouterLink>
              </p>
            </div>
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

    <p v-if="err" class="border-t border-neutral-200 px-4 py-3 text-sm text-red-600">{{ err }}</p>
  </div>
</template>
