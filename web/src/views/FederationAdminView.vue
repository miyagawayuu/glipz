<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import { api } from "../lib/api";
import { getAccessToken } from "../auth";

const { t } = useI18n();
const router = useRouter();
const err = ref("");
const loading = ref(true);
const isAdmin = ref(false);

const pending = ref(0);
const dead = ref(0);

type DeliveryRow = {
  id: string;
  kind: string;
  inbox_url: string;
  status: string;
  attempt_count: number;
  last_error_short: string;
  created_at: string;
};

const deliveries = ref<DeliveryRow[]>([]);
const deliveryStatus = ref("pending");

type BlockRow = { host: string; note: string; created_at: string };
const blocks = ref<BlockRow[]>([]);
const newHost = ref("");
const newNote = ref("");

type KnownInstanceRow = { host: string; note: string; created_at: string };
const knownInstances = ref<KnownInstanceRow[]>([]);
const newKnownHost = ref("");
const newKnownNote = ref("");

async function loadMe() {
  const token = getAccessToken();
  if (!token) {
    router.replace("/login");
    return;
  }
  try {
    const u = await api<{ is_site_admin?: boolean }>("/api/v1/me", { method: "GET", token });
    isAdmin.value = !!u.is_site_admin;
    if (!isAdmin.value) {
      err.value = t("views.adminFederation.errors.adminOnly");
    }
  } catch {
    err.value = t("views.adminFederation.errors.userFetchFailed");
  }
}

async function loadCounts() {
  const token = getAccessToken();
  if (!token || !isAdmin.value) return;
  const c = await api<{ pending: number; dead: number }>("/api/v1/admin/federation/delivery-counts", {
    method: "GET",
    token,
  });
  pending.value = Number(c.pending) || 0;
  dead.value = Number(c.dead) || 0;
}

async function loadDeliveries() {
  const token = getAccessToken();
  if (!token || !isAdmin.value) return;
  const q = deliveryStatus.value === "all" ? "" : `?status=${encodeURIComponent(deliveryStatus.value)}`;
  const res = await api<{ items: DeliveryRow[] }>(`/api/v1/admin/federation/deliveries${q}`, {
    method: "GET",
    token,
  });
  deliveries.value = res.items ?? [];
}

async function loadBlocks() {
  const token = getAccessToken();
  if (!token || !isAdmin.value) return;
  const res = await api<{ items: BlockRow[] }>("/api/v1/admin/federation/domain-blocks", { method: "GET", token });
  blocks.value = res.items ?? [];
}

async function loadKnownInstances() {
  const token = getAccessToken();
  if (!token || !isAdmin.value) return;
  const res = await api<{ items: KnownInstanceRow[] }>("/api/v1/admin/federation/known-instances", {
    method: "GET",
    token,
  });
  knownInstances.value = res.items ?? [];
}

async function refresh() {
  err.value = "";
  loading.value = true;
  await loadMe();
  if (isAdmin.value) {
    try {
      await loadCounts();
      await loadDeliveries();
      await loadBlocks();
      await loadKnownInstances();
    } catch (e: unknown) {
      err.value = e instanceof Error ? e.message : t("views.adminFederation.errors.loadFailed");
    }
  }
  loading.value = false;
}

async function addBlock() {
  err.value = "";
  const token = getAccessToken();
  if (!token) return;
  try {
    await api("/api/v1/admin/federation/domain-blocks", {
      method: "POST",
      token,
      json: { host: newHost.value.trim(), note: newNote.value.trim() },
    });
    newHost.value = "";
    newNote.value = "";
    await loadBlocks();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminFederation.errors.addFailed");
  }
}

async function removeBlock(host: string) {
  err.value = "";
  const token = getAccessToken();
  if (!token) return;
  try {
    await api(`/api/v1/admin/federation/domain-blocks?host=${encodeURIComponent(host)}`, {
      method: "DELETE",
      token,
    });
    await loadBlocks();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminFederation.errors.removeFailed");
  }
}

async function addKnownInstance() {
  err.value = "";
  const token = getAccessToken();
  if (!token) return;
  try {
    await api("/api/v1/admin/federation/known-instances", {
      method: "POST",
      token,
      json: { host: newKnownHost.value.trim(), note: newKnownNote.value.trim() },
    });
    newKnownHost.value = "";
    newKnownNote.value = "";
    await loadKnownInstances();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminFederation.errors.addFailed");
  }
}

async function removeKnownInstance(host: string) {
  err.value = "";
  const token = getAccessToken();
  if (!token) return;
  try {
    await api(`/api/v1/admin/federation/known-instances?host=${encodeURIComponent(host)}`, {
      method: "DELETE",
      token,
    });
    await loadKnownInstances();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminFederation.errors.removeFailed");
  }
}

onMounted(() => {
  void refresh();
});
</script>

<template>
  <div class="mx-auto max-w-6xl px-4 py-8">
    <header>
      <p class="text-xs font-semibold uppercase tracking-[0.18em] text-lime-700">{{ t("views.adminShell.eyebrow") }}</p>
      <h1 class="mt-2 text-2xl font-bold text-neutral-900">{{ t("views.adminFederation.title") }}</h1>
      <p class="mt-2 max-w-3xl text-sm leading-6 text-neutral-600">
        {{ t("views.adminFederation.descriptionPrefix") }}<code class="rounded bg-neutral-100 px-1">GLIPZ_ADMIN_USER_IDS</code>{{ t("views.adminFederation.descriptionSuffix") }}
      </p>
    </header>
    <p v-if="err" class="mt-5 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">{{ err }}</p>
    <p v-if="loading" class="mt-8 text-sm text-neutral-500">{{ t("views.adminFederation.loading") }}</p>
    <template v-else-if="isAdmin">
      <section class="mt-8 rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
        <h2 class="text-sm font-semibold uppercase tracking-wide text-neutral-500">{{ t("views.adminFederation.queueHeading") }}</h2>
        <p class="mt-2 text-sm text-neutral-800">
          {{ t("views.adminFederation.queueSummary", { pending, dead }) }}
        </p>
      </section>
      <section class="mt-6 rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
        <div class="flex flex-wrap items-end gap-3">
          <label class="block text-sm">
            <span class="mb-1 block font-medium text-neutral-700">{{ t("views.adminFederation.stateLabel") }}</span>
            <select
              v-model="deliveryStatus"
              class="block rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              @change="loadDeliveries"
            >
              <option value="pending">pending</option>
              <option value="dead">dead</option>
              <option value="completed">completed</option>
              <option value="all">all</option>
            </select>
          </label>
          <button
            type="button"
            class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700"
            @click="loadDeliveries"
          >
            {{ t("views.adminFederation.refresh") }}
          </button>
        </div>
        <div class="mt-4 overflow-x-auto rounded-2xl border border-neutral-200">
          <table class="min-w-full divide-y divide-neutral-200 text-left text-xs">
            <thead class="bg-neutral-50 text-neutral-600">
              <tr>
                <th class="px-2 py-2">{{ t("views.adminFederation.colKind") }}</th>
                <th class="px-2 py-2">{{ t("views.adminFederation.colStatus") }}</th>
                <th class="px-2 py-2">{{ t("views.adminFederation.colAttempts") }}</th>
                <th class="px-2 py-2">{{ t("views.adminFederation.colInbox") }}</th>
                <th class="px-2 py-2">{{ t("views.adminFederation.colError") }}</th>
                <th class="px-2 py-2">{{ t("views.adminFederation.colCreated") }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-neutral-200">
              <tr v-for="d in deliveries" :key="d.id">
                <td class="px-2 py-1.5 font-mono">{{ d.kind }}</td>
                <td class="px-2 py-1.5">{{ d.status }}</td>
                <td class="px-2 py-1.5">{{ d.attempt_count }}</td>
                <td class="max-w-[200px] truncate px-2 py-1.5 font-mono text-[11px]" :title="d.inbox_url">
                  {{ d.inbox_url }}
                </td>
                <td class="max-w-[220px] truncate px-2 py-1.5 text-[11px]" :title="d.last_error_short">
                  {{ d.last_error_short }}
                </td>
                <td class="whitespace-nowrap px-2 py-1.5 text-[11px]">{{ d.created_at }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <section class="mt-6 rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
        <h2 class="text-sm font-semibold uppercase tracking-wide text-neutral-500">{{ t("views.adminFederation.domainBlockHeading") }}</h2>
        <form class="mt-4 flex flex-wrap items-end gap-3" @submit.prevent="addBlock">
          <input
            v-model="newHost"
            type="text"
            required
            :placeholder="t('views.adminFederation.hostPlaceholder')"
            class="min-w-[12rem] rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
          />
          <input
            v-model="newNote"
            type="text"
            :placeholder="t('views.adminFederation.notePlaceholder')"
            class="min-w-[10rem] flex-1 rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
          />
          <button type="submit" class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700">
            {{ t("views.adminFederation.add") }}
          </button>
        </form>
        <ul class="mt-4 space-y-2 text-sm">
          <li
            v-for="b in blocks"
            :key="b.host"
            class="flex items-center justify-between gap-2 rounded-2xl border border-neutral-200 bg-neutral-50 px-4 py-3"
          >
            <div>
              <span class="font-mono font-medium">{{ b.host }}</span>
              <span v-if="b.note" class="ml-2 text-neutral-600">{{ b.note }}</span>
              <div class="text-xs text-neutral-400">{{ b.created_at }}</div>
            </div>
            <button
              type="button"
              class="shrink-0 text-xs text-red-700 underline"
              @click="removeBlock(b.host)"
            >
              {{ t("views.adminFederation.delete") }}
            </button>
          </li>
        </ul>
      </section>

      <section class="mt-6 rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
        <h2 class="text-sm font-semibold uppercase tracking-wide text-neutral-500">
          {{ t("views.adminFederation.knownInstancesHeading") }}
        </h2>
        <p class="mt-2 text-sm text-neutral-600">{{ t("views.adminFederation.knownInstancesHint") }}</p>
        <form class="mt-4 flex flex-wrap items-end gap-3" @submit.prevent="addKnownInstance">
          <input
            v-model="newKnownHost"
            type="text"
            required
            :placeholder="t('views.adminFederation.hostPlaceholder')"
            class="min-w-[12rem] rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
          />
          <input
            v-model="newKnownNote"
            type="text"
            :placeholder="t('views.adminFederation.notePlaceholder')"
            class="min-w-[10rem] flex-1 rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
          />
          <button type="submit" class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700">
            {{ t("views.adminFederation.add") }}
          </button>
        </form>
        <ul class="mt-4 space-y-2 text-sm">
          <li
            v-for="k in knownInstances"
            :key="k.host"
            class="flex items-center justify-between gap-2 rounded-2xl border border-neutral-200 bg-neutral-50 px-4 py-3"
          >
            <div>
              <span class="font-mono font-medium">{{ k.host }}</span>
              <span v-if="k.note" class="ml-2 text-neutral-600">{{ k.note }}</span>
              <div class="text-xs text-neutral-400">{{ k.created_at }}</div>
            </div>
            <button
              type="button"
              class="shrink-0 text-xs text-red-700 underline"
              @click="removeKnownInstance(k.host)"
            >
              {{ t("views.adminFederation.delete") }}
            </button>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>
