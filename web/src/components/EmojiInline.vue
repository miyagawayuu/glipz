<script setup lang="ts">
import { computed, onMounted } from "vue";
import { ensureCustomEmojiCatalog, ensureRemoteCustomEmojiResolved, resolveEmojiToken } from "../lib/customEmojis";

const props = withDefaults(defineProps<{
  token: string;
  sizeClass?: string;
  imageClass?: string;
  customImageClass?: string;
}>(), {
  sizeClass: "text-base",
  imageClass: "h-[1.25em] w-[1.25em]",
  customImageClass: "h-[1.25em] w-auto max-w-[3em]",
});

const resolved = computed(() => resolveEmojiToken(props.token));

onMounted(() => {
  void ensureCustomEmojiCatalog();
  void ensureRemoteCustomEmojiResolved(props.token);
});
</script>

<template>
  <img
    v-if="resolved.imageUrl"
    :src="resolved.imageUrl"
    :alt="resolved.text"
    class="inline-block align-[-0.2em]"
    :class="resolved.kind === 'custom' ? customImageClass : imageClass"
    loading="lazy"
  />
  <span v-else class="inline-block leading-none" :class="sizeClass">{{ resolved.text }}</span>
</template>
