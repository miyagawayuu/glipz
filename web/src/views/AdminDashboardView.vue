<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { useI18n } from "vue-i18n";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";

type Overview = {
  users_total: number;
  users_suspended: number;
  reports_open_local: number;
  reports_open_federated: number;
  federation_pending: number;
  federation_dead: number;
  registrations_enabled: boolean;
  federation_policy_summary: string;
  operator_announcements_count: number;
};

const { t } = useI18n();
const loading = ref(true);
const err = ref("");
const overview = ref<Overview | null>(null);

const cards = computed(() => {
  const o = overview.value;
  return [
    { label: t("views.adminDashboard.cards.users"), value: o?.users_total ?? 0, to: "/admin/users" },
    { label: t("views.adminDashboard.cards.suspended"), value: o?.users_suspended ?? 0, to: "/admin/users?status=suspended" },
    { label: t("views.adminDashboard.cards.reports"), value: (o?.reports_open_local ?? 0) + (o?.reports_open_federated ?? 0), to: "/admin/reports" },
    { label: t("views.adminDashboard.cards.federation"), value: o?.federation_pending ?? 0, to: "/admin/federation" },
  ];
});

async function refresh() {
  const token = getAccessToken();
  if (!token) return;
  loading.value = true;
  err.value = "";
  try {
    overview.value = await api<Overview>("/api/v1/admin/overview", { method: "GET", token });
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminDashboard.errors.loadFailed");
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
        <h1 class="mt-2 text-2xl font-bold text-neutral-900">{{ $t("views.adminDashboard.title") }}</h1>
        <p class="mt-2 text-sm text-neutral-600">{{ $t("views.adminDashboard.description") }}</p>
      </div>
      <button
        type="button"
        class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700"
        @click="refresh"
      >
        {{ $t("common.actions.refresh") }}
      </button>
    </header>

    <p v-if="err" class="mt-5 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">{{ err }}</p>
    <p v-if="loading" class="mt-8 text-sm text-neutral-500">{{ $t("app.loading") }}</p>

    <template v-else-if="overview">
      <div class="mt-8 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <RouterLink
          v-for="card in cards"
          :key="card.label"
          :to="card.to"
          class="rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm transition hover:border-lime-300 hover:bg-lime-50/50"
        >
          <p class="text-sm font-medium text-neutral-500">{{ card.label }}</p>
          <p class="mt-3 text-3xl font-bold text-neutral-900">{{ card.value }}</p>
        </RouterLink>
      </div>

      <section class="mt-8 grid gap-4 lg:grid-cols-2">
        <article class="rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
          <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.adminDashboard.instanceHeading") }}</h2>
          <dl class="mt-4 space-y-3 text-sm">
            <div class="flex items-center justify-between gap-4">
              <dt class="text-neutral-500">{{ $t("views.adminDashboard.registrations") }}</dt>
              <dd class="font-semibold" :class="overview.registrations_enabled ? 'text-lime-700' : 'text-red-700'">
                {{ overview.registrations_enabled ? $t("common.labels.active") : $t("common.labels.inactive") }}
              </dd>
            </div>
            <div class="flex items-center justify-between gap-4">
              <dt class="text-neutral-500">{{ $t("views.adminDashboard.announcements") }}</dt>
              <dd class="font-semibold text-neutral-900">{{ overview.operator_announcements_count }}</dd>
            </div>
          </dl>
          <RouterLink to="/admin/instance-settings" class="mt-5 inline-flex text-sm font-semibold text-lime-700 hover:underline">
            {{ $t("views.adminDashboard.openSettings") }}
          </RouterLink>
        </article>

        <article class="rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
          <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.adminDashboard.federationHeading") }}</h2>
          <dl class="mt-4 grid gap-3 text-sm sm:grid-cols-2">
            <div class="rounded-2xl bg-neutral-50 p-4">
              <dt class="text-neutral-500">{{ $t("views.adminDashboard.pendingDeliveries") }}</dt>
              <dd class="mt-2 text-2xl font-bold text-neutral-900">{{ overview.federation_pending }}</dd>
            </div>
            <div class="rounded-2xl bg-neutral-50 p-4">
              <dt class="text-neutral-500">{{ $t("views.adminDashboard.deadDeliveries") }}</dt>
              <dd class="mt-2 text-2xl font-bold text-neutral-900">{{ overview.federation_dead }}</dd>
            </div>
          </dl>
        </article>
      </section>
    </template>
  </div>
</template>
