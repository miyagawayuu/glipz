<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { MeResp } from "../../composables/useSecuritySettings";
import { getFanclubLinkStatus } from "../../fanclub/me";

const props = defineProps<{
  me: MeResp | null;
  loading: boolean;
  onConnectMember: () => void;
  onConnectCreator: () => void;
  onDisconnectMember: () => void;
  onDisconnectCreator: () => void;
}>();

const { t } = useI18n();

const status = computed(() => getFanclubLinkStatus(props.me, "patreon"));
</script>

<template>
  <div class="space-y-3 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
    <p class="text-xs leading-relaxed text-neutral-600">
      {{ $t("views.settings.security.patreon.intro") }}
    </p>

    <div v-if="status" class="space-y-2 text-xs text-neutral-700">
      <p>
        {{ $t("views.settings.security.patreon.memberStatus") }}
        <span class="font-medium">{{ status.member_linked ? $t("views.settings.security.patreon.linked") : $t("views.settings.security.patreon.notLinked") }}</span>
        · {{ $t("views.settings.security.patreon.creatorStatus") }}
        <span class="font-medium">{{ status.creator_linked ? $t("views.settings.security.patreon.linked") : $t("views.settings.security.patreon.notLinked") }}</span>
      </p>
    </div>

    <div class="flex flex-wrap gap-2">
      <button
        type="button"
        class="rounded-md bg-neutral-900 px-3 py-2 text-sm font-medium text-white hover:bg-neutral-800 disabled:opacity-50"
        :disabled="loading"
        @click="onConnectMember"
      >
        {{ $t("views.settings.security.patreon.connectMember") }}
      </button>
      <button
        type="button"
        class="rounded-md border border-neutral-200 bg-white px-3 py-2 text-sm font-medium text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
        :disabled="loading"
        @click="onDisconnectMember"
      >
        {{ $t("views.settings.security.patreon.disconnectMember") }}
      </button>
    </div>

    <div class="flex flex-wrap gap-2 border-t border-neutral-200 pt-3">
      <button
        type="button"
        class="rounded-md bg-lime-600 px-3 py-2 text-sm font-medium text-white hover:bg-lime-700 disabled:opacity-50"
        :disabled="loading"
        @click="onConnectCreator"
      >
        {{ $t("views.settings.security.patreon.connectCreator") }}
      </button>
      <button
        type="button"
        class="rounded-md border border-red-200 bg-white px-3 py-2 text-sm font-medium text-red-700 hover:bg-red-50 disabled:opacity-50"
        :disabled="loading"
        @click="onDisconnectCreator"
      >
        {{ $t("views.settings.security.patreon.disconnectCreator") }}
      </button>
    </div>
  </div>
</template>

