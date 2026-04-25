<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { formatDateTime } from "../i18n";
import { api, clearStoredApiBase, readStoredApiBase, writeStoredApiBase } from "../lib/api";
import { fetchPublicInstanceSettings } from "../lib/instanceSettings";
import { legalDocumentLink, type LegalDocumentKey, type LegalDocumentURLSettings } from "../lib/legalDocumentLinks";
import { isNativeApp } from "../lib/runtime";

const { t } = useI18n();
const showApiBaseInput = isNativeApp();
const apiBaseInput = ref(readStoredApiBase() || "");
const apiBaseError = ref("");
const email = ref("");
const password = ref("");
const passwordConfirm = ref("");
const handle = ref("");
const birthDate = ref("");
const termsAgreed = ref(false);
const err = ref("");
const loading = ref(false);
const submitted = ref(false);
const expiresAt = ref("");
const handleTouched = ref(false);
const handleBusy = ref(false);
const legalDocumentUrls = ref<LegalDocumentURLSettings>({});
const handleAvailability = ref<null | { available: boolean; reason: string; normalized: string }>(null);
let handleTimer: ReturnType<typeof setTimeout> | null = null;
/** Monotonic validation generation used to discard stale fetch results from blur, debounce, or delayed typing responses. */
let handleCheckRun = 0;
let handleFetchInflight = 0;

function normalizeHandleInput(raw: string): string {
  return raw.trim().toLowerCase().replace(/^@+/, "");
}

function validateHandleClient(h: string): string | null {
  if (!h) return t("auth.register.errors.handleRequired");
  if (h.length > 30) return t("auth.register.errors.handleTooLong");
  if (!/^[a-z0-9_]+$/.test(h)) return t("auth.register.errors.handleChars");
  return null;
}

function handleReasonMessage(reason: string): string {
  switch (reason) {
    case "invalid_handle":
      return t("auth.register.errors.invalidHandle");
    case "reserved_handle":
      return t("auth.register.errors.reservedHandle");
    case "handle_taken":
      return t("auth.register.errors.handleTaken");
    default:
      return "";
  }
}

const normalizedHandle = computed(() => normalizeHandleInput(handle.value));
const handleClientError = computed(() => validateHandleClient(normalizedHandle.value));
const passwordConfirmError = computed(() => {
  if (!passwordConfirm.value) return t("auth.register.errors.confirmPasswordRequired");
  if (password.value !== passwordConfirm.value) return t("auth.register.errors.confirmPasswordMismatch");
  return null;
});
const birthDateError = computed(() => {
  if (!birthDate.value) return t("auth.register.errors.birthDateRequired");
  const d = new Date(`${birthDate.value}T00:00:00Z`);
  if (Number.isNaN(d.getTime())) return t("auth.register.errors.birthDateInvalid");
  if (d.getTime() > Date.now()) return t("auth.register.errors.birthDateFuture");
  const minBirthDate = new Date();
  minBirthDate.setUTCFullYear(minBirthDate.getUTCFullYear() - 13);
  if (d.getTime() > minBirthDate.getTime()) return t("auth.register.errors.underAge");
  return null;
});
const canSubmit = computed(() =>
  !loading.value
  && !handleBusy.value
  && !handleClientError.value
  && !passwordConfirmError.value
  && !birthDateError.value
  && termsAgreed.value
  && handleAvailability.value?.available === true,
);

function saveApiBase() {
  if (!showApiBaseInput) return;
  apiBaseError.value = "";
  try {
    const normalized = writeStoredApiBase(apiBaseInput.value);
    apiBaseInput.value = normalized;
  } catch {
    apiBaseError.value = t("auth.apiBase.invalid");
  }
}

function resetApiBase() {
  if (!showApiBaseInput) return;
  apiBaseError.value = "";
  clearStoredApiBase();
  apiBaseInput.value = "";
}

function legalLink(key: LegalDocumentKey): { href: string; external: boolean } {
  return legalDocumentLink(legalDocumentUrls.value, key);
}

async function loadLegalDocumentUrls() {
  try {
    legalDocumentUrls.value = await fetchPublicInstanceSettings();
  } catch {
    legalDocumentUrls.value = {};
  }
}

async function checkHandleAvailability() {
  handleTouched.value = true;
  const wave = ++handleCheckRun;
  handleAvailability.value = null;
  const h = normalizedHandle.value;
  const clientErr = validateHandleClient(h);
  if (clientErr) {
    return;
  }
  handleFetchInflight++;
  handleBusy.value = true;
  try {
    const res = await api<{ handle: string; available: boolean; reason?: string }>("/api/v1/auth/handle-availability?handle=" + encodeURIComponent(h), {
      method: "GET",
    });
    if (wave !== handleCheckRun) {
      return;
    }
    handleAvailability.value = {
      available: Boolean(res.available),
      reason: typeof res.reason === "string" ? res.reason : "",
      normalized: typeof res.handle === "string" ? res.handle : h,
    };
  } catch {
    if (wave !== handleCheckRun) {
      return;
    }
    handleAvailability.value = { available: false, reason: "check_failed", normalized: h };
  } finally {
    handleFetchInflight--;
    handleBusy.value = handleFetchInflight > 0;
  }
}

watch(handle, () => {
  handleAvailability.value = null;
  if (handleTimer) clearTimeout(handleTimer);
  handleTimer = setTimeout(() => {
    void checkHandleAvailability();
  }, 450);
});

onBeforeUnmount(() => {
  if (handleTimer) clearTimeout(handleTimer);
});

onMounted(() => {
  void loadLegalDocumentUrls();
});

function messageForError(error: unknown): string {
  if (!(error instanceof Error)) {
    return t("auth.register.errors.registerFailed");
  }
  switch (error.message) {
    case "email_taken":
      return t("auth.register.errors.emailTaken");
    case "invalid_email_or_password":
      return t("auth.register.errors.invalidEmailOrPassword");
    case "invalid_handle":
      return t("auth.register.errors.invalidHandle");
    case "reserved_handle":
      return t("auth.register.errors.reservedHandle");
    case "handle_taken":
      return t("auth.register.errors.handleTaken");
    case "password_mismatch":
      return t("auth.register.errors.passwordMismatch");
    case "invalid_birth_date":
      return t("auth.register.errors.birthDateInvalid");
    case "under_age":
      return t("auth.register.errors.underAge");
    case "terms_not_agreed":
      return t("auth.register.errors.termsNotAgreed");
    case "registrations_disabled":
      return t("auth.register.errors.registrationsDisabled");
    default:
      return t("auth.register.errors.registerFailed");
  }
}

async function submit() {
  err.value = "";
  saveApiBase();
  await checkHandleAvailability();
  if (handleClientError.value) {
    err.value = handleClientError.value;
    return;
  }
  if (!handleAvailability.value?.available) {
    err.value = handleReasonMessage(handleAvailability.value?.reason ?? "") || t("auth.register.errors.reviewHandle");
    return;
  }
  if (passwordConfirmError.value) {
    err.value = passwordConfirmError.value;
    return;
  }
  if (birthDateError.value) {
    err.value = birthDateError.value;
    return;
  }
  if (!termsAgreed.value) {
    err.value = t("auth.register.errors.termsNotAgreed");
    return;
  }
  loading.value = true;
  try {
    const res = await api<{ expires_at: string }>("/api/v1/auth/register", {
      method: "POST",
      json: {
        email: email.value,
        password: password.value,
        password_confirm: passwordConfirm.value,
        handle: normalizedHandle.value,
        birth_date: birthDate.value,
        terms_agreed: termsAgreed.value,
      },
    });
    submitted.value = true;
    expiresAt.value = res.expires_at;
  } catch (e: unknown) {
    err.value = messageForError(e);
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="mx-auto max-w-md space-y-6">
    <div v-if="!submitted">
      <h1 class="text-2xl font-semibold text-neutral-900">{{ $t("auth.register.title") }}</h1>
      <p class="mt-1 text-sm text-neutral-600">
        {{ $t("auth.register.description") }}
      </p>
    </div>
    <form v-if="!submitted" class="space-y-4" @submit.prevent="submit">
      <div v-if="showApiBaseInput" class="rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-3">
        <label class="block text-sm font-medium text-neutral-700">{{ $t("auth.apiBase.label") }}</label>
        <input
          v-model="apiBaseInput"
          type="text"
          inputmode="url"
          autocomplete="off"
          spellcheck="false"
          class="mt-1 w-full rounded-md border border-lime-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
          :placeholder="$t('auth.apiBase.placeholder')"
          @blur="saveApiBase"
        />
        <p class="mt-1 text-xs text-neutral-500">{{ $t("auth.apiBase.hint") }}</p>
        <p v-if="apiBaseError" class="mt-1 text-sm text-red-600">{{ apiBaseError }}</p>
        <div class="mt-2 flex items-center justify-end gap-2">
          <button
            type="button"
            class="rounded-md border border-neutral-200 bg-white px-3 py-1.5 text-xs font-medium text-neutral-700 hover:bg-neutral-50"
            @click="resetApiBase"
          >
            {{ $t("auth.apiBase.reset") }}
          </button>
          <button
            type="button"
            class="rounded-md bg-lime-500 px-3 py-1.5 text-xs font-medium text-white hover:bg-lime-600"
            @click="saveApiBase"
          >
            {{ $t("auth.apiBase.apply") }}
          </button>
        </div>
      </div>
      <div>
        <label class="block text-sm font-medium text-neutral-700">{{ $t("auth.register.email") }}</label>
        <input
          v-model="email"
          type="email"
          required
          autocomplete="email"
          class="mt-1 w-full rounded-md border border-lime-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        />
      </div>
      <div>
        <label class="block text-sm font-medium text-neutral-700">{{ $t("auth.register.handle") }}</label>
        <input
          v-model="handle"
          type="text"
          required
          autocapitalize="off"
          autocomplete="off"
          spellcheck="false"
          maxlength="30"
          class="mt-1 w-full rounded-md border border-lime-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
          :placeholder="$t('auth.register.handlePlaceholder')"
          @blur="checkHandleAvailability"
        />
        <p class="mt-1 text-xs text-neutral-500">{{ $t("auth.register.handleHint") }}</p>
        <p v-if="handleTouched && handleClientError" class="mt-1 text-sm text-red-600">{{ handleClientError }}</p>
        <p v-else-if="handleBusy" class="mt-1 text-sm text-neutral-500">{{ $t("auth.register.handleChecking") }}</p>
        <p v-else-if="handleAvailability?.available" class="mt-1 text-sm text-lime-700">{{ $t("auth.register.handleAvailable") }}</p>
        <p
          v-else-if="handleTouched && handleAvailability && !handleAvailability.available"
          class="mt-1 text-sm text-red-600"
        >
          {{ handleReasonMessage(handleAvailability.reason) || $t("auth.register.handleCheckFailed") }}
        </p>
      </div>
      <div>
        <label class="block text-sm font-medium text-neutral-700">{{ $t("auth.register.password") }}</label>
        <input
          v-model="password"
          type="password"
          required
          minlength="8"
          autocomplete="new-password"
          class="mt-1 w-full rounded-md border border-lime-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        />
      </div>
      <div>
        <label class="block text-sm font-medium text-neutral-700">{{ $t("auth.register.passwordConfirm") }}</label>
        <input
          v-model="passwordConfirm"
          type="password"
          required
          minlength="8"
          autocomplete="new-password"
          class="mt-1 w-full rounded-md border border-lime-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        />
        <p v-if="passwordConfirm && passwordConfirmError" class="mt-1 text-sm text-red-600">{{ passwordConfirmError }}</p>
      </div>
      <div>
        <label class="block text-sm font-medium text-neutral-700">{{ $t("auth.register.birthDate") }}</label>
        <input
          v-model="birthDate"
          type="date"
          required
          max="9999-12-31"
          class="mt-1 w-full rounded-md border border-lime-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        />
        <p v-if="birthDate && birthDateError" class="mt-1 text-sm text-red-600">{{ birthDateError }}</p>
      </div>
      <div class="rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-3 text-sm text-neutral-700">
        <p class="leading-relaxed">
          {{ $t("auth.register.preface") }}
          <a
            v-if="legalLink('terms').external"
            :href="legalLink('terms').href"
            target="_blank"
            rel="noopener noreferrer"
            class="font-medium text-lime-700 hover:text-lime-800"
          >
            {{ $t("app.links.terms") }}
          </a>
          <RouterLink v-else :to="legalLink('terms').href" class="font-medium text-lime-700 hover:text-lime-800">
            {{ $t("app.links.terms") }}
          </RouterLink>
          /
          <a
            v-if="legalLink('privacy').external"
            :href="legalLink('privacy').href"
            target="_blank"
            rel="noopener noreferrer"
            class="font-medium text-lime-700 hover:text-lime-800"
          >
            {{ $t("app.links.privacy") }}
          </a>
          <RouterLink v-else :to="legalLink('privacy').href" class="font-medium text-lime-700 hover:text-lime-800">
            {{ $t("app.links.privacy") }}
          </RouterLink>
          /
          <RouterLink to="/federation/guidelines" class="font-medium text-lime-700 hover:text-lime-800">
            {{ $t("app.links.federation") }}
          </RouterLink>
          /
          <RouterLink to="/legal/api-guidelines" class="font-medium text-lime-700 hover:text-lime-800">
            {{ $t("app.links.apiReference") }}
          </RouterLink>
          {{ $t("auth.register.termsNotice") }}
        </p>
        <label class="mt-3 inline-flex cursor-pointer items-start gap-2 text-neutral-800">
          <input
            v-model="termsAgreed"
            type="checkbox"
            class="mt-0.5 h-4 w-4 rounded border-neutral-200 text-lime-600 focus:ring-lime-500"
          />
          <span>{{ $t("auth.register.agreement") }}</span>
        </label>
      </div>
      <p v-if="err" class="text-sm text-red-600">{{ err }}</p>
      <button
        type="submit"
        class="w-full rounded-md bg-lime-500 py-2 font-medium text-white hover:bg-lime-600 disabled:opacity-50"
        :disabled="!canSubmit"
      >
        {{ $t("auth.register.submit") }}
      </button>
      <p class="text-center text-sm text-neutral-600">
        {{ $t("auth.register.alreadyHaveAccount") }}
        <RouterLink to="/login" class="font-medium text-lime-700 hover:text-lime-800">
          {{ $t("auth.register.login") }}
        </RouterLink>
      </p>
    </form>
    <div v-else class="space-y-3 rounded-xl border border-lime-200 bg-lime-50 p-4 text-sm text-neutral-700">
      <p class="font-medium text-neutral-900">{{ $t("auth.register.submittedTitle") }}</p>
      <p>
        {{ $t("auth.register.submittedBody", { email }) }}
      </p>
      <p v-if="expiresAt" class="text-neutral-600">
        {{ $t("common.labels.validUntil", { date: formatDateTime(expiresAt, { dateStyle: 'short', timeStyle: 'short' }) }) }}
      </p>
      <p class="text-neutral-600">
        {{ $t("auth.register.submittedHint") }}
      </p>
    </div>
  </div>
</template>
