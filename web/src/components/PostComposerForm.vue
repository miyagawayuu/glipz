<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { useI18n } from "vue-i18n";
import { getAccessToken } from "../auth";
import Icon from "./Icon.vue";
import ComposerEmojiPicker from "./ComposerEmojiPicker.vue";
import GlipzAudioPlayer from "./GlipzAudioPlayer.vue";
import GlipzVideoPlayer from "./GlipzVideoPlayer.vue";
import { SAFE_MEDIA_ACCEPT } from "../lib/composerMedia";
import { avatarInitials, handleAt } from "../lib/feedDisplay";
import { patreonSettingsPath } from "../composables/usePatreonComposer";
import { usePostComposerForm, type ComposerVisibility, type PostComposerMode } from "../composables/usePostComposerForm";
import type { TimelinePost } from "../types/timeline";

const props = withDefaults(
  defineProps<{
    mode?: PostComposerMode;
    communityId?: string | null;
    viewerEmail?: string | null;
    viewerHandle?: string | null;
    viewerAvatarUrl?: string | null;
    patreonEnabled?: boolean;
    layout?: "inline" | "modal" | "embedded";
  }>(),
  {
    mode: "normal",
    communityId: null,
    viewerEmail: null,
    viewerHandle: null,
    viewerAvatarUrl: null,
    patreonEnabled: false,
    layout: "inline",
  },
);

const emit = defineEmits<{
  submitted: [detail: { mode: PostComposerMode; communityId?: string | null }];
}>();

const { t } = useI18n();
const modeRef = computed(() => props.mode);
const communityIdRef = computed(() => props.communityId);
const patreonEnabledRef = computed(() => props.patreonEnabled);
const detail = computed(() => ({ mode: props.mode, communityId: props.communityId }));

const composer = usePostComposerForm({
  mode: modeRef,
  communityId: communityIdRef,
  patreonEnabled: patreonEnabledRef,
  t,
  onSubmitted: () => emit("submitted", detail.value),
  eventDetail: () => detail.value,
});

const {
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
  submitPost,
  composerAttachmentLabel,
} = composer;

const patreonSettingsHref = patreonSettingsPath;
const avatarImgFailed = ref(false);
const rootClass = computed(() =>
  props.layout === "embedded"
    ? "composer-anchor flex gap-3"
    : props.layout === "modal"
    ? "composer-anchor flex gap-3 px-4 py-4"
    : "composer-anchor hidden gap-3 border-b border-neutral-200 px-4 py-3 lg:flex",
);
const visibilityOptions = computed<Array<{ value: ComposerVisibility; label: string; description: string }>>(() => [
  { value: "public", label: t("views.compose.visibility.public.label"), description: t("views.compose.visibility.public.description") },
  { value: "logged_in", label: t("views.compose.visibility.loggedIn.label"), description: t("views.compose.visibility.loggedIn.description") },
  { value: "followers", label: t("views.compose.visibility.followers.label"), description: t("views.compose.visibility.followers.description") },
  { value: "private", label: t("views.compose.visibility.private.label"), description: t("views.compose.visibility.private.description") },
]);

function composerVisibilityMeta(value: ComposerVisibility) {
  return visibilityOptions.value.find((option) => option.value === value) ?? visibilityOptions.value[0];
}

function loadPatreonIfAvailable() {
  const token = getAccessToken();
  if (token) void loadPatreon(token);
}

async function connectPatreonFromComposer() {
  const returnTo = `${window.location.pathname}${window.location.search}${window.location.hash}`;
  await connectPatreon(returnTo || "/feed");
}

onMounted(loadPatreonIfAvailable);
watch(() => props.patreonEnabled, loadPatreonIfAvailable);
watch(() => props.viewerAvatarUrl, () => {
  avatarImgFailed.value = false;
});

defineExpose({
  startReply,
  cancelReply,
  focus: () => composerCaptionEl.value?.focus(),
});
</script>

<template>
  <div :class="rootClass">
    <div
      class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-lime-500 text-xs font-bold text-white"
      aria-hidden="true"
    >
      <img
        v-if="viewerAvatarUrl && !avatarImgFailed"
        :src="viewerAvatarUrl"
        alt=""
        class="h-full w-full object-cover"
        @error="avatarImgFailed = true"
      />
      <span v-else>{{ viewerEmail ? avatarInitials(viewerEmail) : "?" }}</span>
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
        :placeholder="
          replyingTo
            ? $t('views.compose.replyPlaceholder', { handle: handleAt(replyingTo) })
            : isCommunityMode
              ? $t('components.sidebarCompose.communityPlaceholder')
              : $t('views.compose.placeholder')
        "
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
              :accept="SAFE_MEDIA_ACCEPT"
              multiple
              class="hidden"
              :disabled="attachmentPickerDisabled"
              @change="onFilesSelect"
            />
            <span class="sr-only">{{ $t("views.compose.addImages") }}</span>
            <Icon name="image" class="h-5 w-5" />
          </label>
          <ComposerEmojiPicker :disabled="busy" :viewer-handle="viewerHandle" @select="insertComposerEmoji" />
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
            v-if="!isCommunityMode"
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
            v-if="fanclubComposerEnabled"
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
            v-if="!isCommunityMode"
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
            v-if="!isCommunityMode"
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
          <span class="text-xs text-neutral-500">{{ composerAttachmentLabel(selectedFiles) }}</span>
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
        v-if="!isCommunityMode && composerScheduleOpen"
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
        v-if="!isCommunityMode && composerVisibilityOpen"
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
        v-if="composerNsfwOpen || (!isCommunityMode && composerPasswordOpen) || (fanclubComposerEnabled && composerMembershipOpen)"
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
          v-if="!isCommunityMode && composerPasswordOpen"
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
          v-if="fanclubComposerEnabled && composerMembershipOpen"
          id="feed-composer-membership-panel"
          class="rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-3 text-sm"
        >
          <p class="text-xs font-medium text-neutral-700">{{ $t("views.compose.membershipTitle") }}</p>
          <p class="mt-1 text-xs text-neutral-600">{{ $t("views.compose.membershipHint") }}</p>
          <div class="mt-3 flex flex-wrap gap-2">
            <button
              v-if="patreonAvailable"
              type="button"
              class="rounded-full border border-lime-500 bg-lime-50 px-3 py-1.5 text-xs font-medium text-lime-800"
            >
              Patreon
            </button>
          </div>
          <div v-if="!patreonConnected" class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center">
            <button
              type="button"
              :disabled="patreonConnectBusy"
              class="inline-flex w-fit rounded-full bg-lime-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
              @click="connectPatreonFromComposer"
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
          <template v-else>
            <label class="mt-3 flex cursor-pointer items-center gap-2 text-sm text-neutral-800">
              <input v-model="membershipUsePatreon" type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-lime-600" />
              <span>{{ $t("views.compose.membershipOpen") }}</span>
            </label>
            <div v-if="membershipUsePatreon" class="mt-3 space-y-2">
              <div v-if="patreonCampaigns.length" class="grid gap-2 sm:grid-cols-2">
                <div>
                  <label class="mb-0.5 block text-xs text-neutral-500" for="feed-mem-camp">{{ $t("views.compose.membershipPickCampaign") }}</label>
                  <select
                    id="feed-mem-camp"
                    v-model="membershipCampaignId"
                    class="w-full rounded-xl border border-neutral-200 bg-white px-2 py-2 text-sm text-neutral-900"
                  >
                    <option value="">{{ "—" }}</option>
                    <option v-for="c in patreonCampaigns" :key="c.id" :value="c.id">{{ c.title || c.id }}</option>
                  </select>
                </div>
                <div>
                  <label class="mb-0.5 block text-xs text-neutral-500" for="feed-mem-tier">{{ $t("views.compose.membershipPickTier") }}</label>
                  <select
                    id="feed-mem-tier"
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
                  <label class="mb-0.5 block text-xs text-neutral-500" for="feed-mem-cid">{{ $t("views.compose.membershipCampaign") }}</label>
                  <input
                    id="feed-mem-cid"
                    v-model="membershipCampaignId"
                    type="text"
                    autocomplete="off"
                    class="w-full rounded-xl border border-neutral-200 bg-white px-2 py-2 text-sm text-neutral-900"
                  />
                </div>
                <div>
                  <label class="mb-0.5 block text-xs text-neutral-500" for="feed-mem-tid">{{ $t("views.compose.membershipTier") }}</label>
                  <input
                    id="feed-mem-tid"
                    v-model="membershipTierId"
                    type="text"
                    autocomplete="off"
                    class="w-full rounded-xl border border-neutral-200 bg-white px-2 py-2 text-sm text-neutral-900"
                  />
                </div>
              </div>
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
      <p v-if="err" class="mt-3 text-sm text-red-600">{{ err }}</p>
    </div>
  </div>
</template>
