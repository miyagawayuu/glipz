<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import type { VideoEmbed } from "../lib/videoEmbed";

const props = defineProps<{
  embed: VideoEmbed;
}>();

const scriptRoot = ref<HTMLDivElement | null>(null);
const isSteamEmbed = props.embed.layout === "steam";

function renderScriptEmbed() {
  const root = scriptRoot.value;
  if (!root || props.embed.embedKind !== "script") return;
  root.innerHTML = "";
  const script = document.createElement("script");
  script.type = "application/javascript";
  script.src = props.embed.embedUrl;
  script.async = true;
  root.appendChild(script);
}

watch(
  () => [props.embed.embedKind, props.embed.embedUrl] as const,
  () => {
    renderScriptEmbed();
  },
);

onMounted(() => {
  renderScriptEmbed();
});

onBeforeUnmount(() => {
  if (scriptRoot.value) scriptRoot.value.innerHTML = "";
});
</script>

<template>
  <div
    class="overflow-hidden rounded-2xl border border-neutral-200 shadow-sm"
    :class="isSteamEmbed ? 'bg-[#1b2838]' : 'bg-black'"
  >
    <div
      v-if="embed.embedKind === 'iframe'"
      class="w-full"
      :class="isSteamEmbed ? 'mx-auto max-w-[646px] bg-[#1b2838]' : 'aspect-video bg-neutral-950'"
    >
      <iframe
        :src="embed.embedUrl"
        :title="embed.title"
        class="border-0"
        :class="isSteamEmbed ? 'block h-[190px] w-full' : 'h-full w-full'"
        loading="lazy"
        allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share"
        allowfullscreen
        referrerpolicy="strict-origin-when-cross-origin"
      />
    </div>
    <div v-else class="bg-white">
      <div ref="scriptRoot" class="mx-auto w-full max-w-[640px]" />
      <noscript>
        <a :href="embed.url" target="_blank" rel="noreferrer noopener">{{ embed.title }}</a>
      </noscript>
    </div>
  </div>
</template>
