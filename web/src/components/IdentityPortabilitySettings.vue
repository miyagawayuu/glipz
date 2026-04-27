<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "vue-i18n";
import {
  declareIdentityMove,
  exportIdentityBundle,
  importIdentityBundle,
  type IdentityBundle,
} from "../lib/identityPortability";

const { t } = useI18n();

const bundleText = ref("");
const movedToAcct = ref("");
const busy = ref(false);
const message = ref("");
const error = ref("");

function clearStatus() {
  message.value = "";
  error.value = "";
}

async function onExport() {
  clearStatus();
  busy.value = true;
  try {
    const bundle = await exportIdentityBundle();
    bundleText.value = JSON.stringify(bundle, null, 2);
    message.value = t("components.identityPortability.exported");
  } catch (err) {
    error.value = err instanceof Error ? err.message : t("components.identityPortability.failed");
  } finally {
    busy.value = false;
  }
}

async function onImport() {
  clearStatus();
  let parsed: IdentityBundle;
  try {
    parsed = JSON.parse(bundleText.value) as IdentityBundle;
  } catch {
    error.value = t("components.identityPortability.invalidJson");
    return;
  }
  busy.value = true;
  try {
    await importIdentityBundle(parsed);
    message.value = t("components.identityPortability.imported");
  } catch (err) {
    error.value = err instanceof Error ? err.message : t("components.identityPortability.failed");
  } finally {
    busy.value = false;
  }
}

async function onMove() {
  clearStatus();
  const acct = movedToAcct.value.trim();
  if (!acct) {
    error.value = t("components.identityPortability.missingMovedTo");
    return;
  }
  busy.value = true;
  try {
    await declareIdentityMove(acct);
    message.value = t("components.identityPortability.moveDeclared");
  } catch (err) {
    error.value = err instanceof Error ? err.message : t("components.identityPortability.failed");
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="mt-3 space-y-4 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
    <div>
      <p class="text-sm font-medium text-neutral-900">{{ $t("components.identityPortability.title") }}</p>
      <p class="mt-1 text-xs leading-5 text-neutral-500">
        {{ $t("components.identityPortability.description") }}
      </p>
    </div>

    <div class="space-y-2">
      <button
        type="button"
        class="rounded-xl bg-neutral-900 px-4 py-2 text-sm font-semibold text-white transition hover:bg-neutral-700 disabled:cursor-not-allowed disabled:opacity-60"
        :disabled="busy"
        @click="onExport"
      >
        {{ $t("components.identityPortability.exportButton") }}
      </button>
      <textarea
        v-model="bundleText"
        class="min-h-40 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 font-mono text-xs text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
        :placeholder="$t('components.identityPortability.bundlePlaceholder')"
      ></textarea>
      <button
        type="button"
        class="rounded-xl border border-neutral-300 px-4 py-2 text-sm font-semibold text-neutral-900 transition hover:bg-neutral-50 disabled:cursor-not-allowed disabled:opacity-60"
        :disabled="busy || !bundleText.trim()"
        @click="onImport"
      >
        {{ $t("components.identityPortability.importButton") }}
      </button>
    </div>

    <div class="border-t border-neutral-200 pt-4">
      <label class="text-sm font-medium text-neutral-900" for="identity-moved-to">
        {{ $t("components.identityPortability.moveLabel") }}
      </label>
      <div class="mt-2 flex flex-col gap-2 sm:flex-row">
        <input
          id="identity-moved-to"
          v-model="movedToAcct"
          class="min-w-0 flex-1 rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
          placeholder="alice@example.social"
        />
        <button
          type="button"
          class="rounded-xl border border-neutral-300 px-4 py-2 text-sm font-semibold text-neutral-900 transition hover:bg-neutral-50 disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="busy || !movedToAcct.trim()"
          @click="onMove"
        >
          {{ $t("components.identityPortability.moveButton") }}
        </button>
      </div>
      <p class="mt-2 text-xs leading-5 text-neutral-500">
        {{ $t("components.identityPortability.moveHint") }}
      </p>
    </div>

    <p v-if="message" class="text-xs font-medium text-lime-700">{{ message }}</p>
    <p v-if="error" class="text-xs font-medium text-red-600">{{ error }}</p>
  </div>
</template>
