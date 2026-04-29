<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { getAccessToken } from "../auth";
import { formatDateTime } from "../i18n";
import { api, apiBase, applyBrowserAuth } from "../lib/api";

const { t } = useI18n();

type LegalRequestRow = {
  id: string;
  request_type: string;
  agency_name: string;
  jurisdiction: string;
  legal_basis: string;
  external_reference: string;
  target_user_id?: string;
  target_handle: string;
  target_from_at?: string;
  target_until_at?: string;
  data_types: string[];
  emergency: boolean;
  status: "incoming" | "reviewing" | "preserved" | "responded" | "rejected" | "closed";
  due_at?: string;
  response_summary: string;
  user_notice_status: "not_applicable" | "pending" | "sent" | "delayed" | "prohibited";
  created_at: string;
  updated_at: string;
};

const loading = ref(true);
const saving = ref(false);
const err = ref("");
const items = ref<LegalRequestRow[]>([]);
const form = ref({
  request_type: "legal_process",
  agency_name: "",
  jurisdiction: "",
  legal_basis: "",
  external_reference: "",
  target_user_id: "",
  target_handle: "",
  target_from_at: "",
  target_until_at: "",
  data_types: "account,dm_metadata,encrypted_dm_payloads,dm_reports,access_events,audit_events",
  emergency: false,
  due_at: "",
  user_notice_status: "not_applicable",
});
const holdForm = ref({
  request_id: "",
  target_user_id: "",
  resource_type: "account",
  resource_id: "",
  expires_at: "",
  reason: "",
});

const statusOptions = computed(() => [
  "incoming",
  "reviewing",
  "preserved",
  "responded",
  "rejected",
  "closed",
] as const);

function formatDate(iso?: string): string {
  return iso ? formatDateTime(iso, { dateStyle: "short", timeStyle: "short" }) || iso : "";
}

function localInputToISO(value: string): string {
  if (!value) return "";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? "" : d.toISOString();
}

async function load() {
  loading.value = true;
  err.value = "";
  try {
    const res = await api<{ items: LegalRequestRow[] }>("/api/v1/admin/legal-requests", { method: "GET" });
    items.value = res.items ?? [];
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminLegalRequests.errors.loadFailed");
  } finally {
    loading.value = false;
  }
}

async function createRequest() {
  saving.value = true;
  err.value = "";
  try {
    const payload = {
      ...form.value,
      target_from_at: localInputToISO(form.value.target_from_at),
      target_until_at: localInputToISO(form.value.target_until_at),
      due_at: localInputToISO(form.value.due_at),
      data_types: form.value.data_types.split(",").map((it) => it.trim()).filter(Boolean),
    };
    await api("/api/v1/admin/legal-requests", { method: "POST", json: payload });
    form.value.agency_name = "";
    form.value.jurisdiction = "";
    form.value.legal_basis = "";
    form.value.external_reference = "";
    form.value.target_user_id = "";
    form.value.target_handle = "";
    await load();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminLegalRequests.errors.saveFailed");
  } finally {
    saving.value = false;
  }
}

async function updateStatus(row: LegalRequestRow, status: LegalRequestRow["status"]) {
  try {
    await api(`/api/v1/admin/legal-requests/${encodeURIComponent(row.id)}`, {
      method: "PATCH",
      json: {
        status,
        response_summary: row.response_summary || "",
        user_notice_status: row.user_notice_status || "not_applicable",
      },
    });
    await load();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminLegalRequests.errors.saveFailed");
  }
}

async function exportRequest(row: LegalRequestRow) {
  const headers = new Headers();
  const token = getAccessToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const res = await fetch(`${apiBase()}/api/v1/admin/legal-requests/${encodeURIComponent(row.id)}/export`, applyBrowserAuth({ method: "GET", headers }));
  if (!res.ok) {
    const data = (await res.json().catch(() => ({}))) as { error?: string };
    throw new Error(data.error || res.statusText);
  }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `glipz-legal-request-${row.id}.json`;
  a.click();
  URL.revokeObjectURL(url);
}

async function createHold(row: LegalRequestRow) {
  err.value = "";
  try {
    await api(`/api/v1/admin/legal-requests/${encodeURIComponent(row.id)}/holds`, {
      method: "POST",
      json: {
        target_user_id: holdForm.value.target_user_id || row.target_user_id || "",
        resource_type: holdForm.value.resource_type,
        resource_id: holdForm.value.resource_id,
        expires_at: localInputToISO(holdForm.value.expires_at),
        reason: holdForm.value.reason,
      },
    });
    holdForm.value.request_id = "";
    holdForm.value.target_user_id = "";
    holdForm.value.resource_id = "";
    holdForm.value.expires_at = "";
    holdForm.value.reason = "";
    await updateStatus(row, "preserved");
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminLegalRequests.errors.saveFailed");
  }
}

onMounted(() => {
  void load();
});
</script>

<template>
  <div class="mx-auto max-w-6xl px-4 py-8">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-lime-700">{{ $t("views.adminShell.eyebrow") }}</p>
        <h1 class="mt-2 text-2xl font-bold text-neutral-900">{{ $t("views.adminLegalRequests.title") }}</h1>
        <p class="mt-2 max-w-3xl text-sm leading-6 text-neutral-600">{{ $t("views.adminLegalRequests.description") }}</p>
      </div>
      <button type="button" class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700" @click="load">
        {{ $t("views.adminLegalRequests.refresh") }}
      </button>
    </header>

    <p v-if="err" class="mt-5 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">{{ err }}</p>

    <form class="mt-6 grid gap-4 rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm md:grid-cols-2" @submit.prevent="createRequest">
      <label class="text-sm text-neutral-700">
        <span class="font-medium">{{ $t("views.adminLegalRequests.agencyName") }}</span>
        <input v-model="form.agency_name" required maxlength="200" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
      </label>
      <label class="text-sm text-neutral-700">
        <span class="font-medium">{{ $t("views.adminLegalRequests.requestType") }}</span>
        <select v-model="form.request_type" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40">
          <option value="legal_process">legal_process</option>
          <option value="preservation">preservation</option>
          <option value="emergency">emergency</option>
          <option value="user_notice">user_notice</option>
          <option value="other">other</option>
        </select>
      </label>
      <label class="text-sm text-neutral-700">
        <span class="font-medium">{{ $t("views.adminLegalRequests.jurisdiction") }}</span>
        <input v-model="form.jurisdiction" maxlength="200" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
      </label>
      <label class="text-sm text-neutral-700">
        <span class="font-medium">{{ $t("views.adminLegalRequests.externalReference") }}</span>
        <input v-model="form.external_reference" maxlength="200" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
      </label>
      <label class="text-sm text-neutral-700">
        <span class="font-medium">{{ $t("views.adminLegalRequests.targetUserId") }}</span>
        <input v-model="form.target_user_id" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
      </label>
      <label class="text-sm text-neutral-700">
        <span class="font-medium">{{ $t("views.adminLegalRequests.targetHandle") }}</span>
        <input v-model="form.target_handle" maxlength="120" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
      </label>
      <label class="text-sm text-neutral-700">
        <span class="font-medium">{{ $t("views.adminLegalRequests.fromAt") }}</span>
        <input v-model="form.target_from_at" type="datetime-local" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
      </label>
      <label class="text-sm text-neutral-700">
        <span class="font-medium">{{ $t("views.adminLegalRequests.untilAt") }}</span>
        <input v-model="form.target_until_at" type="datetime-local" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
      </label>
      <label class="text-sm text-neutral-700 md:col-span-2">
        <span class="font-medium">{{ $t("views.adminLegalRequests.legalBasis") }}</span>
        <textarea v-model="form.legal_basis" maxlength="2000" rows="3" class="mt-1 w-full rounded-2xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
      </label>
      <label class="text-sm text-neutral-700">
        <span class="font-medium">{{ $t("views.adminLegalRequests.dataTypes") }}</span>
        <input v-model="form.data_types" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
      </label>
      <label class="flex items-center gap-2 self-end text-sm text-neutral-700">
        <input v-model="form.emergency" type="checkbox" class="rounded border-neutral-300" />
        <span>{{ $t("views.adminLegalRequests.emergency") }}</span>
      </label>
      <div class="md:col-span-2">
        <button type="submit" :disabled="saving" class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-60">
          {{ saving ? $t("views.adminLegalRequests.saving") : $t("views.adminLegalRequests.create") }}
        </button>
      </div>
    </form>

    <p v-if="loading" class="mt-8 text-sm text-neutral-500">{{ $t("views.adminLegalRequests.loading") }}</p>
    <div v-else class="mt-6 space-y-3">
      <article v-for="row in items" :key="row.id" class="rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">{{ row.request_type }} · {{ row.status }}</p>
            <h2 class="mt-1 text-base font-semibold text-neutral-900">{{ row.agency_name }}</h2>
            <p class="mt-1 text-sm text-neutral-600">{{ row.jurisdiction || row.external_reference || row.id }}</p>
          </div>
          <time class="text-xs text-neutral-500" :datetime="row.created_at">{{ formatDate(row.created_at) }}</time>
        </div>
        <p class="mt-3 whitespace-pre-wrap rounded-xl bg-neutral-50 px-3 py-2 text-sm text-neutral-700">{{ row.legal_basis || $t("views.adminLegalRequests.noLegalBasis") }}</p>
        <div class="mt-3 grid gap-2 text-xs text-neutral-500 md:grid-cols-2">
          <p>{{ $t("views.adminLegalRequests.target") }}: {{ row.target_handle || row.target_user_id || "-" }}</p>
          <p>{{ $t("views.adminLegalRequests.dataTypes") }}: {{ (row.data_types || []).join(", ") || "-" }}</p>
        </div>
        <div class="mt-3 flex flex-wrap items-center gap-3">
          <label class="flex items-center gap-2 text-sm text-neutral-700">
            <span>{{ $t("views.adminLegalRequests.status") }}</span>
            <select class="rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" :value="row.status" @change="updateStatus(row, ($event.target as HTMLSelectElement).value as LegalRequestRow['status'])">
              <option v-for="status in statusOptions" :key="status" :value="status">{{ status }}</option>
            </select>
          </label>
          <button type="button" class="font-medium text-lime-700 hover:underline" @click="exportRequest(row).catch((e: unknown) => { err = e instanceof Error ? e.message : String(e); })">
            {{ $t("views.adminLegalRequests.exportJson") }}
          </button>
          <button type="button" class="font-medium text-neutral-700 hover:underline" @click="holdForm.request_id = holdForm.request_id === row.id ? '' : row.id">
            {{ $t("views.adminLegalRequests.createHold") }}
          </button>
        </div>
        <form v-if="holdForm.request_id === row.id" class="mt-4 grid gap-4 rounded-2xl border border-neutral-200 bg-neutral-50 p-4 md:grid-cols-2" @submit.prevent="createHold(row)">
          <label class="text-xs text-neutral-700">
            <span class="font-semibold">{{ $t("views.adminLegalRequests.resourceType") }}</span>
            <select v-model="holdForm.resource_type" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40">
              <option value="account">account</option>
              <option value="dm_thread">dm_thread</option>
              <option value="dm_message">dm_message</option>
              <option value="report">report</option>
              <option value="media">media</option>
            </select>
          </label>
          <label class="text-xs text-neutral-700">
            <span class="font-semibold">{{ $t("views.adminLegalRequests.holdExpiresAt") }}</span>
            <input v-model="holdForm.expires_at" required type="datetime-local" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
          </label>
          <label class="text-xs text-neutral-700">
            <span class="font-semibold">{{ $t("views.adminLegalRequests.targetUserId") }}</span>
            <input v-model="holdForm.target_user_id" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" :placeholder="row.target_user_id || ''" />
          </label>
          <label class="text-xs text-neutral-700">
            <span class="font-semibold">{{ $t("views.adminLegalRequests.resourceId") }}</span>
            <input v-model="holdForm.resource_id" class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
          </label>
          <label class="text-xs text-neutral-700 md:col-span-2">
            <span class="font-semibold">{{ $t("views.adminLegalRequests.holdReason") }}</span>
            <textarea v-model="holdForm.reason" rows="2" maxlength="2000" class="mt-1 w-full rounded-2xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40" />
          </label>
          <div class="md:col-span-2">
            <button type="submit" class="rounded-full bg-lime-600 px-4 py-2 text-xs font-semibold text-white hover:bg-lime-700">
              {{ $t("views.adminLegalRequests.saveHold") }}
            </button>
          </div>
        </form>
      </article>
      <p v-if="!items.length" class="text-sm text-neutral-500">{{ $t("views.adminLegalRequests.empty") }}</p>
    </div>
  </div>
</template>
