<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRouter } from "vue-router";
import { formatUpdatedAt } from "../i18n";
import { api } from "../lib/api";
import { getAccessToken } from "../auth";

type MeResp = { handle: string };
type NoteRow = { id: string; title: string; status: string; updated_at: string };

const router = useRouter();
const { t } = useI18n();
const err = ref("");
const loading = ref(true);
const items = ref<NoteRow[]>([]);

async function load() {
  const token = getAccessToken();
  if (!token) {
    await router.replace("/login");
    return;
  }
  loading.value = true;
  err.value = "";
  try {
    const me = await api<MeResp>("/api/v1/me", { method: "GET", token });
    const h = (me.handle ?? "").trim();
    if (!h) {
      err.value = t("views.notesList.loadUserFailed");
      items.value = [];
      return;
    }
    const res = await api<{ items: NoteRow[] }>(`/api/v1/users/by-handle/${encodeURIComponent(h)}/notes`, {
      method: "GET",
      token,
    });
    items.value = res.items ?? [];
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.notesList.loadFailed");
    items.value = [];
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  void load();
});
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="flex h-14 items-center justify-between gap-3">
      <h1 class="truncate text-lg font-bold">{{ $t("views.notesList.title") }}</h1>
      <RouterLink
        to="/notes/new"
        class="shrink-0 rounded-full bg-lime-600 px-4 py-1.5 text-sm font-semibold text-white hover:bg-lime-700"
      >
        {{ $t("views.notesList.create") }}
      </RouterLink>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="flex items-center justify-between gap-3 px-4 py-4">
      <h1 class="text-xl font-bold">{{ $t("views.notesList.title") }}</h1>
      <RouterLink
        to="/notes/new"
        class="shrink-0 rounded-full bg-lime-600 px-4 py-1.5 text-sm font-semibold text-white hover:bg-lime-700"
      >
        {{ $t("views.notesList.create") }}
      </RouterLink>
    </div>
  </Teleport>
  <div class="min-h-0 w-full min-w-0 text-neutral-900">
    <p v-if="err" class="border-b border-neutral-200 px-4 py-2 text-sm text-red-600">{{ err }}</p>
    <p v-if="loading" class="px-4 py-12 text-center text-sm text-neutral-500">{{ $t("app.loading") }}</p>
    <ul v-else-if="items.length" class="divide-y divide-neutral-200">
      <li v-for="n in items" :key="n.id">
        <RouterLink
          :to="`/notes/${encodeURIComponent(n.id)}`"
          class="flex flex-col gap-0.5 px-4 py-3 transition-colors hover:bg-lime-50/80"
        >
          <span class="font-medium text-neutral-900">{{ n.title?.trim() || $t("views.notesList.untitled") }}</span>
          <span class="text-xs text-neutral-500">
            <span
              class="rounded px-1.5 py-0.5 font-medium"
              :class="n.status === 'draft' ? 'bg-amber-100 text-amber-900' : 'bg-lime-100 text-lime-900'"
            >
              {{ n.status === "draft" ? $t("views.notesList.statusDraft") : $t("views.notesList.statusPublished") }}
            </span>
            · {{ formatUpdatedAt(n.updated_at) }}
          </span>
        </RouterLink>
      </li>
    </ul>
    <p v-else class="px-4 py-12 text-center text-sm text-neutral-500">{{ $t("views.notesList.empty") }}</p>
  </div>
</template>
