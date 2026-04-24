<script setup lang="ts">
import { onActivated, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { getAccessToken } from "../auth";
import { disconnectPatreon, fetchPatreonStatus, startPatreonOAuth } from "../lib/fanclubPatreon";

const { t } = useI18n();
const available = ref(false);
const connected = ref(false);
const busy = ref(false);
const connectBusy = ref(false);
const err = ref("");

async function load() {
  err.value = "";
  const token = getAccessToken();
  if (!token) {
    available.value = false;
    return;
  }
  try {
    const s = await fetchPatreonStatus(token);
    const p = s.patreon;
    available.value = Boolean(p?.available);
    connected.value = Boolean(p?.available && p?.connected);
  } catch (e) {
    err.value = e instanceof Error ? e.message : t("views.settings.fanclubPatreon.loadError");
  }
}

async function onConnectPatreon() {
  if (connectBusy.value) return;
  connectBusy.value = true;
  err.value = "";
  try {
    await startPatreonOAuth("/settings");
  } catch (e) {
    err.value = e instanceof Error ? e.message : t("views.settings.fanclubPatreon.loadError");
  } finally {
    connectBusy.value = false;
  }
}

async function onDisconnect() {
  if (busy.value) return;
  const token = getAccessToken();
  if (!token) return;
  busy.value = true;
  err.value = "";
  try {
    await disconnectPatreon(token);
    connected.value = false;
  } catch (e) {
    err.value = e instanceof Error ? e.message : t("views.settings.fanclubPatreon.disconnectError");
  } finally {
    busy.value = false;
  }
}

onMounted(() => {
  void load();
});

onActivated(() => {
  void load();
});
</script>

<template>
  <div class="mt-3 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
    <p class="text-sm font-medium text-neutral-900">{{ $t("views.settings.fanclubPatreon.title") }}</p>
    <template v-if="!available">
      <p class="mt-2 text-sm text-neutral-500">{{ $t("views.settings.fanclubPatreon.unavailable") }}</p>
    </template>
    <template v-else>
      <p class="mt-1 text-xs text-neutral-500">
        {{ connected ? $t("views.settings.fanclubPatreon.leadConnected") : $t("views.settings.fanclubPatreon.lead") }}
      </p>
      <div class="mt-3 flex flex-wrap items-center gap-2">
        <button
          v-if="!connected"
          type="button"
          class="inline-flex rounded-full bg-sky-600 px-4 py-2 text-sm font-semibold text-white hover:bg-sky-700 disabled:opacity-50"
          :disabled="connectBusy"
          @click="onConnectPatreon"
        >
          {{ connectBusy ? $t("views.settings.fanclubPatreon.connecting") : $t("views.settings.fanclubPatreon.connect") }}
        </button>
        <button
          v-else
          type="button"
          class="rounded-full border border-neutral-300 px-3 py-1.5 text-sm text-neutral-800 hover:bg-neutral-50 disabled:opacity-50"
          :disabled="busy"
          @click="onDisconnect"
        >
          {{ $t("views.settings.fanclubPatreon.disconnect") }}
        </button>
      </div>
    </template>
    <p v-if="err" class="mt-2 text-xs text-red-600">{{ err }}</p>
  </div>
</template>
