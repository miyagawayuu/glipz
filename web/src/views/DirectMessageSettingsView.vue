<script setup lang="ts">
import { provide } from "vue";
import DMSettingsPanel from "../components/DMSettingsPanel.vue";
import SettingsBackLink from "../components/SettingsBackLink.vue";
import { securitySettingsKey, useSecuritySettings } from "../composables/useSecuritySettings";

provide(securitySettingsKey, useSecuritySettings());
</script>

<template>
  <div class="w-full px-4 py-8">
    <SettingsBackLink />
    <div class="mt-4">
      <h1 class="text-2xl font-bold text-neutral-900">{{ $t("routes.directMessageSettings") }}</h1>
    </div>
    <div class="mt-6">
      <DMSettingsPanel />
    </div>
  </div>
</template>
