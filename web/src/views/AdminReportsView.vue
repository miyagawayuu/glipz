<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRouter } from "vue-router";
import { getAccessToken } from "../auth";
import { formatDateTime } from "../i18n";
import { api } from "../lib/api";

const { t } = useI18n();
const router = useRouter();
const loading = ref(true);
const err = ref("");
const isAdmin = ref(false);

type LocalReportRow = {
  id: string;
  created_at: string;
  post_id: string;
  post_caption: string;
  category: string;
  reason: string;
  status: "open" | "resolved" | "dismissed" | "spam";
  resolved_at?: string;
  post_author_user_id: string;
  post_author_handle: string;
  post_author_display_name: string;
  reporter_user_id: string;
  reporter_handle: string;
  reporter_display_name: string;
};

type FederatedReportRow = {
  id: string;
  created_at: string;
  incoming_post_id: string;
  object_iri: string;
  caption_text: string;
  category: string;
  reason: string;
  status: "open" | "resolved" | "dismissed" | "spam";
  resolved_at?: string;
  actor_acct: string;
  actor_name: string;
  reporter_user_id: string;
  reporter_handle: string;
  reporter_display_name: string;
};

type DMReportRow = {
  id: string;
  created_at: string;
  thread_id: string;
  message_id: string;
  category: string;
  reason: string;
  include_plaintext: boolean;
  reporter_submitted_plaintext: string;
  attachments_note: string;
  status: "open" | "resolved" | "dismissed" | "spam";
  resolved_at?: string;
  message_sender_handle: string;
  message_sender_display_name: string;
  reporter_handle: string;
  reporter_display_name: string;
};

const localReports = ref<LocalReportRow[]>([]);
const federatedReports = ref<FederatedReportRow[]>([]);
const dmReports = ref<DMReportRow[]>([]);
const updatingKey = ref("");
const reportStatusOptions = computed(() =>
  [
    { value: "open" as const, label: t("views.adminReports.statusOpen") },
    { value: "resolved" as const, label: t("views.adminReports.statusResolved") },
    { value: "dismissed" as const, label: t("views.adminReports.statusDismissed") },
    { value: "spam" as const, label: t("views.adminReports.statusSpam") },
  ] satisfies Array<{ value: LocalReportRow["status"]; label: string }>,
);

function formatDate(iso: string): string {
  return formatDateTime(iso, { dateStyle: "short", timeStyle: "short" }) || iso;
}

function excerpt(text: string, max = 180): string {
  const trimmed = (text ?? "").trim();
  if (!trimmed) return t("views.adminReports.noCaption");
  return trimmed.length > max ? `${trimmed.slice(0, max)}…` : trimmed;
}

function reportStatusLabel(status: string): string {
  if (status === "resolved") return t("views.adminReports.statusResolved");
  if (status === "dismissed") return t("views.adminReports.statusDismissed");
  if (status === "spam") return t("views.adminReports.statusSpam");
  return t("views.adminReports.statusOpen");
}

function reportCategoryLabel(category: string): string {
  switch (category) {
    case "legal":
      return t("views.adminReports.categoryLegal");
    case "safety":
      return t("views.adminReports.categorySafety");
    case "spam":
      return t("views.adminReports.categorySpam");
    case "abuse":
      return t("views.adminReports.categoryAbuse");
    default:
      return t("views.adminReports.categoryOther");
  }
}

function reportCategoryClass(category: string): string {
  if (category === "legal") return "bg-red-50 text-red-700";
  if (category === "safety") return "bg-amber-50 text-amber-800";
  return "bg-neutral-100 text-neutral-600";
}

async function updateLocalReportStatus(row: LocalReportRow, status: LocalReportRow["status"]) {
  const token = getAccessToken();
  if (!token) return;
  updatingKey.value = `local:${row.id}`;
  err.value = "";
  try {
    await api(`/api/v1/admin/reports/posts/${encodeURIComponent(row.id)}`, {
      method: "PATCH",
      token,
      json: { status },
    });
    localReports.value = localReports.value.map((it) =>
      it.id === row.id
        ? { ...it, status, resolved_at: status === "open" ? "" : new Date().toISOString() }
        : it,
    );
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminReports.errors.updateFailed");
  } finally {
    updatingKey.value = "";
  }
}

async function updateFederatedReportStatus(row: FederatedReportRow, status: FederatedReportRow["status"]) {
  const token = getAccessToken();
  if (!token) return;
  updatingKey.value = `fed:${row.id}`;
  err.value = "";
  try {
    await api(`/api/v1/admin/reports/federated-posts/${encodeURIComponent(row.id)}`, {
      method: "PATCH",
      token,
      json: { status },
    });
    federatedReports.value = federatedReports.value.map((it) =>
      it.id === row.id
        ? { ...it, status, resolved_at: status === "open" ? "" : new Date().toISOString() }
        : it,
    );
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminReports.errors.updateFailed");
  } finally {
    updatingKey.value = "";
  }
}

async function updateDMReportStatus(row: DMReportRow, status: DMReportRow["status"]) {
  const token = getAccessToken();
  if (!token) return;
  updatingKey.value = `dm:${row.id}`;
  err.value = "";
  try {
    await api(`/api/v1/admin/reports/dm/${encodeURIComponent(row.id)}`, {
      method: "PATCH",
      token,
      json: { status },
    });
    dmReports.value = dmReports.value.map((it) =>
      it.id === row.id
        ? { ...it, status, resolved_at: status === "open" ? "" : new Date().toISOString() }
        : it,
    );
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminReports.errors.updateFailed");
  } finally {
    updatingKey.value = "";
  }
}

async function loadMe() {
  const token = getAccessToken();
  if (!token) {
    await router.replace("/login");
    return;
  }
  const u = await api<{ is_site_admin?: boolean }>("/api/v1/me", { method: "GET", token });
  isAdmin.value = !!u.is_site_admin;
  if (!isAdmin.value) {
    err.value = t("views.adminReports.errors.adminOnly");
  }
}

async function loadReports() {
  const token = getAccessToken();
  if (!token || !isAdmin.value) return;
  const [localRes, federatedRes, dmRes] = await Promise.all([
    api<{ items: LocalReportRow[] }>("/api/v1/admin/reports/posts", { method: "GET", token }),
    api<{ items: FederatedReportRow[] }>("/api/v1/admin/reports/federated-posts", { method: "GET", token }),
    api<{ items: DMReportRow[] }>("/api/v1/admin/reports/dm", { method: "GET", token }),
  ]);
  localReports.value = localRes.items ?? [];
  federatedReports.value = federatedRes.items ?? [];
  dmReports.value = dmRes.items ?? [];
}

async function refresh() {
  loading.value = true;
  err.value = "";
  try {
    await loadMe();
    if (isAdmin.value) {
      await loadReports();
    }
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminReports.errors.loadFailed");
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  void refresh();
});
</script>

<template>
  <div class="mx-auto max-w-6xl px-4 py-8">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-lime-700">{{ $t("views.adminShell.eyebrow") }}</p>
        <h1 class="mt-2 text-2xl font-bold text-neutral-900">{{ $t("views.adminReports.title") }}</h1>
        <p class="mt-2 text-sm text-neutral-600">{{ $t("views.adminReports.description") }}</p>
      </div>
      <button
        type="button"
        class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700"
        @click="refresh"
      >
        {{ $t("views.adminReports.refresh") }}
      </button>
    </header>

    <p v-if="err" class="mt-5 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">{{ err }}</p>
    <p v-if="loading" class="mt-8 text-sm text-neutral-500">{{ $t("views.adminReports.loading") }}</p>

    <template v-else-if="isAdmin">
      <section class="mt-8">
        <div class="flex items-center justify-between gap-3">
          <h2 class="text-sm font-semibold uppercase tracking-wide text-neutral-500">{{ $t("views.adminReports.localSection") }}</h2>
          <span class="text-xs text-neutral-500">{{ localReports.length }}{{ $t("views.adminReports.countSuffix") }}</span>
        </div>
        <div v-if="localReports.length" class="mt-3 space-y-3">
          <article
            v-for="row in localReports"
            :key="row.id"
            class="rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm"
          >
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div class="min-w-0">
                <p class="text-xs font-medium uppercase tracking-wide text-neutral-500">
                  {{ reportStatusLabel(row.status) }}
                  <span class="ml-2 rounded-full px-2 py-0.5 text-[11px] font-semibold normal-case" :class="reportCategoryClass(row.category)">{{ reportCategoryLabel(row.category) }}</span>
                  <span v-if="row.resolved_at" class="ml-2 normal-case text-neutral-400">{{ $t("views.adminReports.updatedPrefix") }} {{ formatDate(row.resolved_at) }}</span>
                </p>
                <p class="text-sm text-neutral-900">
                  {{ $t("views.adminReports.postAuthor") }}
                  <RouterLink :to="`/@${row.post_author_handle}`" class="font-medium text-lime-700 hover:underline">
                    {{ row.post_author_display_name }}
                  </RouterLink>
                  <span class="ml-1 text-neutral-500">@{{ row.post_author_handle }}</span>
                </p>
                <p class="mt-1 text-sm text-neutral-700">
                  {{ $t("views.adminReports.reporter") }}
                  <RouterLink :to="`/@${row.reporter_handle}`" class="font-medium text-lime-700 hover:underline">
                    {{ row.reporter_display_name }}
                  </RouterLink>
                  <span class="ml-1 text-neutral-500">@{{ row.reporter_handle }}</span>
                </p>
              </div>
              <time class="shrink-0 text-xs text-neutral-500" :datetime="row.created_at">{{ formatDate(row.created_at) }}</time>
            </div>
            <p class="mt-3 whitespace-pre-wrap rounded-xl bg-neutral-50 px-3 py-2 text-sm text-neutral-800">
              {{ excerpt(row.post_caption) }}
            </p>
            <div class="mt-3 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2">
              <p class="text-xs font-semibold text-amber-900">{{ $t("views.adminReports.reasonHeading") }}</p>
              <p class="mt-1 whitespace-pre-wrap text-sm text-amber-950">{{ row.reason }}</p>
            </div>
            <div class="mt-3 flex flex-wrap gap-3 text-sm">
              <RouterLink :to="`/posts/${row.post_id}`" class="font-medium text-lime-700 hover:underline">
                {{ $t("views.adminReports.openPost") }}
              </RouterLink>
              <label class="flex items-center gap-2 text-sm text-neutral-700">
                <span>{{ $t("views.adminReports.statusLabel") }}</span>
                <select
                  class="rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
                  :disabled="updatingKey === `local:${row.id}`"
                  :value="row.status"
                  @change="updateLocalReportStatus(row, ($event.target as HTMLSelectElement).value as LocalReportRow['status'])"
                >
                  <option v-for="opt in reportStatusOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
                </select>
              </label>
              <button
                type="button"
                class="font-medium text-neutral-600 hover:text-neutral-900"
                @click="router.push('/admin/federation')"
              >
                {{ $t("views.adminReports.federationAdminLink") }}
              </button>
            </div>
          </article>
        </div>
        <p v-else class="mt-3 text-sm text-neutral-500">{{ $t("views.adminReports.emptyLocal") }}</p>
      </section>

      <section class="mt-10">
        <div class="flex items-center justify-between gap-3">
          <h2 class="text-sm font-semibold uppercase tracking-wide text-neutral-500">{{ $t("views.adminReports.federatedSection") }}</h2>
          <span class="text-xs text-neutral-500">{{ federatedReports.length }}{{ $t("views.adminReports.countSuffix") }}</span>
        </div>
        <div v-if="federatedReports.length" class="mt-3 space-y-3">
          <article
            v-for="row in federatedReports"
            :key="row.id"
            class="rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm"
          >
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div class="min-w-0">
                <p class="text-xs font-medium uppercase tracking-wide text-neutral-500">
                  {{ reportStatusLabel(row.status) }}
                  <span class="ml-2 rounded-full px-2 py-0.5 text-[11px] font-semibold normal-case" :class="reportCategoryClass(row.category)">{{ reportCategoryLabel(row.category) }}</span>
                  <span v-if="row.resolved_at" class="ml-2 normal-case text-neutral-400">{{ $t("views.adminReports.updatedPrefix") }} {{ formatDate(row.resolved_at) }}</span>
                </p>
                <p class="text-sm text-neutral-900">
                  {{ $t("views.adminReports.sourceActor") }}
                  <span class="font-medium">{{ row.actor_name || row.actor_acct || $t("views.adminReports.federatedActorFallback") }}</span>
                  <span v-if="row.actor_acct" class="ml-1 text-neutral-500">{{ row.actor_acct }}</span>
                </p>
                <p class="mt-1 text-sm text-neutral-700">
                  {{ $t("views.adminReports.reporter") }}
                  <RouterLink :to="`/@${row.reporter_handle}`" class="font-medium text-lime-700 hover:underline">
                    {{ row.reporter_display_name }}
                  </RouterLink>
                  <span class="ml-1 text-neutral-500">@{{ row.reporter_handle }}</span>
                </p>
              </div>
              <time class="shrink-0 text-xs text-neutral-500" :datetime="row.created_at">{{ formatDate(row.created_at) }}</time>
            </div>
            <p class="mt-3 whitespace-pre-wrap rounded-xl bg-neutral-50 px-3 py-2 text-sm text-neutral-800">
              {{ excerpt(row.caption_text) }}
            </p>
            <div class="mt-3 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2">
              <p class="text-xs font-semibold text-amber-900">{{ $t("views.adminReports.reasonHeading") }}</p>
              <p class="mt-1 whitespace-pre-wrap text-sm text-amber-950">{{ row.reason }}</p>
            </div>
            <div class="mt-3 flex flex-wrap gap-3 text-sm">
              <RouterLink :to="`/posts/federated/${row.incoming_post_id}`" class="font-medium text-lime-700 hover:underline">
                {{ $t("views.adminReports.openIncomingPost") }}
              </RouterLink>
              <label class="flex items-center gap-2 text-sm text-neutral-700">
                <span>{{ $t("views.adminReports.statusLabel") }}</span>
                <select
                  class="rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
                  :disabled="updatingKey === `fed:${row.id}`"
                  :value="row.status"
                  @change="updateFederatedReportStatus(row, ($event.target as HTMLSelectElement).value as FederatedReportRow['status'])"
                >
                  <option v-for="opt in reportStatusOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
                </select>
              </label>
              <a
                :href="row.object_iri"
                target="_blank"
                rel="noopener noreferrer"
                class="font-medium text-violet-700 hover:underline"
              >
                {{ $t("views.adminReports.openOriginal") }}
              </a>
            </div>
          </article>
        </div>
        <p v-else class="mt-3 text-sm text-neutral-500">{{ $t("views.adminReports.emptyFederated") }}</p>
      </section>

      <section class="mt-10">
        <div class="flex items-center justify-between gap-3">
          <h2 class="text-sm font-semibold uppercase tracking-wide text-neutral-500">{{ $t("views.adminReports.dmSection") }}</h2>
          <span class="text-xs text-neutral-500">{{ dmReports.length }}{{ $t("views.adminReports.countSuffix") }}</span>
        </div>
        <div v-if="dmReports.length" class="mt-3 space-y-3">
          <article
            v-for="row in dmReports"
            :key="row.id"
            class="rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm"
          >
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div class="min-w-0">
                <p class="text-xs font-medium uppercase tracking-wide text-neutral-500">
                  {{ reportStatusLabel(row.status) }}
                  <span class="ml-2 rounded-full px-2 py-0.5 text-[11px] font-semibold normal-case" :class="reportCategoryClass(row.category)">{{ reportCategoryLabel(row.category) }}</span>
                  <span v-if="row.resolved_at" class="ml-2 normal-case text-neutral-400">{{ $t("views.adminReports.updatedPrefix") }} {{ formatDate(row.resolved_at) }}</span>
                </p>
                <p class="text-sm text-neutral-900">
                  {{ $t("views.adminReports.dmSender") }}
                  <span class="font-medium">{{ row.message_sender_display_name }}</span>
                  <span class="ml-1 text-neutral-500">@{{ row.message_sender_handle }}</span>
                </p>
                <p class="mt-1 text-sm text-neutral-700">
                  {{ $t("views.adminReports.reporter") }}
                  <RouterLink :to="`/@${row.reporter_handle}`" class="font-medium text-lime-700 hover:underline">
                    {{ row.reporter_display_name }}
                  </RouterLink>
                  <span class="ml-1 text-neutral-500">@{{ row.reporter_handle }}</span>
                </p>
              </div>
              <time class="shrink-0 text-xs text-neutral-500" :datetime="row.created_at">{{ formatDate(row.created_at) }}</time>
            </div>
            <div class="mt-3 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2">
              <p class="text-xs font-semibold text-amber-900">{{ $t("views.adminReports.reasonHeading") }}</p>
              <p class="mt-1 whitespace-pre-wrap text-sm text-amber-950">{{ row.reason }}</p>
            </div>
            <details v-if="row.include_plaintext && row.reporter_submitted_plaintext" class="mt-3 rounded-xl border border-red-200 bg-red-50 px-3 py-2">
              <summary class="cursor-pointer text-xs font-semibold text-red-900">{{ $t("views.adminReports.dmPlaintextSummary") }}</summary>
              <p class="mt-2 whitespace-pre-wrap text-sm text-red-950">{{ row.reporter_submitted_plaintext }}</p>
            </details>
            <p v-if="row.attachments_note" class="mt-3 whitespace-pre-wrap rounded-xl bg-neutral-50 px-3 py-2 text-sm text-neutral-800">
              {{ row.attachments_note }}
            </p>
            <div class="mt-3 flex flex-wrap gap-3 text-sm">
              <RouterLink :to="`/messages/${row.thread_id}`" class="font-medium text-lime-700 hover:underline">
                {{ $t("views.adminReports.openDMThread") }}
              </RouterLink>
              <label class="flex items-center gap-2 text-sm text-neutral-700">
                <span>{{ $t("views.adminReports.statusLabel") }}</span>
                <select
                  class="rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
                  :disabled="updatingKey === `dm:${row.id}`"
                  :value="row.status"
                  @change="updateDMReportStatus(row, ($event.target as HTMLSelectElement).value as DMReportRow['status'])"
                >
                  <option v-for="opt in reportStatusOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
                </select>
              </label>
            </div>
          </article>
        </div>
        <p v-else class="mt-3 text-sm text-neutral-500">{{ $t("views.adminReports.emptyDM") }}</p>
      </section>
    </template>
  </div>
</template>
