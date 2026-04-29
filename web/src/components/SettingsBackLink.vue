<script setup lang="ts">
import { RouterLink } from "vue-router";
</script>

<template>
  <RouterLink to="/settings" class="text-sm font-medium text-lime-800 hover:underline">
    {{ $t("views.settings.backToSettings") }}
  </RouterLink>
</template>
