<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import EmojiInline from "../components/EmojiInline.vue";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";
import {
  createAdminSiteCustomEmoji,
  deleteAdminSiteCustomEmoji,
  listAdminSiteCustomEmojis,
  patchAdminSiteCustomEmoji,
} from "../lib/customEmojiApi";
import { refreshCustomEmojiCatalog } from "../lib/customEmojis";
import { SAFE_PROFILE_IMAGE_ACCEPT } from "../lib/composerMedia";
import type { CustomEmoji } from "../types/customEmoji";

type DraftRow = {
  shortcode_name: string;
  is_enabled: boolean;
  file: File | null;
};

const { t } = useI18n();
const router = useRouter();

const loading = ref(true);
const isAdmin = ref(false);
const error = ref("");
const notice = ref("");
const createBusy = ref(false);
const rowBusy = ref<string | null>(null);
const items = ref<CustomEmoji[]>([]);
const drafts = reactive<Record<string, DraftRow>>({});
const createForm = reactive({
  shortcode_name: "",
  is_enabled: true,
  file: null as File | null,
});

function syncDrafts(rows: CustomEmoji[]) {
  for (const row of rows) {
    drafts[row.id] = {
      shortcode_name: row.shortcode_name,
      is_enabled: row.is_enabled,
      file: null,
    };
  }
}

async function loadPage() {
  const token = getAccessToken();
  if (!token) {
    await router.replace("/login");
    return;
  }
  const me = await api<{ is_site_admin?: boolean }>("/api/v1/me", { method: "GET", token });
  isAdmin.value = !!me.is_site_admin;
  if (!isAdmin.value) {
    error.value = t("views.adminCustomEmojis.errors.adminOnly");
    return;
  }
  items.value = await listAdminSiteCustomEmojis(token);
  syncDrafts(items.value);
}

async function createEmoji() {
  const token = getAccessToken();
  if (!token || !createForm.file) return;
  createBusy.value = true;
  error.value = "";
  notice.value = "";
  try {
    const item = await createAdminSiteCustomEmoji(token, {
      shortcode_name: createForm.shortcode_name,
      file: createForm.file,
      is_enabled: createForm.is_enabled,
    });
    items.value = [...items.value, item];
    syncDrafts(items.value);
    createForm.shortcode_name = "";
    createForm.is_enabled = true;
    createForm.file = null;
    await refreshCustomEmojiCatalog(token);
    notice.value = t("views.adminCustomEmojis.created");
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : t("views.adminCustomEmojis.errors.create");
  } finally {
    createBusy.value = false;
  }
}

async function saveEmoji(item: CustomEmoji) {
  const token = getAccessToken();
  const draft = drafts[item.id];
  if (!token || !draft) return;
  rowBusy.value = item.id;
  error.value = "";
  notice.value = "";
  try {
    const updated = await patchAdminSiteCustomEmoji(token, item.id, draft);
    items.value = items.value.map((row) => (row.id === item.id ? updated : row));
    syncDrafts(items.value);
    await refreshCustomEmojiCatalog(token);
    notice.value = t("views.adminCustomEmojis.saved");
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : t("views.adminCustomEmojis.errors.save");
  } finally {
    rowBusy.value = null;
  }
}

async function removeEmoji(item: CustomEmoji) {
  const token = getAccessToken();
  if (!token) return;
  rowBusy.value = item.id;
  error.value = "";
  notice.value = "";
  try {
    await deleteAdminSiteCustomEmoji(token, item.id);
    items.value = items.value.filter((row) => row.id !== item.id);
    delete drafts[item.id];
    await refreshCustomEmojiCatalog(token);
    notice.value = t("views.adminCustomEmojis.deleted");
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : t("views.adminCustomEmojis.errors.delete");
  } finally {
    rowBusy.value = null;
  }
}

onMounted(async () => {
  loading.value = true;
  error.value = "";
  try {
    await loadPage();
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : t("views.adminCustomEmojis.errors.load");
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div class="mx-auto max-w-3xl px-4 py-8">
    <header>
      <h1 class="text-xl font-semibold text-neutral-900">{{ $t("views.adminCustomEmojis.title") }}</h1>
      <p class="mt-2 text-sm text-neutral-600">{{ $t("views.adminCustomEmojis.description") }}</p>
    </header>

    <p v-if="error" class="mt-4 rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">{{ error }}</p>
    <p v-if="notice" class="mt-4 rounded border border-lime-200 bg-lime-50 px-3 py-2 text-sm text-lime-800">{{ notice }}</p>
    <p v-if="loading" class="mt-6 text-sm text-neutral-500">{{ $t("views.adminCustomEmojis.loading") }}</p>

    <template v-else-if="isAdmin">
      <section class="mt-6 rounded-2xl border border-neutral-200 bg-white p-5 shadow-sm">
        <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.adminCustomEmojis.createHeading") }}</h2>
        <div class="mt-4 grid gap-4 sm:grid-cols-2">
          <label class="block text-sm">
            <span class="mb-1 block font-medium text-neutral-800">{{ $t("views.adminCustomEmojis.shortcodeLabel") }}</span>
            <input
              v-model="createForm.shortcode_name"
              type="text"
              class="w-full rounded-xl border border-neutral-200 px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              :placeholder="$t('views.adminCustomEmojis.shortcodePlaceholder')"
            />
          </label>
          <label class="block text-sm">
            <span class="mb-1 block font-medium text-neutral-800">{{ $t("views.adminCustomEmojis.fileLabel") }}</span>
            <input type="file" :accept="SAFE_PROFILE_IMAGE_ACCEPT" class="block w-full text-sm text-neutral-700" @change="(e) => createForm.file = (e.target as HTMLInputElement).files?.[0] ?? null" />
          </label>
        </div>
        <label class="mt-4 flex items-center gap-2 text-sm text-neutral-700">
          <input v-model="createForm.is_enabled" type="checkbox" class="h-4 w-4 rounded border-neutral-300 text-lime-600 focus:ring-lime-500" />
          <span>{{ $t("views.adminCustomEmojis.enabledLabel") }}</span>
        </label>
        <div class="mt-4 flex items-center justify-between gap-3">
          <p class="text-sm text-neutral-500">{{ $t("views.adminCustomEmojis.shortcodeHint") }}</p>
          <button
            type="button"
            class="rounded-xl bg-lime-600 px-4 py-2.5 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
            :disabled="createBusy || !createForm.shortcode_name.trim() || !createForm.file"
            @click="createEmoji"
          >
            {{ createBusy ? $t("views.adminCustomEmojis.createBusy") : $t("views.adminCustomEmojis.create") }}
          </button>
        </div>
      </section>

      <section class="mt-6 rounded-2xl border border-neutral-200 bg-white p-5 shadow-sm">
        <div class="flex items-center justify-between gap-3">
          <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.adminCustomEmojis.listHeading") }}</h2>
          <span class="text-xs text-neutral-500">{{ items.length }}</span>
        </div>
        <p v-if="!items.length" class="mt-4 text-sm text-neutral-500">{{ $t("views.adminCustomEmojis.empty") }}</p>
        <div v-else class="mt-4 space-y-4">
          <article v-for="item in items" :key="item.id" class="rounded-2xl border border-neutral-200 bg-neutral-50 p-4">
            <div class="flex items-start justify-between gap-4">
              <div class="flex min-w-0 items-center gap-3">
                <span class="inline-flex min-w-12 max-w-24 items-center justify-center rounded-2xl bg-white px-2 py-2 shadow-sm">
                  <EmojiInline :token="item.shortcode" image-class="h-8 w-8" custom-image-class="h-8 w-auto max-w-20" size-class="text-2xl" />
                </span>
                <div class="min-w-0">
                  <p class="truncate text-sm font-semibold text-neutral-900">{{ item.shortcode }}</p>
                  <p class="mt-1 text-xs text-neutral-500">{{ item.is_enabled ? $t("views.adminCustomEmojis.enabledState") : $t("views.adminCustomEmojis.disabledState") }}</p>
                </div>
              </div>
              <button
                type="button"
                class="rounded-lg px-3 py-2 text-xs font-semibold text-red-700 hover:bg-red-50"
                :disabled="rowBusy === item.id"
                @click="removeEmoji(item)"
              >
                {{ $t("views.adminCustomEmojis.delete") }}
              </button>
            </div>

            <div class="mt-4 grid gap-4 sm:grid-cols-2">
              <label class="block text-sm">
                <span class="mb-1 block font-medium text-neutral-800">{{ $t("views.adminCustomEmojis.shortcodeLabel") }}</span>
                <input
                  v-model="drafts[item.id].shortcode_name"
                  type="text"
                  class="w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
                />
              </label>
              <label class="block text-sm">
                <span class="mb-1 block font-medium text-neutral-800">{{ $t("views.adminCustomEmojis.replaceFileLabel") }}</span>
                <input type="file" :accept="SAFE_PROFILE_IMAGE_ACCEPT" class="block w-full text-sm text-neutral-700" @change="(e) => drafts[item.id].file = (e.target as HTMLInputElement).files?.[0] ?? null" />
              </label>
            </div>
            <label class="mt-4 flex items-center gap-2 text-sm text-neutral-700">
              <input v-model="drafts[item.id].is_enabled" type="checkbox" class="h-4 w-4 rounded border-neutral-300 text-lime-600 focus:ring-lime-500" />
              <span>{{ $t("views.adminCustomEmojis.enabledLabel") }}</span>
            </label>
            <div class="mt-4 flex justify-end">
              <button
                type="button"
                class="rounded-xl bg-neutral-900 px-4 py-2.5 text-sm font-semibold text-white hover:bg-neutral-800 disabled:opacity-50"
                :disabled="rowBusy === item.id"
                @click="saveEmoji(item)"
              >
                {{ rowBusy === item.id ? $t("views.adminCustomEmojis.saveBusy") : $t("views.adminCustomEmojis.save") }}
              </button>
            </div>
          </article>
        </div>
      </section>
    </template>
  </div>
</template>
