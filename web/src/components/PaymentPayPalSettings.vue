<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { listPayPalPlans, upsertPayPalPlan } from "../lib/paymentPayPal";
import { getAccessToken } from "../auth";

const { t } = useI18n();
const token = computed(() => getAccessToken());

const loading = ref(false);
const error = ref("");
const plans = ref<{ id: string; plan_id: string; label?: string; active?: boolean }[]>([]);
const rowBusy = ref<string | null>(null);

const planID = ref("");
const label = ref("");
const active = ref(true);

async function refresh() {
  if (!token.value) return;
  loading.value = true;
  error.value = "";
  try {
    plans.value = await listPayPalPlans(token.value);
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (!token.value) return;
  const pid = planID.value.trim();
  if (!pid) return;
  loading.value = true;
  error.value = "";
  try {
    await upsertPayPalPlan(token.value, { plan_id: pid, label: label.value.trim(), active: active.value });
    planID.value = "";
    label.value = "";
    active.value = true;
    await refresh();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

async function saveExisting(p: { id: string; plan_id: string; label?: string; active?: boolean }) {
  if (!token.value || !p.plan_id.trim()) return;
  rowBusy.value = p.id;
  error.value = "";
  try {
    await upsertPayPalPlan(token.value, { plan_id: p.plan_id, label: (p.label ?? "").trim(), active: p.active !== false });
    await refresh();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    rowBusy.value = null;
  }
}

onMounted(refresh);
</script>

<template>
  <div class="overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
    <div class="px-4 py-4">
      <div class="flex items-start justify-between gap-4">
        <div>
          <h3 class="text-sm font-semibold text-neutral-900">{{ t("payments.paypal.title") }}</h3>
          <p class="mt-1 text-xs text-neutral-600">
            {{ t("payments.paypal.lead") }}
          </p>
          <p class="mt-1 text-xs text-neutral-500">
            {{ t("payments.paypal.planHelp") }}
          </p>
        </div>
        <button
          class="rounded-xl border border-neutral-200 bg-white px-3 py-1.5 text-xs font-medium text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
          :disabled="loading"
          @click="refresh"
        >
          {{ t("common.actions.refresh") }}
        </button>
      </div>

      <div v-if="error" class="mt-3 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800">
        {{ error }}
      </div>

      <div class="mt-4 grid gap-2">
        <div class="grid gap-3 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_8rem]">
          <label class="grid min-w-0 gap-1 text-xs text-neutral-700">
            <span class="font-medium">{{ t("payments.paypal.planId") }}</span>
            <input v-model="planID" class="min-w-0 rounded-xl border border-neutral-200 px-3 py-2 text-sm" />
          </label>
          <label class="grid min-w-0 gap-1 text-xs text-neutral-700">
            <span class="font-medium">{{ t("payments.paypal.label") }}</span>
            <input v-model="label" class="min-w-0 rounded-xl border border-neutral-200 px-3 py-2 text-sm" />
          </label>
          <label class="grid min-w-0 gap-1 text-xs text-neutral-700">
            <span class="font-medium">{{ t("payments.paypal.active") }}</span>
            <select v-model="active" class="min-w-0 rounded-xl border border-neutral-200 px-3 py-2 text-sm">
              <option :value="true">{{ t("common.labels.on") }}</option>
              <option :value="false">{{ t("common.labels.off") }}</option>
            </select>
          </label>
        </div>
        <div>
          <button
            class="rounded-xl bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-500 disabled:opacity-50"
            :disabled="loading || !planID.trim()"
            @click="save"
          >
            {{ t("common.actions.save") }}
          </button>
        </div>
      </div>

      <div class="mt-4">
        <p class="text-xs font-semibold uppercase tracking-wide text-neutral-500">{{ t("payments.paypal.registeredPlans") }}</p>
        <div class="mt-2 space-y-2">
          <div
            v-for="p in plans"
            :key="p.id"
            class="grid gap-3 rounded-xl border border-neutral-200 px-3 py-3 text-sm lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_8rem_auto]"
          >
            <div class="min-w-0">
              <div class="truncate font-medium text-neutral-900">{{ p.label || p.plan_id }}</div>
              <div class="truncate text-xs text-neutral-500">{{ p.plan_id }}</div>
            </div>
            <label class="grid min-w-0 gap-1 text-xs text-neutral-700">
              <span class="font-medium">{{ t("payments.paypal.label") }}</span>
              <input v-model="p.label" class="min-w-0 rounded-xl border border-neutral-200 px-3 py-2 text-sm" />
            </label>
            <label class="grid min-w-0 gap-1 text-xs text-neutral-700">
              <span class="font-medium">{{ t("payments.paypal.active") }}</span>
              <select v-model="p.active" class="min-w-0 rounded-xl border border-neutral-200 px-3 py-2 text-sm">
                <option :value="true">{{ t("common.labels.on") }}</option>
                <option :value="false">{{ t("common.labels.off") }}</option>
              </select>
            </label>
            <button
              class="self-end rounded-xl border border-neutral-200 bg-white px-3 py-2 text-xs font-medium text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
              :disabled="rowBusy === p.id || loading"
              @click="saveExisting(p)"
            >
              {{ rowBusy === p.id ? t("common.actions.saving") : t("common.actions.save") }}
            </button>
          </div>
          <div v-if="plans.length === 0" class="text-xs text-neutral-500">
            {{ t("payments.paypal.noPlans") }}
            <span class="block">{{ t("payments.paypal.noPlansHelp") }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

