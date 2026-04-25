<script setup lang="ts">
import { RouterLink } from "vue-router";
import Icon from "./Icon.vue";
import { useSecuritySettingsContext } from "../composables/useSecuritySettings";

const {
  dmSaveMsg,
  loading,
  dmInviteAutoAccept,
  dmInviteAutoDirty,
  saveDMSettings,
} = useSecuritySettingsContext();

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
      <div class="flex flex-wrap items-center justify-end gap-3">
        <button
          type="button"
          class="rounded-md bg-neutral-900 px-3 py-2 text-sm font-medium text-white hover:bg-neutral-800 disabled:opacity-50"
          :disabled="loading || !dmInviteAutoDirty"
          @click="saveDMSettings"
        >
          {{ $t("views.settings.directMessages.save") }}
        </button>
      </div>
      <p v-if="dmSaveMsg" class="text-sm font-medium text-lime-700">{{ dmSaveMsg }}</p>
    </div>
  </div>
</template>
