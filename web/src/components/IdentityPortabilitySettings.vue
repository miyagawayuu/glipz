<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import { useI18n } from "vue-i18n";
import {
  cancelIdentityImportJob,
  createIdentityImportJob,
  createIdentityTransferSession,
  declareIdentityMove,
  exportSecureIdentityBundle,
  getIdentityImportJob,
  importSecureIdentityBundle,
  retryIdentityImportJob,
  type IdentityBundle,
  type IdentityTransferImportJob,
} from "../lib/identityPortability";

const { t } = useI18n();

const movedToAcct = ref("");
const busy = ref(false);
const message = ref("");
const error = ref("");

const targetOrigin = ref("");
const sourceOrigin = ref("");
const sourceSessionID = ref("");
const transferToken = ref("");
const passphrase = ref("");
const passphraseConfirm = ref("");
const showPassphrase = ref(false);
const includePrivate = ref(false);
const includeGated = ref(false);
const secureBundleText = ref("");
const job = ref<IdentityTransferImportJob | null>(null);
let pollTimer: number | undefined;

function currentOrigin(): string {
  return typeof window !== "undefined" ? window.location.origin : "";
}

function normalizeOrigin(raw: string): string {
  try {
    return new URL(raw.trim()).origin.replace(/\/+$/, "");
  } catch {
    return raw.trim().replace(/\/+$/, "");
  }
}

const progressPct = computed(() => {
  if (!job.value) return 0;
  const total = job.value.total_items > 0 ? job.value.total_items : job.value.total_posts;
  const imported = job.value.total_items > 0 ? job.value.imported_items : job.value.imported_posts;
  if (total <= 0) return 0;
  return Math.min(100, Math.round((imported / total) * 100));
});
const jobProgressLabel = computed(() => {
  if (!job.value) return "";
  const total = job.value.total_items > 0 ? job.value.total_items : job.value.total_posts;
  const imported = job.value.total_items > 0 ? job.value.imported_items : job.value.imported_posts;
  return `${imported} / ${total}`;
});
const jobStatsSummary = computed(() => {
  if (!job.value?.stats) return [];
  const stats = job.value.stats;
  return ([
    ["profile", stats.profile],
    ["posts", stats.posts],
    ["following", stats.following],
    ["followers", stats.followers],
    ["bookmarks", stats.bookmarks],
  ] as const).flatMap(([key, value]) => {
    if (!value || value.total <= 0) return [];
    return [{
      key: String(key),
      label: t(`components.identityPortability.transferWizard.stats.${key}`),
      value: `${value.imported}/${value.total}${value.skipped ? ` (${t("components.identityPortability.transferWizard.skipped", { count: value.skipped })})` : ""}`,
    }];
  });
});
const passphraseTooShort = computed(() => passphrase.value.trim().length > 0 && passphrase.value.trim().length < 12);
const passphraseMismatch = computed(() => passphraseConfirm.value.length > 0 && passphrase.value !== passphraseConfirm.value);
const canCreateTransferSession = computed(() =>
  !busy.value
  && targetOrigin.value.trim().length > 0
  && passphrase.value.trim().length >= 12
  && passphrase.value === passphraseConfirm.value,
);
const secureBundleOriginWarning = computed(() => {
  if (!secureBundleText.value.trim() || !currentOrigin()) return "";
  try {
    const parsed = JSON.parse(secureBundleText.value) as IdentityBundle;
    if (parsed.created_for_origin && normalizeOrigin(parsed.created_for_origin) !== normalizeOrigin(currentOrigin())) {
      return t("components.identityPortability.transferWizard.bundleOriginWarning", {
        expected: parsed.created_for_origin,
        actual: currentOrigin(),
      });
    }
  } catch {
    return "";
  }
  return "";
});
const sourceOriginLooksCurrent = computed(() =>
  sourceOrigin.value.trim().length > 0
  && currentOrigin().length > 0
  && normalizeOrigin(sourceOrigin.value) === normalizeOrigin(currentOrigin()),
);
const visibleJobError = computed(() => {
  const lastError = job.value?.last_error?.trim() ?? "";
  if (lastError.includes("source status 401")) {
    return t("components.identityPortability.transferWizard.sourceUnauthorizedHint");
  }
  return lastError;
});

function clearStatus() {
  message.value = "";
  error.value = "";
}

function setError(err: unknown) {
  const msg = err instanceof Error ? err.message : "";
  error.value = msg === "unauthorized"
    ? t("components.identityPortability.transferWizard.loginRequired")
    : msg || t("components.identityPortability.failed");
}

function validatePassphrase() {
  if (passphrase.value.trim().length < 12) {
    error.value = t("components.identityPortability.transferWizard.weakPassphrase");
    return false;
  }
  if (passphrase.value !== passphraseConfirm.value) {
    error.value = t("components.identityPortability.transferWizard.passphraseMismatch");
    return false;
  }
  return true;
}

async function onCreateTransferSession() {
  clearStatus();
  if (!targetOrigin.value.trim()) {
    error.value = t("components.identityPortability.transferWizard.missingTargetOrigin");
    return;
  }
  if (!validatePassphrase()) return;
  busy.value = true;
  try {
    const [bundle, transfer] = await Promise.all([
      exportSecureIdentityBundle(passphrase.value, targetOrigin.value.trim()),
      createIdentityTransferSession({
        target_origin: targetOrigin.value.trim(),
        include_private: includePrivate.value,
        include_gated: includeGated.value,
      }),
    ]);
    secureBundleText.value = JSON.stringify(bundle, null, 2);
    sourceSessionID.value = transfer.session.id;
    transferToken.value = transfer.token;
    message.value = t("components.identityPortability.transferWizard.sessionCreated");
  } catch (err) {
    setError(err);
  } finally {
    busy.value = false;
  }
}

async function onImportSecureBundle() {
  clearStatus();
  let parsed: IdentityBundle;
  try {
    parsed = JSON.parse(secureBundleText.value) as IdentityBundle;
  } catch {
    error.value = t("components.identityPortability.invalidJson");
    return;
  }
  if (!passphrase.value.trim()) {
    error.value = t("components.identityPortability.transferWizard.missingPassphrase");
    return;
  }
  busy.value = true;
  try {
    await importSecureIdentityBundle(parsed, passphrase.value);
    message.value = t("components.identityPortability.imported");
  } catch (err) {
    setError(err);
  } finally {
    busy.value = false;
  }
}

async function onCreateImportJob() {
  clearStatus();
  if (!sourceOrigin.value.trim() || !sourceSessionID.value.trim() || !transferToken.value.trim()) {
    error.value = t("components.identityPortability.transferWizard.missingSource");
    return;
  }
  busy.value = true;
  try {
    job.value = await createIdentityImportJob({
      source_origin: sourceOrigin.value.trim(),
      target_origin: currentOrigin() || targetOrigin.value.trim(),
      source_session_id: sourceSessionID.value.trim(),
      token: transferToken.value.trim(),
      include_private: includePrivate.value,
      include_gated: includeGated.value,
    });
    startPollingJob();
    message.value = t("components.identityPortability.transferWizard.jobStarted");
  } catch (err) {
    setError(err);
  } finally {
    busy.value = false;
  }
}

function startPollingJob() {
  if (pollTimer) window.clearInterval(pollTimer);
  if (!job.value) return;
  pollTimer = window.setInterval(async () => {
    if (!job.value) return;
    try {
      job.value = await getIdentityImportJob(job.value.id);
      if (["completed", "failed", "cancelled"].includes(job.value.status) && pollTimer) {
        window.clearInterval(pollTimer);
        pollTimer = undefined;
      }
    } catch {
      /* Keep the current job snapshot visible. */
    }
  }, 3000);
}

async function onRetryJob() {
  if (!job.value) return;
  clearStatus();
  busy.value = true;
  try {
    await retryIdentityImportJob(job.value.id);
    job.value = await getIdentityImportJob(job.value.id);
    startPollingJob();
  } catch (err) {
    setError(err);
  } finally {
    busy.value = false;
  }
}

async function onCancelJob() {
  if (!job.value) return;
  clearStatus();
  busy.value = true;
  try {
    await cancelIdentityImportJob(job.value.id);
    job.value = await getIdentityImportJob(job.value.id);
  } catch (err) {
    setError(err);
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
    setError(err);
  } finally {
    busy.value = false;
  }
}

onBeforeUnmount(() => {
  if (pollTimer) window.clearInterval(pollTimer);
});
</script>

<template>
  <div class="mt-3 space-y-5 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
    <div>
      <p class="text-sm font-medium text-neutral-900">{{ $t("components.identityPortability.title") }}</p>
      <p class="mt-1 text-xs leading-5 text-neutral-500">
        {{ $t("components.identityPortability.description") }}
      </p>
    </div>

    <section class="space-y-3 rounded-2xl border border-lime-100 bg-lime-50/40 p-4">
      <div>
        <p class="text-sm font-semibold text-neutral-900">
          {{ $t("components.identityPortability.transferWizard.title") }}
        </p>
        <p class="mt-1 text-xs leading-5 text-neutral-600">
          {{ $t("components.identityPortability.transferWizard.description") }}
        </p>
        <p class="mt-2 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800">
          {{ $t("components.identityPortability.transferWizard.securityNotice") }}
        </p>
      </div>

      <div v-if="message || error" class="rounded-xl border px-3 py-2 text-xs"
        :class="error ? 'border-red-200 bg-red-50 text-red-700' : 'border-lime-200 bg-lime-50 text-lime-800'"
      >
        {{ error || message }}
      </div>

      <div class="grid gap-3 md:grid-cols-2">
        <label class="block text-xs font-medium text-neutral-700">
          {{ $t("components.identityPortability.transferWizard.targetOrigin") }}
          <input
            v-model="targetOrigin"
            class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
            placeholder="https://new.example"
          />
        </label>
        <label class="block text-xs font-medium text-neutral-700">
          {{ $t("components.identityPortability.transferWizard.sourceOrigin") }}
          <input
            v-model="sourceOrigin"
            class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
            placeholder="https://old.example"
          />
          <span v-if="sourceOriginLooksCurrent" class="mt-1 block text-xs text-amber-700">
            {{ $t("components.identityPortability.transferWizard.sourceOriginCurrentWarning") }}
          </span>
        </label>
      </div>

      <div class="grid gap-3 md:grid-cols-2">
        <label class="block text-xs font-medium text-neutral-700">
          {{ $t("components.identityPortability.transferWizard.passphrase") }}
          <input
            v-model="passphrase"
            :type="showPassphrase ? 'text' : 'password'"
            class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
          />
          <span v-if="passphraseTooShort" class="mt-1 block text-xs text-red-600">
            {{ $t("components.identityPortability.transferWizard.weakPassphrase") }}
          </span>
        </label>
        <label class="block text-xs font-medium text-neutral-700">
          {{ $t("components.identityPortability.transferWizard.passphraseConfirm") }}
          <input
            v-model="passphraseConfirm"
            :type="showPassphrase ? 'text' : 'password'"
            class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
          />
          <span v-if="passphraseMismatch" class="mt-1 block text-xs text-red-600">
            {{ $t("components.identityPortability.transferWizard.passphraseMismatch") }}
          </span>
        </label>
      </div>

      <label class="flex items-center gap-2 text-xs text-neutral-600">
        <input v-model="showPassphrase" type="checkbox" class="rounded border-neutral-300" />
        {{ $t("components.identityPortability.transferWizard.showPassphrase") }}
      </label>

      <div class="flex flex-wrap gap-4 text-xs text-neutral-700">
        <label class="flex items-center gap-2">
          <input v-model="includePrivate" type="checkbox" class="rounded border-neutral-300" />
          {{ $t("components.identityPortability.transferWizard.includePrivate") }}
        </label>
        <label class="flex items-center gap-2">
          <input v-model="includeGated" type="checkbox" class="rounded border-neutral-300" />
          {{ $t("components.identityPortability.transferWizard.includeGated") }}
        </label>
      </div>

      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="rounded-xl bg-neutral-900 px-4 py-2 text-sm font-semibold text-white transition hover:bg-neutral-700 disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="!canCreateTransferSession"
          @click="onCreateTransferSession"
        >
          {{
            busy
              ? $t("components.identityPortability.transferWizard.creatingSession")
              : $t("components.identityPortability.transferWizard.createSession")
          }}
        </button>
        <button
          type="button"
          class="rounded-xl border border-neutral-300 bg-white px-4 py-2 text-sm font-semibold text-neutral-900 transition hover:bg-neutral-50 disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="busy || !secureBundleText.trim()"
          @click="onImportSecureBundle"
        >
          {{ $t("components.identityPortability.transferWizard.importSecureBundle") }}
        </button>
        <button
          type="button"
          class="rounded-xl border border-neutral-300 bg-white px-4 py-2 text-sm font-semibold text-neutral-900 transition hover:bg-neutral-50 disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="busy"
          @click="onCreateImportJob"
        >
          {{ $t("components.identityPortability.transferWizard.startImport") }}
        </button>
      </div>

      <div class="grid gap-3 md:grid-cols-2">
        <label class="block text-xs font-medium text-neutral-700">
          {{ $t("components.identityPortability.transferWizard.sessionId") }}
          <input
            v-model="sourceSessionID"
            class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 font-mono text-xs text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
          />
        </label>
        <label class="block text-xs font-medium text-neutral-700">
          {{ $t("components.identityPortability.transferWizard.transferToken") }}
          <input
            v-model="transferToken"
            class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 font-mono text-xs text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
          />
        </label>
      </div>

      <textarea
        v-model="secureBundleText"
        class="min-h-32 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 font-mono text-xs text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
        :placeholder="$t('components.identityPortability.transferWizard.secureBundlePlaceholder')"
      ></textarea>
      <p v-if="secureBundleOriginWarning" class="text-xs text-amber-700">{{ secureBundleOriginWarning }}</p>

      <div v-if="job" class="space-y-2 rounded-xl border border-neutral-200 bg-white p-3">
        <div class="flex items-center justify-between text-xs text-neutral-600">
          <span>{{ $t("components.identityPortability.transferWizard.jobStatus") }}: {{ job.status }}</span>
          <span>{{ jobProgressLabel }}</span>
        </div>
        <div class="h-2 overflow-hidden rounded-full bg-neutral-100">
          <div class="h-full bg-lime-500 transition-all" :style="{ width: `${progressPct}%` }"></div>
        </div>
        <div v-if="jobStatsSummary.length" class="grid gap-1 text-xs text-neutral-600 sm:grid-cols-2">
          <div v-for="item in jobStatsSummary" :key="item.key" class="flex justify-between gap-2 rounded-lg bg-neutral-50 px-2 py-1">
            <span>{{ item.label }}</span>
            <span>{{ item.value }}</span>
          </div>
        </div>
        <p v-if="visibleJobError" class="text-xs text-red-600">{{ visibleJobError }}</p>
        <div class="flex gap-2">
          <button
            type="button"
            class="rounded-xl border border-neutral-300 px-3 py-1.5 text-xs font-semibold text-neutral-900 transition hover:bg-neutral-50 disabled:opacity-60"
            :disabled="busy || job.status !== 'failed'"
            @click="onRetryJob"
          >
            {{ $t("components.identityPortability.transferWizard.retry") }}
          </button>
          <button
            type="button"
            class="rounded-xl border border-neutral-300 px-3 py-1.5 text-xs font-semibold text-neutral-900 transition hover:bg-neutral-50 disabled:opacity-60"
            :disabled="busy || ['completed', 'cancelled'].includes(job.status)"
            @click="onCancelJob"
          >
            {{ $t("components.identityPortability.transferWizard.cancel") }}
          </button>
        </div>
      </div>
    </section>

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
