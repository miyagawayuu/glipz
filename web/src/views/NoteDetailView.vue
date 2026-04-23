<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import { formatUpdatedAt } from "../i18n";
import { api } from "../lib/api";
import Icon from "../components/Icon.vue";
import { fullHandleAt } from "../lib/feedDisplay";
import { renderNoteMarkdown } from "../lib/noteRender";

type NotePayload = {
  id: string;
  title: string;
  body_md: string;
  body_premium_md: string;
  editor_mode: string;
  status: string;
  visibility: string;
  created_at: string;
  updated_at: string;
  user_handle: string;
  user_display_name: string;
  user_avatar_url?: string;
  is_owner: boolean;
  premium_locked: boolean;
};

const route = useRoute();
const router = useRouter();
const { t } = useI18n();

const note = ref<NotePayload | null>(null);
const err = ref("");
const loading = ref(true);
const htmlFree = ref("");
const htmlPremium = ref("");

const visibilityLabel = computed(() => {
  const n = note.value;
  if (!n) return "";
  if (n.visibility === "followers") return t("views.noteDetail.visibilityFollowers");
  if (n.visibility === "private") return t("views.noteDetail.visibilityPrivate");
  return t("views.noteDetail.visibilityPublic");
});

async function load() {
  const token = getAccessToken();
  if (!token) {
    await router.replace("/login");
    return;
  }
  const id = typeof route.params.noteId === "string" ? route.params.noteId : "";
  if (!id || id === "new") {
    err.value = t("views.noteDetail.notFound");
    note.value = null;
    loading.value = false;
    return;
  }
  loading.value = true;
  err.value = "";
  try {
    const res = await api<{ note: NotePayload }>(`/api/v1/notes/${encodeURIComponent(id)}`, {
      method: "GET",
      token,
    });
    note.value = res.note;
    htmlFree.value = renderNoteMarkdown(res.note.body_md);
    htmlPremium.value = res.note.body_premium_md ? renderNoteMarkdown(res.note.body_premium_md) : "";
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.noteDetail.loadFailed");
    note.value = null;
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  void load();
});

watch(
  () => route.params.noteId,
  () => {
    void load();
  },
);
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="flex h-12 items-center gap-3">
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="$t('common.back.previous')"
        @click="router.back()"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <div v-if="note" class="min-w-0 flex-1">
        <h1 class="truncate text-lg font-bold">{{ note.title || $t("views.noteDetail.untitled") }}</h1>
      </div>
      <RouterLink
        v-if="note?.is_owner"
        :to="`/notes/${note.id}/edit`"
        class="shrink-0 rounded-full bg-lime-600 px-4 py-1.5 text-sm font-semibold text-white hover:bg-lime-700"
      >
        {{ $t("views.noteDetail.edit") }}
      </RouterLink>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="flex h-12 items-center gap-3 px-4">
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="$t('common.back.previous')"
        @click="router.back()"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <div v-if="note" class="min-w-0 flex-1">
        <h1 class="truncate text-lg font-bold">{{ note.title || $t("views.noteDetail.untitled") }}</h1>
      </div>
      <RouterLink
        v-if="note?.is_owner"
        :to="`/notes/${note.id}/edit`"
        class="shrink-0 rounded-full bg-lime-600 px-4 py-1.5 text-sm font-semibold text-white hover:bg-lime-700"
      >
        {{ $t("views.noteDetail.edit") }}
      </RouterLink>
    </div>
  </Teleport>
  <div class="min-h-0 w-full min-w-0 text-neutral-900">
    <p v-if="loading" class="px-4 py-12 text-center text-sm text-neutral-500">{{ $t("app.loading") }}</p>
    <p v-else-if="err" class="px-4 py-8 text-center text-sm text-red-600">{{ err }}</p>
    <article v-else-if="note" class="px-4 py-8">
      <p class="mb-6 truncate text-xs text-neutral-500">
        <RouterLink v-if="note.user_handle" :to="`/@${note.user_handle}`" class="hover:text-lime-700 hover:underline">
          {{ note.user_display_name }} {{ fullHandleAt(note.user_handle) }}
        </RouterLink>
        <span class="mx-1">·</span>
        {{ formatUpdatedAt(note.updated_at) }}
        <span class="mx-1">·</span>
        {{ visibilityLabel }}
        <template v-if="note.status === 'draft'">
          <span class="mx-1">·</span>
          <span class="rounded bg-amber-100 px-1.5 py-0.5 text-amber-900">{{ $t("views.noteDetail.draftBadge") }}</span>
        </template>
      </p>
      <div
        class="note-body prose prose-neutral max-w-none prose-headings:font-bold prose-img:rounded-lg prose-video:max-w-full prose-video:rounded-lg"
        v-html="htmlFree"
      />
      <div
        v-if="note.premium_locked"
        class="mt-8 rounded-xl border border-amber-200 bg-amber-50/80 px-4 py-5 text-sm text-amber-950"
      >
        <p class="font-semibold">{{ $t("views.noteDetail.premiumTitle") }}</p>
        <p class="mt-2 text-xs leading-relaxed text-amber-900/95">
          {{ $t("views.noteDetail.premiumHintBefore") }}
          <RouterLink to="/settings" class="font-medium text-lime-800 underline hover:text-lime-900">{{
            $t("views.noteDetail.premiumHintLink")
          }}</RouterLink>
          {{ $t("views.noteDetail.premiumHintAfter") }}
        </p>
      </div>
      <div
        v-else-if="htmlPremium"
        class="note-body prose prose-neutral mt-8 max-w-none border-t border-neutral-200 pt-8 prose-headings:font-bold prose-img:rounded-lg prose-video:max-w-full prose-video:rounded-lg"
        v-html="htmlPremium"
      />
    </article>
  </div>
</template>

<style scoped>
.note-body :deep(a) {
  color: rgb(77 124 15);
  text-decoration: underline;
}
.note-body :deep(video) {
  max-width: 100%;
  border-radius: 0.5rem;
}
</style>
