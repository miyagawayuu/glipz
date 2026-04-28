<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import { clearTokens } from "../auth";
import { accountDeletionInputReady, deleteAccount } from "../lib/account";

const { t } = useI18n();
const router = useRouter();

const password = ref("");
const confirmText = ref("");
const busy = ref(false);
const error = ref("");
const message = ref("");
const progress = ref(0);
const stage = ref<"idle" | "starting" | "deleting" | "completed">("idle");
let completionTimer: number | undefined;

const canDelete = computed(() =>
  !busy.value
  && accountDeletionInputReady(password.value, confirmText.value),
);
const progressLabel = computed(() => {
  if (stage.value === "completed") return t("views.settings.accountDeletion.progressCompleted");
  if (stage.value === "deleting") return t("views.settings.accountDeletion.progressDeleting");
  if (stage.value === "starting") return t("views.settings.accountDeletion.progressStarting");
  return "";
});

function setError(err: unknown) {
  const msg = err instanceof Error ? err.message : "";
  if (msg === "invalid_password") {
    error.value = t("views.settings.accountDeletion.invalidPassword");
  } else if (msg === "confirmation_required") {
    error.value = t("views.settings.accountDeletion.confirmationRequired");
  } else {
    error.value = msg || t("views.settings.accountDeletion.failed");
  }
}

async function onDeleteAccount() {
  error.value = "";
  message.value = "";
  stage.value = "starting";
  progress.value = 15;
  busy.value = true;
  try {
    window.setTimeout(() => {
      if (busy.value && stage.value === "starting") {
        stage.value = "deleting";
        progress.value = 65;
      }
    }, 250);
    await deleteAccount({ password: password.value, confirm: confirmText.value });
    stage.value = "completed";
    progress.value = 100;
    message.value = t("views.settings.accountDeletion.completed");
    password.value = "";
    confirmText.value = "";
    clearTokens();
    completionTimer = window.setTimeout(() => {
      void router.push("/");
    }, 3000);
  } catch (err) {
    stage.value = "idle";
    progress.value = 0;
    setError(err);
  } finally {
    busy.value = false;
  }
}

onBeforeUnmount(() => {
  if (completionTimer) window.clearTimeout(completionTimer);
});
</script>

<template>
  <section>
    <h2 class="text-xs font-semibold uppercase tracking-wide text-red-600">
      {{ $t("views.settings.accountDeletion.section") }}
    </h2>
    <div class="mt-3 space-y-4 rounded-2xl border border-red-200 bg-red-50/40 p-4 shadow-sm">
      <div>
        <p class="text-sm font-semibold text-red-900">{{ $t("views.settings.accountDeletion.title") }}</p>
        <p class="mt-1 text-xs leading-5 text-red-800">
          {{ $t("views.settings.accountDeletion.description") }}
        </p>
      </div>

      <div v-if="message || error" class="rounded-xl border px-3 py-2 text-xs"
        :class="error ? 'border-red-200 bg-white text-red-700' : 'border-lime-200 bg-lime-50 text-lime-800'"
      >
        {{ error || message }}
      </div>

      <div v-if="stage !== 'idle'" class="space-y-2 rounded-xl border border-red-100 bg-white p-3">
        <div class="flex items-center justify-between text-xs text-neutral-600">
          <span>{{ progressLabel }}</span>
          <span>{{ progress }}%</span>
        </div>
        <div class="h-2 overflow-hidden rounded-full bg-neutral-100">
          <div class="h-full bg-red-500 transition-all" :style="{ width: `${progress}%` }"></div>
        </div>
      </div>

      <label class="block text-xs font-medium text-neutral-700">
        {{ $t("views.settings.accountDeletion.password") }}
        <input
          v-model="password"
          type="password"
          autocomplete="current-password"
          class="mt-1 w-full rounded-xl border border-red-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-red-500/30 transition focus:border-red-400 focus:ring-2 focus:ring-red-400/40"
          :disabled="busy || stage === 'completed'"
        />
      </label>

      <label class="block text-xs font-medium text-neutral-700">
        {{ $t("views.settings.accountDeletion.confirmLabel") }}
        <input
          v-model="confirmText"
          class="mt-1 w-full rounded-xl border border-red-200 bg-white px-3 py-2 font-mono text-sm text-neutral-900 outline-none ring-red-500/30 transition focus:border-red-400 focus:ring-2 focus:ring-red-400/40"
          placeholder="DELETE"
          :disabled="busy || stage === 'completed'"
        />
      </label>

      <button
        type="button"
        class="rounded-xl bg-red-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-60"
        :disabled="!canDelete"
        @click="onDeleteAccount"
      >
        {{
          busy
            ? $t("views.settings.accountDeletion.deletingButton")
            : $t("views.settings.accountDeletion.deleteButton")
        }}
      </button>
    </div>
  </section>
</template>
