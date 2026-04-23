<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import EmojiInline from "./EmojiInline.vue";
import Icon from "./Icon.vue";
import VideoEmbedCard from "./VideoEmbedCard.vue";
import { fetchLinkPreview, type LinkPreview } from "../lib/linkPreview";
import { extractPreviewUrls, parseRichText } from "../lib/richText";
import { extractVideoEmbeds } from "../lib/videoEmbed";

const { t } = useI18n();
const maskPlaceholder = computed(() => t("components.postRichText.masked"));

const props = withDefaults(
  defineProps<{
    text: string;
    cardLimit?: number;
  }>(),
  {
    cardLimit: 4,
  },
);

type DisplaySegment =
  | { type: "text"; value: string }
  | { type: "link"; value: string; href: string }
  | { type: "hashtag"; value: string; tag: string }
  | { type: "emoji_shortcode"; value: string }
  | { type: "mask"; value: string };

const lines = computed(() =>
  parseRichText(props.text ?? "").map((line) => ({
    ...line,
    segments: line.segments.flatMap((segment): DisplaySegment[] => {
      if (segment.type !== "text" || !segment.value.includes(maskPlaceholder.value)) return [segment];
      const chunks = segment.value.split(maskPlaceholder.value);
      const out: DisplaySegment[] = [];
      chunks.forEach((chunk, idx) => {
        if (chunk) out.push({ type: "text", value: chunk });
        if (idx < chunks.length - 1) out.push({ type: "mask", value: maskPlaceholder.value });
      });
      return out;
    }),
  })),
);
const previewUrls = computed(() => extractPreviewUrls(props.text ?? ""));
const videoEmbeds = computed(() =>
  props.cardLimit > 0 ? extractVideoEmbeds(previewUrls.value).slice(0, props.cardLimit) : [],
);
const previews = ref<LinkPreview[]>([]);
let previewRequestId = 0;

watch(
  () => [props.text, props.cardLimit, videoEmbeds.value.map((embed) => embed.url).join("\n")] as const,
  async ([text]) => {
    const requestId = ++previewRequestId;
    const embeddedUrls = new Set(videoEmbeds.value.map((embed) => embed.url));
    const urls = extractPreviewUrls(text ?? "")
      .filter((url) => !embeddedUrls.has(url))
      .slice(0, props.cardLimit);
    if (!urls.length) {
      previews.value = [];
      return;
    }
    const rows = await Promise.all(urls.map((url) => fetchLinkPreview(url)));
    if (requestId !== previewRequestId) return;
    previews.value = rows.filter((row): row is LinkPreview => Boolean(row && row.title));
  },
  { immediate: true },
);
</script>

<template>
  <div class="space-y-3">
    <p class="whitespace-normal break-words text-[15px] leading-6 text-neutral-900">
      <template v-for="(line, lineIndex) in lines" :key="line.key">
        <template v-for="(segment, segmentIndex) in line.segments" :key="`${line.key}-${segmentIndex}`">
          <a
            v-if="segment.type === 'link'"
            :href="segment.href"
            target="_blank"
            rel="noreferrer noopener"
            class="text-lime-700 underline underline-offset-2 break-all hover:text-lime-800"
          >
            {{ segment.value }}
          </a>
          <RouterLink
            v-else-if="segment.type === 'hashtag'"
            :to="{ path: '/search', query: { q: segment.value } }"
            class="text-lime-700 underline underline-offset-2 hover:text-lime-800"
          >
            {{ segment.value }}
          </RouterLink>
          <EmojiInline
            v-else-if="segment.type === 'emoji_shortcode'"
            :token="segment.value"
            size-class="text-[1.1em]"
            image-class="h-[1.35em] w-[1.35em]"
            custom-image-class="h-[1.35em] w-auto max-w-[4.5em]"
          />
          <span
            v-else-if="segment.type === 'mask'"
            class="mx-0.5 inline-flex h-7 min-w-[8rem] select-none items-center justify-center rounded-md border border-neutral-400/70 bg-[repeating-linear-gradient(-45deg,_#d4d4d8,_#d4d4d8_8px,_#c4c4ca_8px,_#c4c4ca_16px)] px-3 align-middle text-neutral-700 shadow-inner"
          >
            <span class="sr-only">{{ segment.value }}</span>
            <Icon name="lock" class="h-4 w-4" stroke-width="2" />
          </span>
          <template v-else>{{ segment.value }}</template>
        </template>
        <br v-if="lineIndex < lines.length - 1" />
      </template>
    </p>

    <div v-if="videoEmbeds.length || previews.length" class="space-y-3">
      <VideoEmbedCard
        v-for="embed in videoEmbeds"
        :key="embed.embedUrl"
        :embed="embed"
      />
      <a
        v-for="preview in previews"
        :key="preview.url"
        :href="preview.url"
        target="_blank"
        rel="noreferrer noopener"
        class="flex overflow-hidden rounded-2xl border border-neutral-200 bg-white transition hover:border-lime-300 hover:bg-lime-50/30"
      >
        <div v-if="preview.image_url" class="hidden w-32 shrink-0 bg-neutral-100 sm:block">
          <img :src="preview.image_url" alt="" class="h-full w-full object-cover" loading="lazy" />
        </div>
        <div class="min-w-0 flex-1 p-4">
          <p v-if="preview.site_name" class="text-xs font-medium uppercase tracking-wide text-neutral-500">
            {{ preview.site_name }}
          </p>
          <p class="line-clamp-2 text-sm font-semibold text-neutral-900">{{ preview.title }}</p>
          <p v-if="preview.description" class="mt-1 line-clamp-3 text-sm text-neutral-600">
            {{ preview.description }}
          </p>
          <p class="mt-2 truncate text-xs text-neutral-500">{{ preview.url }}</p>
        </div>
      </a>
    </div>
  </div>
</template>
