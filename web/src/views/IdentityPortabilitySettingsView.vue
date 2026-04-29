<script setup lang="ts">
import IdentityPortabilitySettings from "../components/IdentityPortabilitySettings.vue";
import SettingsBackLink from "../components/SettingsBackLink.vue";
</script>

<template>
  <div class="w-full px-4 py-8">
    <SettingsBackLink />
    <div class="mt-4">
      <h1 class="text-2xl font-bold text-neutral-900">{{ $t("routes.identityPortabilitySettings") }}</h1>
    </div>
    <div class="mt-6">
      <IdentityPortabilitySettings />
    </div>
  </div>
</template>
