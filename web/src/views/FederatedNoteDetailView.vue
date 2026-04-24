<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import { formatUpdatedAt } from "../i18n";
import { api, apiPublicGet } from "../lib/api";
import Icon from "../components/Icon.vue";
import { renderNoteMarkdown } from "../lib/noteRender";
import { unlockFederatedNote } from "../lib/federationNotes";

type FederatedNotePayload = {
  id: string;
  object_iri: string;
  actor_iri: string;
  actor_acct: string;
  actor_name: string;
  actor_icon_url?: string;
  actor_profile_url?: string;
  title: string;
  body_md: string;
  body_premium_md: string;
  premium_locked: boolean;
  visibility: string;
  published_at: string;
  updated_at: string;
  has_premium: boolean;
  paywall_provider: string;
  patreon_campaign_id: string;
  patreon_required_reward_tier_id: string;
  unlock_url: string;
};

const route = useRoute();
const router = useRouter();
const { t } = useI18n();

const note = ref<FederatedNotePayload | null>(null);
const err = ref("");
const loading = ref(true);
const unlocking = ref(false);
const htmlFree = ref("");
const htmlPremium = ref("");
const password = ref("");

const viewerAuthed = computed(() => !!getAccessToken());

async function load() {
  const id = typeof route.params.incomingId === "string" ? route.params.incomingId : "";
  if (!id) {
    err.value = "not_found";
    note.value = null;
    loading.value = false;
    return;
  }
  loading.value = true;
  err.value = "";
  try {
    const token = getAccessToken();
    const path = `/api/v1/public/federation/notes/${encodeURIComponent(id)}`;
    const res = token
      ? await api<{ item: FederatedNotePayload }>(path, { method: "GET", token })
      : await apiPublicGet<{ item: FederatedNotePayload }>(path);
    note.value = res.item;
    htmlFree.value = renderNoteMarkdown(res.item.body_md);
    htmlPremium.value = res.item.body_premium_md ? renderNoteMarkdown(res.item.body_premium_md) : "";
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.noteDetail.loadFailed");
    note.value = null;
  } finally {
    loading.value = false;
  }
}

async function unlockPremium() {
  const token = getAccessToken();
  if (!token) {
    await router.push({ path: "/login", query: { next: route.fullPath } });
    return;
  }
  const id = note.value?.id;
  if (!id || unlocking.value) return;
  const pw = password.value.trim();
  if (!pw) {
    err.value = t("views.federatedNoteDetail.passwordRequired");
    return;
  }
  unlocking.value = true;
  err.value = "";
  try {
    const res = await unlockFederatedNote(token, id, pw);
    note.value = res;
    htmlPremium.value = res.body_premium_md ? renderNoteMarkdown(res.body_premium_md) : "";
    password.value = "";
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : "unlock_failed";
  } finally {
    unlocking.value = false;
  }
}

const titleLabel = computed(() => note.value?.title?.trim() || t("views.noteDetail.untitled"));
const authorLabel = computed(() => note.value?.actor_name?.trim() || note.value?.actor_acct?.trim() || t("routes.remoteProfile"));

onMounted(() => void load());
watch(
  () => route.params.incomingId,
  () => void load(),
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
        <h1 class="truncate text-lg font-bold">{{ titleLabel }}</h1>
        <p class="truncate text-sm text-neutral-500">
          {{ t("views.federatedNoteDetail.subtitle", { author: authorLabel }) }}
        </p>
      </div>
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
        <p class="truncate text-sm font-semibold text-neutral-900">{{ titleLabel }}</p>
      </div>
    </div>
  </Teleport>

  <div class="mx-auto w-full max-w-3xl px-4 py-6">
    <p v-if="err" class="mb-4 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
      {{ err }}
    </p>
    <p v-if="loading" class="py-10 text-center text-sm text-neutral-500">{{ t("views.federatedNoteDetail.loading") }}</p>
    <div v-else-if="note" class="space-y-6">
      <div class="rounded-2xl border border-neutral-200 bg-white p-5">
        <p class="text-sm text-neutral-500">
          {{ t("views.federatedNoteDetail.updatedPrefix") }} {{ formatUpdatedAt(note.updated_at) }}
        </p>
        <h2 class="mt-1 text-xl font-bold text-neutral-900">{{ titleLabel }}</h2>
        <div class="mt-4 prose prose-sm max-w-none" v-html="htmlFree" />
      </div>

      <div v-if="note.has_premium" class="rounded-2xl border border-neutral-200 bg-white p-5">
        <div class="flex items-center justify-between gap-3">
          <h3 class="text-base font-semibold text-neutral-900">{{ t("views.federatedNoteDetail.premiumHeading") }}</h3>
        </div>
        <p v-if="note.premium_locked" class="mt-2 text-sm text-neutral-600">
          {{ t("views.federatedNoteDetail.premiumLockedHint") }}
        </p>
        <div v-if="note.premium_locked" class="mt-4 flex flex-col gap-3 sm:flex-row sm:items-center">
          <input
            v-model="password"
            type="password"
            class="w-full flex-1 rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-400 focus:ring-2"
            :placeholder="t('views.federatedNoteDetail.passwordPlaceholder')"
          />
          <button
            type="button"
            class="w-full shrink-0 rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50 sm:w-auto"
            :disabled="unlocking || !viewerAuthed"
            @click="unlockPremium"
          >
            {{
              unlocking
                ? t("views.federatedNoteDetail.unlockBusy")
                : viewerAuthed
                  ? t("views.federatedNoteDetail.unlock")
                  : t("views.federatedNoteDetail.unlockLogin")
            }}
          </button>
        </div>
        <div v-else class="mt-4 prose prose-sm max-w-none" v-html="htmlPremium" />
      </div>
    </div>
    <p v-else class="py-10 text-center text-sm text-neutral-500">{{ t("views.noteDetail.notFound") }}</p>
  </div>
</template>

