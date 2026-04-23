<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import type { TimelinePost } from "../types/timeline";
import { useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";
import Icon from "../components/Icon.vue";
import { mapFeedItem } from "../lib/feedStream";
import PostTimeline from "../components/PostTimeline.vue";

const { t } = useI18n();
const router = useRouter();
const items = ref<TimelinePost[]>([]);
const err = ref("");
const busy = ref(false);

async function load() {
  const token = getAccessToken();
  if (!token) {
    await router.replace("/login");
    return;
  }
  err.value = "";
  busy.value = true;
  try {
    const res = await api<{ items: Record<string, unknown>[] }>("/api/v1/me/scheduled-posts", {
      method: "GET",
      token,
    });
    items.value = (res.items ?? []).map((x) => mapFeedItem(x as Parameters<typeof mapFeedItem>[0]));
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.scheduledPosts.loadFailed");
  } finally {
    busy.value = false;
  }
}

function patchItem(id: string, patch: Partial<TimelinePost>) {
  items.value = items.value.map((x) => (x.id === id ? { ...x, ...patch } : x));
}

function removePost(id: string) {
  items.value = items.value.filter((x) => x.id !== id);
}

onMounted(() => {
  void load();
});
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="flex h-12 items-center gap-3">
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="t('views.scheduledPosts.backAria')"
        @click="router.push('/feed')"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <h1 class="text-lg font-bold">{{ t("views.scheduledPosts.title") }}</h1>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="flex h-12 items-center gap-3 px-4">
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="t('views.scheduledPosts.backAria')"
        @click="router.push('/feed')"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <h1 class="text-lg font-bold">{{ t("views.scheduledPosts.title") }}</h1>
    </div>
  </Teleport>
  <p v-if="err" class="border-b border-neutral-200 px-4 py-3 text-sm text-red-600">{{ err }}</p>
  <p v-if="busy" class="px-4 py-8 text-center text-sm text-neutral-500">{{ t("views.scheduledPosts.loading") }}</p>
  <p v-else-if="!items.length && !err" class="px-4 py-16 text-center text-sm text-neutral-500">
    {{ t("views.scheduledPosts.empty") }}
  </p>
  <PostTimeline
    v-else
    :items="items"
    hide-post-detail-link
    :action-busy="null"
    :viewer-email="null"
    @reply="() => {}"
    @toggle-reaction="() => {}"
    @toggle-bookmark="() => {}"
    @toggle-repost="() => {}"
    @share="() => {}"
    @open-lightbox="() => {}"
    @patch-item="({ id, patch }) => patchItem(id, patch)"
    @remove-post="removePost"
  />
</template>
