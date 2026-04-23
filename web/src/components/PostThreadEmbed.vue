<script setup lang="ts">
import PostTimeline from "./PostTimeline.vue";
import type { TimelinePost } from "../types/timeline";

defineEmits<{
  reply: [it: TimelinePost];
  toggleReaction: [it: TimelinePost, emoji: string];
  toggleBookmark: [it: TimelinePost];
  toggleRepost: [it: TimelinePost];
  share: [it: TimelinePost];
  openLightbox: [urls: string[], index: number];
  patchItem: [payload: { id: string; patch: Partial<TimelinePost> }];
  removePost: [id: string];
}>();

defineProps<{
  items: TimelinePost[];
  threadArticleIndentByPostId: Record<string, number>;
  actionBusy: string | null;
  viewerEmail?: string | null;
  viewerIsAdmin?: boolean | null;
  hidePostDetailLink?: boolean;
}>();
</script>

<template>
  <PostTimeline
    :items="items"
    :action-busy="actionBusy"
    :viewer-email="viewerEmail"
    :viewer-is-admin="viewerIsAdmin"
    :hide-post-detail-link="hidePostDetailLink ?? true"
    :embed-thread-replies="false"
    :thread-article-indent-by-post-id="threadArticleIndentByPostId"
    @reply="$emit('reply', $event)"
    @toggle-reaction="(it, emoji) => $emit('toggleReaction', it, emoji)"
    @toggle-bookmark="$emit('toggleBookmark', $event)"
    @toggle-repost="$emit('toggleRepost', $event)"
    @share="$emit('share', $event)"
    @open-lightbox="(...args) => $emit('openLightbox', ...args)"
    @patch-item="$emit('patchItem', $event)"
    @remove-post="$emit('removePost', $event)"
  />
</template>
