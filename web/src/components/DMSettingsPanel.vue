<script setup lang="ts">
import { computed } from "vue";
import { RouterLink } from "vue-router";
import { useI18n } from "vue-i18n";
import Icon from "./Icon.vue";
import UserBadges from "./UserBadges.vue";
import { useSecuritySettingsContext } from "../composables/useSecuritySettings";

const { t } = useI18n();
const {
  me,
  dmSaveMsg,
  loading,
  dmCallTimeoutSeconds,
  dmCallTimeoutOptions,
  dmCallTimeoutDirty,
  dmCallEnabled,
  dmCallScope,
  dmCallAllowedUserIDs,
  selectableThreads,
  dmCallPolicyDirty,
  dmInviteAutoAccept,
  dmInviteAutoDirty,
  saveDMCallSettings,
} = useSecuritySettingsContext();

const dmCallScopeCurrentKey = computed(() => {
  if (!me.value?.dm_call_enabled) return "none";
  const s = me.value.dm_call_scope ?? "none";
  if (s === "none" || s === "all" || s === "followers" || s === "specific_users") return s;
  if (s === "specific_groups") return "none";
  return "none";
});

const dmCallCurrentLine = computed(() =>
  t("views.settings.security.dmCalls.currentLine", {
    seconds: me.value?.dm_call_timeout_seconds ?? 30,
    scope: t(`views.settings.security.dmCalls.scopeCurrent.${dmCallScopeCurrentKey.value}`),
  }),
);

const dmSettingsDirty = computed(
  () => dmCallTimeoutDirty.value || dmCallPolicyDirty.value || dmInviteAutoDirty.value,
);

function toggleDmInviteAuto() {
  dmInviteAutoAccept.value = !dmInviteAutoAccept.value;
}
</script>

<template>
  <div class="space-y-4">
    <div class="divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
      <RouterLink
        to="/messages"
        class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
      >
        <span class="font-medium">{{ $t("views.settings.items.messages") }}</span>
        <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
      </RouterLink>
    </div>

    <div class="space-y-3 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
      <div class="flex items-start justify-between gap-4">
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-neutral-900">{{ $t("views.settings.directMessages.autoInviteTitle") }}</p>
          <p class="mt-1 text-xs leading-relaxed text-neutral-600">
            {{ $t("views.settings.directMessages.autoInviteHint") }}
          </p>
        </div>
        <button
          type="button"
          role="switch"
          :aria-checked="dmInviteAutoAccept"
          :aria-label="$t('views.settings.directMessages.autoInviteAria')"
          class="relative inline-flex h-7 w-12 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-lime-500 focus-visible:ring-offset-2"
          :class="dmInviteAutoAccept ? 'bg-lime-600' : 'bg-neutral-300'"
          @click="toggleDmInviteAuto"
        >
          <span
            class="pointer-events-none inline-block h-6 w-6 translate-x-0 transform rounded-full bg-white shadow transition"
            :class="dmInviteAutoAccept ? 'translate-x-5' : 'translate-x-0.5'"
            aria-hidden="true"
          />
        </button>
      </div>
    </div>

    <div class="space-y-3 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
      <h3 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
        {{ $t("views.settings.sections.dmCalls") }}
      </h3>
      <p class="text-sm leading-relaxed text-neutral-600">{{ $t("views.settings.security.dmCalls.intro") }}</p>
      <label class="flex items-center gap-3 rounded-lg border border-neutral-200 bg-neutral-50/80 px-3 py-3 text-sm text-neutral-800">
        <input v-model="dmCallEnabled" type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-lime-600 focus:ring-lime-500" />
        <span>{{ $t("views.settings.security.dmCalls.enableLabel") }}</span>
      </label>
      <div class="grid gap-3 sm:grid-cols-2">
        <label class="block min-w-0 flex-1">
          <span class="mb-1 block text-sm font-medium text-neutral-700">{{ $t("views.settings.security.dmCalls.timeoutLabel") }}</span>
          <select
            v-model="dmCallTimeoutSeconds"
            class="w-full rounded-md border border-neutral-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
            :disabled="loading"
          >
            <option v-for="opt in dmCallTimeoutOptions" :key="opt.value" :value="opt.value">
              {{ opt.label }}
            </option>
          </select>
        </label>
        <label class="block min-w-0 flex-1">
          <span class="mb-1 block text-sm font-medium text-neutral-700">{{ $t("views.settings.security.dmCalls.scopeLabel") }}</span>
          <select
            v-model="dmCallScope"
            class="w-full rounded-md border border-neutral-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
            :disabled="loading || !dmCallEnabled"
          >
            <option value="all">{{ $t("views.settings.security.dmCalls.scopeAll") }}</option>
            <option value="followers">{{ $t("views.settings.security.dmCalls.scopeFollowers") }}</option>
            <option value="specific_users">{{ $t("views.settings.security.dmCalls.scopeSpecificUsers") }}</option>
          </select>
        </label>
      </div>
      <div v-if="dmCallEnabled && dmCallScope === 'specific_users'" class="space-y-2">
        <p class="text-sm font-medium text-neutral-700">{{ $t("views.settings.security.dmCalls.usersHeading") }}</p>
        <div v-if="!selectableThreads.length" class="rounded-lg border border-dashed border-neutral-200 bg-neutral-50/80 px-3 py-3 text-sm text-neutral-500">
          {{ $t("views.settings.security.dmCalls.noThreads") }}
        </div>
        <label
          v-for="thread in selectableThreads"
          :key="thread.id"
          class="flex items-center gap-3 rounded-lg border border-neutral-200 bg-neutral-50/80 px-3 py-2 text-sm text-neutral-800"
        >
          <input v-model="dmCallAllowedUserIDs" type="checkbox" :value="thread.peer_id" class="h-4 w-4 rounded border-neutral-200 text-lime-600 focus:ring-lime-500" />
          <span class="flex flex-wrap items-center gap-1.5">
            <span>{{ thread.peer_display_name || `@${thread.peer_handle}` }}</span>
            <UserBadges :badges="thread.peer_badges" size="xs" />
          </span>
        </label>
      </div>
      <div class="flex flex-wrap items-center justify-end gap-3">
        <button
          type="button"
          class="rounded-md bg-neutral-900 px-3 py-2 text-sm font-medium text-white hover:bg-neutral-800 disabled:opacity-50"
          :disabled="loading || !dmSettingsDirty"
          @click="saveDMCallSettings"
        >
          {{ $t("views.settings.security.dmCalls.save") }}
        </button>
      </div>
      <p class="text-xs text-neutral-500">{{ dmCallCurrentLine }}</p>
      <p v-if="dmSaveMsg" class="text-sm font-medium text-lime-700">{{ dmSaveMsg }}</p>
    </div>
  </div>
</template>
