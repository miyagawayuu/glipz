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
  isValidIdentityMigrationFile,
  retryIdentityImportJob,
  type IdentityMigrationFile,
  type IdentityTransferImportJob,
} from "../lib/identityPortability";

type WizardStep = "source" | "upload" | "review" | "import" | "complete";
type MigrationRole = "source" | "target";

const { t } = useI18n();
const migrationFileMaxBytes = 2 * 1024 * 1024;

const movedToAcct = ref("");
const busy = ref(false);
const message = ref("");
const error = ref("");
const currentStep = ref<WizardStep>("source");
const selectedRole = ref<MigrationRole | null>(null);

const targetOrigin = ref("");
const passphrase = ref("");
const passphraseConfirm = ref("");
const importPassphrase = ref("");
const showPassphrase = ref(false);
const includePrivate = ref(false);
const includeGated = ref(false);
const migrationFile = ref<IdentityMigrationFile | null>(null);
const uploadedFileName = ref("");
const identityImported = ref(false);
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

const wizardSteps = computed(() => ([
  { key: "source", label: t("components.identityPortability.transferWizard.steps.source") },
  { key: "upload", label: t("components.identityPortability.transferWizard.steps.upload") },
  { key: "review", label: t("components.identityPortability.transferWizard.steps.review") },
  { key: "import", label: t("components.identityPortability.transferWizard.steps.import") },
  { key: "complete", label: t("components.identityPortability.transferWizard.steps.complete") },
] as Array<{ key: WizardStep; label: string }>));

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
const canCreateMigrationFile = computed(() =>
  !busy.value
  && targetOrigin.value.trim().length > 0
  && passphrase.value.trim().length >= 12
  && passphrase.value === passphraseConfirm.value,
);
const migrationFileExpired = computed(() => {
  if (!migrationFile.value?.expires_at) return false;
  const expires = new Date(migrationFile.value.expires_at).getTime();
  return Number.isFinite(expires) && expires <= Date.now();
});
const canImportIdentity = computed(() =>
  !busy.value
  && !!migrationFile.value
  && !identityImported.value
  && importPassphrase.value.trim().length > 0,
);
const canStartImportJob = computed(() =>
  !busy.value
  && !!migrationFile.value
  && identityImported.value
  && !migrationFileExpired.value
  && !job.value,
);
const sourceOriginLooksCurrent = computed(() =>
  !!migrationFile.value
  && currentOrigin().length > 0
  && normalizeOrigin(migrationFile.value.source_origin) === normalizeOrigin(currentOrigin()),
);
const secureBundleOriginWarning = computed(() => {
  const expected = migrationFile.value?.bundle.created_for_origin;
  if (!expected || !currentOrigin()) return "";
  if (normalizeOrigin(expected) !== normalizeOrigin(currentOrigin())) {
    return t("components.identityPortability.transferWizard.bundleOriginWarning", {
      expected,
      actual: currentOrigin(),
    });
  }
  return "";
});
const visibleJobError = computed(() => {
  const lastError = job.value?.last_error?.trim() ?? "";
  if (lastError.includes("source status 401")) {
    return t("components.identityPortability.transferWizard.sourceUnauthorizedHint");
  }
  return lastError;
});
const migrationSummary = computed(() => {
  if (!migrationFile.value) return [];
  return [
    { label: t("components.identityPortability.transferWizard.sourceOrigin"), value: migrationFile.value.source_origin },
    { label: t("components.identityPortability.transferWizard.targetOrigin"), value: migrationFile.value.target_origin },
    { label: t("components.identityPortability.transferWizard.expiresAt"), value: formatDateTime(migrationFile.value.expires_at) },
    { label: t("components.identityPortability.transferWizard.migrationFileIncludes"), value: migrationIncludesLabel() },
  ];
});

function clearStatus() {
  message.value = "";
  error.value = "";
}

function setError(err: unknown) {
  const msg = err instanceof Error ? err.message : "";
  if (msg === "unauthorized") {
    error.value = t("components.identityPortability.transferWizard.loginRequired");
  } else if (msg === "wrong_passphrase") {
    error.value = t("components.identityPortability.transferWizard.wrongPassphrase");
  } else if (msg === "invalid_identity_bundle") {
    error.value = t("components.identityPortability.transferWizard.invalidIdentityBundle");
  } else {
    error.value = msg || t("components.identityPortability.failed");
  }
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

function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function migrationIncludesLabel(): string {
  if (!migrationFile.value) return "";
  const parts = [t("components.identityPortability.transferWizard.includesStandard")];
  if (migrationFile.value.include_private) parts.push(t("components.identityPortability.transferWizard.includePrivate"));
  if (migrationFile.value.include_gated) parts.push(t("components.identityPortability.transferWizard.includeGated"));
  return parts.join(" / ");
}

function migrationFileName(file: IdentityMigrationFile): string {
  const handle = file.bundle.handle.replace(/[^a-z0-9_-]+/gi, "-").replace(/^-+|-+$/g, "") || "account";
  return `glipz-migration-${handle}-${new Date().toISOString().slice(0, 10)}.json`;
}

function downloadMigrationFile(file: IdentityMigrationFile) {
  if (typeof document === "undefined" || typeof URL === "undefined") return;
  const blob = new Blob([JSON.stringify(file, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = migrationFileName(file);
  link.rel = "noopener";
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function clearMigrationFile() {
  migrationFile.value = null;
  uploadedFileName.value = "";
  identityImported.value = false;
  importPassphrase.value = "";
  if (!job.value) currentStep.value = "upload";
}

function selectRole(role: MigrationRole) {
  clearStatus();
  selectedRole.value = role;
  currentStep.value = role === "source" ? "source" : migrationFile.value ? "review" : "upload";
}

function backToRoleChoice() {
  clearStatus();
  selectedRole.value = null;
}

async function onCreateMigrationFile() {
  clearStatus();
  if (!targetOrigin.value.trim()) {
    error.value = t("components.identityPortability.transferWizard.missingTargetOrigin");
    return;
  }
  if (!currentOrigin()) {
    error.value = t("components.identityPortability.transferWizard.sourceOriginUnavailable");
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
    const file: IdentityMigrationFile = {
      v: 1,
      kind: "glipz_identity_migration",
      source_origin: currentOrigin(),
      target_origin: transfer.session.allowed_target_origin || normalizeOrigin(targetOrigin.value),
      created_at: new Date().toISOString(),
      expires_at: transfer.session.expires_at,
      session_id: transfer.session.id,
      token: transfer.token,
      include_private: transfer.session.include_private,
      include_gated: transfer.session.include_gated,
      bundle,
    };
    migrationFile.value = null;
    identityImported.value = false;
    downloadMigrationFile(file);
    passphrase.value = "";
    passphraseConfirm.value = "";
    message.value = t("components.identityPortability.transferWizard.fileCreated");
    currentStep.value = "upload";
    selectedRole.value = null;
  } catch (err) {
    setError(err);
  } finally {
    busy.value = false;
  }
}

async function onMigrationFileChange(event: Event) {
  clearStatus();
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  if (file.size > migrationFileMaxBytes) {
    error.value = t("components.identityPortability.transferWizard.fileTooLarge");
    return;
  }
  try {
    const parsed = JSON.parse(await file.text()) as unknown;
    if (!isValidIdentityMigrationFile(parsed)) {
      error.value = t("components.identityPortability.transferWizard.invalidMigrationFile");
      return;
    }
    migrationFile.value = parsed;
    uploadedFileName.value = file.name;
    targetOrigin.value = parsed.target_origin;
    includePrivate.value = parsed.include_private;
    includeGated.value = parsed.include_gated;
    identityImported.value = false;
    job.value = null;
    selectedRole.value = "target";
    currentStep.value = "review";
    message.value = t("components.identityPortability.transferWizard.fileLoaded");
  } catch {
    error.value = t("components.identityPortability.invalidJson");
  }
}

async function onImportSecureBundle() {
  clearStatus();
  if (!migrationFile.value) {
    error.value = t("components.identityPortability.transferWizard.invalidMigrationFile");
    return;
  }
  if (!importPassphrase.value.trim()) {
    error.value = t("components.identityPortability.transferWizard.missingPassphrase");
    return;
  }
  busy.value = true;
  try {
    await importSecureIdentityBundle(migrationFile.value.bundle, importPassphrase.value);
    identityImported.value = true;
    importPassphrase.value = "";
    currentStep.value = "import";
    message.value = t("components.identityPortability.transferWizard.identityImported");
  } catch (err) {
    setError(err);
  } finally {
    busy.value = false;
  }
}

async function onCreateImportJob() {
  clearStatus();
  if (!migrationFile.value) {
    error.value = t("components.identityPortability.transferWizard.invalidMigrationFile");
    return;
  }
  if (migrationFileExpired.value) {
    error.value = t("components.identityPortability.transferWizard.fileExpired");
    return;
  }
  busy.value = true;
  try {
    const file = migrationFile.value;
    job.value = await createIdentityImportJob({
      source_origin: file.source_origin,
      target_origin: currentOrigin() || file.target_origin,
      source_session_id: file.session_id,
      token: file.token,
      include_private: file.include_private,
      include_gated: file.include_gated,
    });
    migrationFile.value = null;
    uploadedFileName.value = "";
    startPollingJob();
    currentStep.value = "complete";
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

    <section class="space-y-4 rounded-2xl border border-lime-100 bg-lime-50/40 p-4">
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

      <ol v-if="selectedRole" class="grid gap-2 text-xs sm:grid-cols-5">
        <li
          v-for="(step, index) in wizardSteps"
          :key="step.key"
          class="rounded-xl border px-3 py-2"
          :class="step.key === currentStep ? 'border-lime-300 bg-white text-lime-800' : 'border-neutral-200 bg-white/60 text-neutral-500'"
        >
          <span class="font-semibold">{{ index + 1 }}.</span>
          {{ step.label }}
        </li>
      </ol>

      <div v-if="message || error" class="rounded-xl border px-3 py-2 text-xs"
        :class="error ? 'border-red-200 bg-red-50 text-red-700' : 'border-lime-200 bg-lime-50 text-lime-800'"
      >
        {{ error || message }}
      </div>

      <div v-if="!selectedRole" class="space-y-3 rounded-2xl border border-neutral-200 bg-white p-4">
        <p class="text-sm font-semibold text-neutral-900">
          {{ $t("components.identityPortability.transferWizard.roleChoiceTitle") }}
        </p>
        <p class="text-xs leading-5 text-neutral-500">
          {{ $t("components.identityPortability.transferWizard.roleChoiceDescription") }}
        </p>
        <div class="grid gap-3 md:grid-cols-2">
          <button
            type="button"
            class="rounded-2xl border border-neutral-200 bg-neutral-50 p-4 text-left transition hover:border-lime-300 hover:bg-lime-50"
            @click="selectRole('source')"
          >
            <span class="block text-sm font-semibold text-neutral-900">
              {{ $t("components.identityPortability.transferWizard.sourceCardTitle") }}
            </span>
            <span class="mt-1 block text-xs leading-5 text-neutral-500">
              {{ $t("components.identityPortability.transferWizard.sourceCardDescription") }}
            </span>
          </button>
          <button
            type="button"
            class="rounded-2xl border border-neutral-200 bg-neutral-50 p-4 text-left transition hover:border-lime-300 hover:bg-lime-50"
            @click="selectRole('target')"
          >
            <span class="block text-sm font-semibold text-neutral-900">
              {{ $t("components.identityPortability.transferWizard.targetCardTitle") }}
            </span>
            <span class="mt-1 block text-xs leading-5 text-neutral-500">
              {{ $t("components.identityPortability.transferWizard.targetCardDescription") }}
            </span>
          </button>
        </div>
      </div>

      <div v-else class="space-y-4">
        <button
          type="button"
          class="text-xs font-semibold text-neutral-600 transition hover:text-neutral-900"
          @click="backToRoleChoice"
        >
          {{ $t("components.identityPortability.transferWizard.changeRole") }}
        </button>

        <div v-if="selectedRole === 'source'" class="space-y-3 rounded-2xl border border-neutral-200 bg-white p-4">
          <div>
            <p class="text-sm font-semibold text-neutral-900">
              {{ $t("components.identityPortability.transferWizard.sourceCardTitle") }}
            </p>
            <p class="mt-1 text-xs leading-5 text-neutral-500">
              {{ $t("components.identityPortability.transferWizard.sourceCardDescription") }}
            </p>
          </div>

          <label class="block text-xs font-medium text-neutral-700">
            {{ $t("components.identityPortability.transferWizard.targetOrigin") }}
            <input
              v-model="targetOrigin"
              class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              placeholder="https://new.example"
            />
          </label>

          <div class="grid gap-3 md:grid-cols-2">
            <label class="block text-xs font-medium text-neutral-700">
              {{ $t("components.identityPortability.transferWizard.passphrase") }}
              <input
                v-model="passphrase"
                :type="showPassphrase ? 'text' : 'password'"
                autocomplete="new-password"
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
                autocomplete="new-password"
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

          <button
            type="button"
            class="rounded-xl bg-neutral-900 px-4 py-2 text-sm font-semibold text-white transition hover:bg-neutral-700 disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="!canCreateMigrationFile"
            @click="onCreateMigrationFile"
          >
            {{
              busy
                ? $t("components.identityPortability.transferWizard.creatingSession")
                : $t("components.identityPortability.transferWizard.createMigrationFile")
            }}
          </button>
        </div>

        <div v-if="selectedRole === 'target'" class="space-y-3 rounded-2xl border border-neutral-200 bg-white p-4">
          <div>
            <p class="text-sm font-semibold text-neutral-900">
              {{ $t("components.identityPortability.transferWizard.targetCardTitle") }}
            </p>
            <p class="mt-1 text-xs leading-5 text-neutral-500">
              {{ $t("components.identityPortability.transferWizard.targetCardDescription") }}
            </p>
          </div>

          <label class="block rounded-2xl border border-dashed border-neutral-300 bg-neutral-50 p-4 text-center text-sm font-semibold text-neutral-900 transition hover:bg-neutral-100">
            <input class="sr-only" type="file" accept="application/json,.json" @change="onMigrationFileChange" />
            {{ uploadedFileName || $t("components.identityPortability.transferWizard.chooseMigrationFile") }}
          </label>

          <div v-if="migrationFile" class="space-y-2 rounded-xl border border-neutral-200 bg-neutral-50 p-3 text-xs text-neutral-600">
            <div v-for="item in migrationSummary" :key="item.label" class="flex flex-col gap-1 sm:flex-row sm:justify-between">
              <span class="font-medium text-neutral-700">{{ item.label }}</span>
              <span class="break-all sm:text-right">{{ item.value }}</span>
            </div>
          </div>

          <p v-if="sourceOriginLooksCurrent" class="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800">
            {{ $t("components.identityPortability.transferWizard.sourceOriginCurrentWarning") }}
          </p>
          <p v-if="secureBundleOriginWarning" class="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800">
            {{ secureBundleOriginWarning }}
          </p>
          <p v-if="migrationFileExpired" class="rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs leading-5 text-red-700">
            {{ $t("components.identityPortability.transferWizard.fileExpired") }}
          </p>

          <label v-if="migrationFile && !identityImported" class="block text-xs font-medium text-neutral-700">
            {{ $t("components.identityPortability.transferWizard.passphrase") }}
            <input
              v-model="importPassphrase"
              :type="showPassphrase ? 'text' : 'password'"
              autocomplete="current-password"
              class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
            />
          </label>

          <div class="flex flex-wrap gap-2">
            <button
              v-if="migrationFile && !identityImported"
              type="button"
              class="rounded-xl bg-neutral-900 px-4 py-2 text-sm font-semibold text-white transition hover:bg-neutral-700 disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="!canImportIdentity"
              @click="onImportSecureBundle"
            >
              {{ $t("components.identityPortability.transferWizard.importSecureBundle") }}
            </button>
            <button
              v-if="migrationFile && identityImported"
              type="button"
              class="rounded-xl bg-lime-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-lime-700 disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="!canStartImportJob"
              @click="onCreateImportJob"
            >
              {{ $t("components.identityPortability.transferWizard.startImport") }}
            </button>
            <button
              v-if="migrationFile"
              type="button"
              class="rounded-xl border border-neutral-300 bg-white px-4 py-2 text-sm font-semibold text-neutral-900 transition hover:bg-neutral-50 disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="busy"
              @click="clearMigrationFile"
            >
              {{ $t("components.identityPortability.transferWizard.clearMigrationFile") }}
            </button>
          </div>
        </div>
      </div>

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
