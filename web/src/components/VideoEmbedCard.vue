<script setup lang="ts">
import type { VideoEmbed } from "../lib/videoEmbed";

const props = defineProps<{
  embed: VideoEmbed;
}>();

const isSteamEmbed = props.embed.layout === "steam";
</script>

<template>
  <div
    class="overflow-hidden rounded-2xl border border-neutral-200 shadow-sm"
    :class="isSteamEmbed ? 'bg-[#1b2838]' : 'bg-black'"
  >
    <div class="w-full" :class="isSteamEmbed ? 'mx-auto max-w-[646px] bg-[#1b2838]' : 'aspect-video bg-neutral-950'">
      <iframe
        :src="embed.embedUrl"
        :title="embed.title"
        class="border-0"
        :class="isSteamEmbed ? 'block h-[190px] w-full' : 'h-full w-full'"
        loading="lazy"
        allow="autoplay; encrypted-media; picture-in-picture"
        allowfullscreen
        referrerpolicy="strict-origin-when-cross-origin"
        sandbox="allow-scripts allow-same-origin allow-presentation"
      />
    </div>
  </div>
</template>
