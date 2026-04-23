<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";
import Icon from "../components/Icon.vue";
import NoteRichEditor from "../components/NoteRichEditor.vue";
import { uploadNoteMedia } from "../lib/noteMediaUpload";

type NotePayload = {
  id: string;
  title: string;
  body_md: string;
  body_premium_md: string;
  editor_mode: string;
  status: string;
  visibility: string;
  is_owner: boolean;
  patreon_campaign_id?: string | null;
  patreon_required_reward_tier_id?: string | null;
};

type PatreonTier = { id: string; title: string; amount_cents: number };
type PatreonCampaign = { id: string; name: string };

const route = useRoute();
const router = useRouter();
const { t } = useI18n();

const isNew = computed(() => route.name === "note-new" || route.path === "/notes/new");

const noteId = computed(() => {
  if (isNew.value) return "";
  const p = route.params.noteId;
  return typeof p === "string" ? p : Array.isArray(p) ? p[0] ?? "" : "";
});

const title = ref("");
const bodyMd = ref("");
const bodyPremiumMd = ref("");
const status = ref<"draft" | "published">("draft");
const visibility = ref<"public" | "followers" | "private">("public");
/** Empty string means "use the default campaign from the security settings page". */
const patreonCampaignId = ref("");
const patreonCampaigns = ref<PatreonCampaign[]>([]);
const patreonCampaignsErr = ref("");
/** Empty string means "use the default required tier from the security settings page". */
const patreonRequiredRewardTierId = ref("");
const patreonTiers = ref<PatreonTier[]>([]);
const patreonTiersErr = ref("");
/** Selected editing surface: raw Markdown source or rich-text mode. */
const surfaceEditor = ref<"markdown" | "richtext">("markdown");

const err = ref("");
const busy = ref(false);
const loading = ref(!isNew.value);
const token = ref("");

const mdImageInput = ref<HTMLInputElement | null>(null);
const mdVideoInput = ref<HTMLInputElement | null>(null);
const mdPremImageInput = ref<HTMLInputElement | null>(null);
const mdPremVideoInput = ref<HTMLInputElement | null>(null);

function editorModeStr() {
  return surfaceEditor.value === "richtext" ? "richtext" : "markdown";
}

function payloadJson(st: "draft" | "published") {
  return {
    title: title.value,
    body_md: bodyMd.value,
    body_premium_md: bodyPremiumMd.value,
    editor_mode: editorModeStr(),
    status: st,
    visibility: visibility.value,
    patreon_campaign_id: patreonCampaignId.value.trim(),
    patreon_required_reward_tier_id: patreonRequiredRewardTierId.value.trim(),
  };
}

async function loadPatreonCampaigns() {
  patreonCampaignsErr.value = "";
  const t = getAccessToken();
  if (!t) {
    patreonCampaigns.value = [];
    return;
  }
  try {
    const res = await api<{ campaigns: PatreonCampaign[] }>("/api/v1/patreon/creator/campaigns", {
      method: "GET",
      token: t,
    });
    patreonCampaigns.value = res.campaigns ?? [];
  } catch (e: unknown) {
    patreonCampaigns.value = [];
    patreonCampaignsErr.value =
      e instanceof Error ? e.message : t("views.noteEdit.errors.patreonCampaignsLoadFailed");
  }
}

async function loadPatreonTiers() {
  patreonTiersErr.value = "";
  const t = getAccessToken();
  if (!t) {
    patreonTiers.value = [];
    return;
  }
  const cid = patreonCampaignId.value.trim();
  const qs = cid ? `?campaign_id=${encodeURIComponent(cid)}` : "";
  try {
    const res = await api<{ tiers: PatreonTier[] }>(`/api/v1/patreon/creator/tiers${qs}`, {
      method: "GET",
      token: t,
    });
    patreonTiers.value = res.tiers ?? [];
  } catch (e: unknown) {
    patreonTiers.value = [];
    patreonTiersErr.value =
      e instanceof Error ? e.message : t("views.noteEdit.errors.patreonTiersLoadFailed");
  }
}

function onPatreonCampaignChange() {
  void loadPatreonTiers();
}

async function loadNote() {
  if (isNew.value) {
    loading.value = false;
    patreonCampaignId.value = "";
    patreonRequiredRewardTierId.value = "";
    await loadPatreonCampaigns();
    await loadPatreonTiers();
    return;
  }
  const t = getAccessToken();
  if (!t) {
    await router.replace("/login");
    return;
  }
  loading.value = true;
  err.value = "";
  try {
    const res = await api<{ note: NotePayload }>(`/api/v1/notes/${encodeURIComponent(noteId.value)}`, {
      method: "GET",
      token: t,
    });
    if (!res.note.is_owner) {
      err.value = t("views.noteEdit.errors.editForbidden");
      return;
    }
    title.value = res.note.title;
    bodyMd.value = res.note.body_md;
    bodyPremiumMd.value = res.note.body_premium_md ?? "";
    status.value = res.note.status === "draft" ? "draft" : "published";
    const v = res.note.visibility;
    visibility.value = v === "followers" || v === "private" ? v : "public";
    const em = res.note.editor_mode === "richtext" ? "richtext" : "markdown";
    surfaceEditor.value = em;
    const cid = res.note.patreon_campaign_id;
    patreonCampaignId.value = typeof cid === "string" && cid.trim() ? cid.trim() : "";
    const tid = res.note.patreon_required_reward_tier_id;
    patreonRequiredRewardTierId.value = typeof tid === "string" && tid.trim() ? tid.trim() : "";
    await loadPatreonCampaigns();
    await loadPatreonTiers();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.noteEdit.errors.loadFailed");
  } finally {
    loading.value = false;
  }
}

async function saveDraft() {
  const t = getAccessToken();
  if (!t) return;
  busy.value = true;
  err.value = "";
  try {
    if (isNew.value) {
      const res = await api<{ note: NotePayload }>("/api/v1/notes", {
        method: "POST",
        token: t,
        json: payloadJson("draft"),
      });
      await router.replace(`/notes/${res.note.id}/edit`);
      return;
    }
    await api(`/api/v1/notes/${encodeURIComponent(noteId.value)}`, {
      method: "PATCH",
      token: t,
      json: payloadJson("draft"),
    });
    status.value = "draft";
    await router.push(`/notes/${noteId.value}`);
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.noteEdit.errors.saveFailed");
  } finally {
    busy.value = false;
  }
}

async function publishNote() {
  const t = getAccessToken();
  if (!t) return;
  busy.value = true;
  err.value = "";
  try {
    if (isNew.value) {
      const res = await api<{ note: NotePayload }>("/api/v1/notes", {
        method: "POST",
        token: t,
        json: payloadJson("published"),
      });
      await router.replace(`/notes/${res.note.id}`);
      return;
    }
    await api(`/api/v1/notes/${encodeURIComponent(noteId.value)}`, {
      method: "PATCH",
      token: t,
      json: payloadJson("published"),
    });
    status.value = "published";
    await router.push(`/notes/${noteId.value}`);
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.noteEdit.errors.publishFailed");
  } finally {
    busy.value = false;
  }
}

async function remove() {
  if (isNew.value) return;
  if (!window.confirm(t("views.noteEdit.confirmDelete"))) return;
  const t = getAccessToken();
  if (!t) return;
  busy.value = true;
  err.value = "";
  try {
    await api(`/api/v1/notes/${encodeURIComponent(noteId.value)}`, { method: "DELETE", token: t });
    await router.push("/feed");
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.noteEdit.errors.deleteFailed");
  } finally {
    busy.value = false;
  }
}

async function onMdImage(ev: Event) {
  const inp = ev.target as HTMLInputElement;
  const f = inp.files?.[0];
  inp.value = "";
  if (!f || !token.value) return;
  try {
    const url = await uploadNoteMedia(f, token.value);
    bodyMd.value += `\n\n![](${url})\n`;
  } catch (e: unknown) {
    window.alert(e instanceof Error ? e.message : t("errors.uploadFailed"));
  }
}

async function onMdVideo(ev: Event) {
  const inp = ev.target as HTMLInputElement;
  const f = inp.files?.[0];
  inp.value = "";
  if (!f || !token.value) return;
  try {
    const url = await uploadNoteMedia(f, token.value);
    bodyMd.value += `\n\n<video src="${url}" controls playsinline></video>\n`;
  } catch (e: unknown) {
    window.alert(e instanceof Error ? e.message : t("errors.uploadFailed"));
  }
}

async function onMdPremImage(ev: Event) {
  const inp = ev.target as HTMLInputElement;
  const f = inp.files?.[0];
  inp.value = "";
  if (!f || !token.value) return;
  try {
    const url = await uploadNoteMedia(f, token.value);
    bodyPremiumMd.value += `\n\n![](${url})\n`;
  } catch (e: unknown) {
    window.alert(e instanceof Error ? e.message : t("errors.uploadFailed"));
  }
}

async function onMdPremVideo(ev: Event) {
  const inp = ev.target as HTMLInputElement;
  const f = inp.files?.[0];
  inp.value = "";
  if (!f || !token.value) return;
  try {
    const url = await uploadNoteMedia(f, token.value);
    bodyPremiumMd.value += `\n\n<video src="${url}" controls playsinline></video>\n`;
  } catch (e: unknown) {
    window.alert(e instanceof Error ? e.message : t("errors.uploadFailed"));
  }
}

watch(
  () => route.fullPath,
  async () => {
    token.value = getAccessToken() ?? "";
    if (route.name === "note-new") {
      title.value = "";
      bodyMd.value = "";
      bodyPremiumMd.value = "";
      status.value = "draft";
      visibility.value = "public";
      surfaceEditor.value = "markdown";
      patreonCampaignId.value = "";
      patreonRequiredRewardTierId.value = "";
      err.value = "";
      loading.value = false;
      void loadPatreonCampaigns();
      void loadPatreonTiers();
      return;
    }
    if (route.name === "note-edit") {
      await loadNote();
    }
  },
  { immediate: true },
);
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="flex h-12 items-center gap-2">
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="$t('common.back.previous')"
        @click="router.back()"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <h1 class="min-w-0 flex-1 truncate text-lg font-bold">
        {{ isNew ? $t("views.noteEdit.titleNew") : $t("views.noteEdit.titleEdit") }}
      </h1>
      <button
        type="button"
        class="shrink-0 rounded-full border border-neutral-200 px-3 py-1.5 text-sm font-semibold text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
        :disabled="busy"
        @click="saveDraft"
      >
        {{ busy ? $t("views.noteEdit.saveDraftBusy") : $t("views.noteEdit.saveDraft") }}
      </button>
      <button
        type="button"
        class="shrink-0 rounded-full bg-lime-600 px-4 py-1.5 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
        :disabled="busy"
        @click="publishNote"
      >
        {{ busy ? $t("views.noteEdit.publishBusy") : $t("views.noteEdit.publish") }}
      </button>
      <button
        v-if="!isNew"
        type="button"
        class="shrink-0 rounded-full border border-red-200 px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 disabled:opacity-50"
        :disabled="busy"
        @click="remove"
      >
        {{ $t("views.noteEdit.remove") }}
      </button>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="flex flex-wrap items-center gap-2 px-4 py-3">
      <button
        type="button"
        class="rounded-full p-2 text-neutral-600 hover:bg-neutral-100"
        :aria-label="$t('common.back.previous')"
        @click="router.back()"
      >
        <Icon name="back" class="h-5 w-5" />
      </button>
      <h1 class="min-w-0 flex-1 truncate text-lg font-bold">
        {{ isNew ? $t("views.noteEdit.titleNew") : $t("views.noteEdit.titleEdit") }}
      </h1>
      <button
        type="button"
        class="shrink-0 rounded-full border border-neutral-200 px-3 py-1.5 text-sm font-semibold text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
        :disabled="busy"
        @click="saveDraft"
      >
        {{ busy ? $t("views.noteEdit.saveDraftBusy") : $t("views.noteEdit.saveDraft") }}
      </button>
      <button
        type="button"
        class="shrink-0 rounded-full bg-lime-600 px-4 py-1.5 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
        :disabled="busy"
        @click="publishNote"
      >
        {{ busy ? $t("views.noteEdit.publishBusy") : $t("views.noteEdit.publish") }}
      </button>
      <button
        v-if="!isNew"
        type="button"
        class="shrink-0 rounded-full border border-red-200 px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 disabled:opacity-50"
        :disabled="busy"
        @click="remove"
      >
        {{ $t("views.noteEdit.remove") }}
      </button>
    </div>
  </Teleport>
  <div class="min-h-0 w-full min-w-0 text-neutral-900">
    <p v-if="err" class="border-b border-neutral-200 px-4 py-2 text-sm text-red-600">{{ err }}</p>
    <p v-if="loading" class="px-4 py-12 text-center text-sm text-neutral-500">{{ $t("views.noteEdit.loading") }}</p>
    <div v-else class="space-y-4 px-4 py-6">
      <div class="flex flex-wrap items-center gap-3 text-xs text-neutral-600">
        <span
          class="rounded-full px-2 py-0.5 font-medium"
          :class="status === 'published' ? 'bg-lime-100 text-lime-900' : 'bg-amber-100 text-amber-900'"
        >
          {{ status === "published" ? $t("views.noteEdit.statusPublished") : $t("views.noteEdit.statusDraft") }}
        </span>
      </div>

      <div>
        <label class="mb-1 block text-xs font-medium text-neutral-600" for="note-visibility">{{ $t("views.noteEdit.visibilityLabel") }}</label>
        <select
          id="note-visibility"
          v-model="visibility"
          class="w-full max-w-md rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        >
          <option value="public">{{ $t("views.noteEdit.visibilityPublic") }}</option>
          <option value="followers">{{ $t("views.noteEdit.visibilityFollowers") }}</option>
          <option value="private">{{ $t("views.noteEdit.visibilityPrivate") }}</option>
        </select>
      </div>

      <div>
        <label class="mb-1 block text-xs font-medium text-neutral-600" for="note-title">{{ $t("views.noteEdit.titleLabel") }}</label>
        <input
          id="note-title"
          v-model="title"
          type="text"
          maxlength="500"
          :placeholder="$t('views.noteEdit.titlePlaceholder')"
          class="w-full rounded-xl border border-neutral-200 px-3 py-2 text-lg font-semibold text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        />
      </div>

      <div>
        <p class="mb-2 text-xs font-medium text-neutral-600">{{ $t("views.noteEdit.editorMode") }}</p>
        <div class="inline-flex rounded-full border border-neutral-200 bg-neutral-50 p-0.5">
          <button
            type="button"
            class="rounded-full px-4 py-1.5 text-sm font-semibold transition-colors"
            :class="surfaceEditor === 'markdown' ? 'bg-white text-neutral-900 shadow-sm' : 'text-neutral-600 hover:text-neutral-900'"
            @click="surfaceEditor = 'markdown'"
          >
            {{ $t("views.noteEdit.editorMarkdown") }}
          </button>
          <button
            type="button"
            class="rounded-full px-4 py-1.5 text-sm font-semibold transition-colors"
            :class="surfaceEditor === 'richtext' ? 'bg-white text-neutral-900 shadow-sm' : 'text-neutral-600 hover:text-neutral-900'"
            @click="surfaceEditor = 'richtext'"
          >
            {{ $t("views.noteEdit.editorRichtext") }}
          </button>
        </div>
      </div>

      <div v-if="surfaceEditor === 'markdown'">
        <div class="mb-2 flex flex-wrap gap-2">
          <button
            type="button"
            class="rounded-lg border border-neutral-200 bg-white px-3 py-1.5 text-xs font-medium text-lime-800 hover:bg-lime-50"
            @click="mdImageInput?.click()"
          >
            {{ $t("views.noteEdit.insertImage") }}
          </button>
          <button
            type="button"
            class="rounded-lg border border-neutral-200 bg-white px-3 py-1.5 text-xs font-medium text-lime-800 hover:bg-lime-50"
            @click="mdVideoInput?.click()"
          >
            {{ $t("views.noteEdit.insertVideo") }}
          </button>
          <input ref="mdImageInput" type="file" accept="image/*" class="hidden" @change="onMdImage" />
          <input ref="mdVideoInput" type="file" accept="video/*" class="hidden" @change="onMdVideo" />
        </div>
        <label class="mb-1 block text-xs font-medium text-neutral-600" for="note-md">{{ $t("views.noteEdit.bodyLabel") }}</label>
        <textarea
          id="note-md"
          v-model="bodyMd"
          rows="14"
          class="w-full rounded-xl border border-neutral-200 px-3 py-2 font-mono text-sm leading-relaxed text-neutral-900 outline-none ring-lime-500 focus:ring-2"
          :placeholder="$t('views.noteEdit.bodyPlaceholder')"
        />
      </div>
      <div v-else>
        <p v-if="!token" class="mb-2 text-sm text-red-600">{{ $t("views.noteEdit.loginRequired") }}</p>
        <NoteRichEditor v-else :markdown="bodyMd" :upload-token="token" @update:markdown="(v) => (bodyMd = v)" />
      </div>

      <div class="border-t border-dashed border-lime-300 pt-6 dark:border-lime-700/50">
        <div class="mb-4 flex items-center gap-3">
          <div class="h-px flex-1 bg-lime-200 dark:bg-lime-700/50" />
          <p class="text-sm font-semibold text-lime-800 dark:text-lime-300">{{ $t("views.noteEdit.premiumSeparator") }}</p>
          <div class="h-px flex-1 bg-lime-200 dark:bg-lime-700/50" />
        </div>
        <p class="mb-1 text-sm font-semibold text-lime-800 dark:text-lime-300">{{ $t("views.noteEdit.premiumTitle") }}</p>
        <p class="mb-4 text-xs text-lime-700/90 dark:text-lime-200/90">
          {{ $t("views.noteEdit.premiumDescription") }}
        </p>
        <div class="mb-3">
          <label class="mb-1 block text-xs font-medium text-lime-800 dark:text-lime-300" for="note-patreon-campaign">{{ $t("views.noteEdit.patreonCampaignLabel") }}</label>
          <select
            id="note-patreon-campaign"
            v-model="patreonCampaignId"
            class="mb-2 w-full max-w-md rounded-xl border border-lime-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-400 focus:ring-2 dark:border-lime-700/50 dark:bg-neutral-950 dark:text-neutral-100"
            @change="onPatreonCampaignChange"
          >
            <option value="">{{ $t("views.noteEdit.patreonCampaignDefault") }}</option>
            <option
              v-if="
                patreonCampaignId &&
                !patreonCampaigns.some((x) => x.id === patreonCampaignId)
              "
              :value="patreonCampaignId"
            >
              {{ $t("views.noteEdit.patreonCurrentId", { id: patreonCampaignId }) }}
            </option>
            <option v-for="c in patreonCampaigns" :key="c.id" :value="c.id">
              {{ c.name || c.id }}
            </option>
          </select>
          <p v-if="patreonCampaignsErr" class="mb-2 text-xs text-lime-700/90 dark:text-lime-200/90">{{ patreonCampaignsErr }}</p>
          <label class="mb-1 block text-xs font-medium text-lime-800 dark:text-lime-300" for="note-patreon-tier">{{ $t("views.noteEdit.patreonTierLabel") }}</label>
          <select
            id="note-patreon-tier"
            v-model="patreonRequiredRewardTierId"
            class="w-full max-w-md rounded-xl border border-lime-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-400 focus:ring-2 dark:border-lime-700/50 dark:bg-neutral-950 dark:text-neutral-100"
          >
            <option value="">{{ $t("views.noteEdit.patreonTierDefault") }}</option>
            <option
              v-if="
                patreonRequiredRewardTierId &&
                !patreonTiers.some((x) => x.id === patreonRequiredRewardTierId)
              "
              :value="patreonRequiredRewardTierId"
            >
              {{ $t("views.noteEdit.patreonCurrentId", { id: patreonRequiredRewardTierId }) }}
            </option>
            <option v-for="t in patreonTiers" :key="t.id" :value="t.id">
              {{ t.title || $t("views.noteEdit.untitledTier") }}{{ t.amount_cents > 0 ? ` - ${(t.amount_cents / 100).toFixed(0)}` : "" }}
            </option>
          </select>
          <p v-if="patreonTiersErr" class="mt-1 text-xs text-lime-700/90 dark:text-lime-200/90">{{ patreonTiersErr }}</p>
        </div>
        <div v-if="surfaceEditor === 'markdown'">
          <div class="mb-2 flex flex-wrap gap-2">
            <button
              type="button"
              class="rounded-lg border border-lime-300 bg-white px-3 py-1.5 text-xs font-medium text-lime-800 hover:bg-lime-50 dark:border-lime-700/50 dark:bg-neutral-950 dark:text-lime-300 dark:hover:bg-lime-950/40"
              @click="mdPremImageInput?.click()"
            >
              {{ $t("views.noteEdit.premiumImage") }}
            </button>
            <button
              type="button"
              class="rounded-lg border border-lime-300 bg-white px-3 py-1.5 text-xs font-medium text-lime-800 hover:bg-lime-50 dark:border-lime-700/50 dark:bg-neutral-950 dark:text-lime-300 dark:hover:bg-lime-950/40"
              @click="mdPremVideoInput?.click()"
            >
              {{ $t("views.noteEdit.premiumVideo") }}
            </button>
            <input ref="mdPremImageInput" type="file" accept="image/*" class="hidden" @change="onMdPremImage" />
            <input ref="mdPremVideoInput" type="file" accept="video/*" class="hidden" @change="onMdPremVideo" />
          </div>
          <label class="mb-1 block text-xs font-medium text-lime-800 dark:text-lime-300" for="note-md-prem">{{ $t("views.noteEdit.premiumBodyLabel") }}</label>
          <textarea
            id="note-md-prem"
            v-model="bodyPremiumMd"
            rows="10"
            class="w-full rounded-xl border border-lime-200 bg-white px-3 py-2 font-mono text-sm leading-relaxed text-neutral-900 outline-none ring-lime-400 focus:ring-2 dark:border-lime-700/50 dark:bg-neutral-950 dark:text-neutral-100"
            :placeholder="$t('views.noteEdit.premiumBodyPlaceholder')"
          />
        </div>
        <div v-else>
          <p v-if="!token" class="mb-2 text-sm text-red-600">{{ $t("views.noteEdit.loginRequired") }}</p>
          <NoteRichEditor v-else :markdown="bodyPremiumMd" :upload-token="token" @update:markdown="(v) => (bodyPremiumMd = v)" />
        </div>
      </div>
    </div>
  </div>
</template>
