<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { api, clearStoredApiBase, readStoredApiBase, writeStoredApiBase } from "../lib/api";
import { setAccessToken, setMfaToken } from "../auth";
import { isNativeApp } from "../lib/runtime";

const router = useRouter();
const route = useRoute();
const { t } = useI18n();
const showApiBaseInput = isNativeApp();
const apiBaseInput = ref(readStoredApiBase() || "");
const apiBaseError = ref("");
const effectiveApiBase = computed(() => (apiBaseInput.value.trim() ? apiBaseInput.value.trim() : t("auth.apiBase.sameOrigin")));
const email = ref("");
const password = ref("");
const err = ref("");
const loading = ref(false);

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

async function submit() {
  err.value = "";
  loading.value = true;
  try {
    const res = await api<{
      mfa_required: boolean;
      access_token?: string;
      mfa_token?: string;
    }>("/api/v1/auth/login", {
      method: "POST",
      json: { email: email.value, password: password.value },
    });
    if (res.mfa_required && res.mfa_token) {
      setMfaToken(res.mfa_token);
      await router.push("/mfa");
      return;
    }
    if (res.access_token) {
      setAccessToken(res.access_token);
      const next = typeof route.query.next === "string" ? route.query.next : "/feed";
      await router.push(next);
    }
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : "";
    err.value = msg === "account_suspended" ? t("auth.login.suspended") : msg || t("auth.login.failed");
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="mx-auto max-w-md space-y-6">
    <div>
      <h1 class="text-2xl font-semibold text-neutral-900">{{ $t("auth.login.title") }}</h1>
      <p class="mt-1 text-sm text-neutral-600">
        {{ $t("auth.login.description") }}
      </p>
    </div>
    <form class="space-y-4" @submit.prevent="submit">
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
        <p class="mt-1 text-xs text-neutral-500">{{ $t("auth.apiBase.current") }}: {{ effectiveApiBase }}</p>
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
        <label class="block text-sm font-medium text-neutral-700">{{ $t("auth.login.email") }}</label>
        <input
          v-model="email"
          type="email"
          required
          autocomplete="username"
          class="mt-1 w-full rounded-md border border-lime-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        />
      </div>
      <div>
        <label class="block text-sm font-medium text-neutral-700">{{ $t("auth.login.password") }}</label>
        <input
          v-model="password"
          type="password"
          required
          autocomplete="current-password"
          class="mt-1 w-full rounded-md border border-lime-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        />
      </div>
      <p v-if="err" class="text-sm text-red-600">{{ err }}</p>
      <button
        type="submit"
        class="w-full rounded-md bg-lime-500 py-2 font-medium text-white hover:bg-lime-600 disabled:opacity-50"
        :disabled="loading"
      >
        {{ $t("auth.login.submit") }}
      </button>
    </form>
    <p class="text-center text-sm text-neutral-600">
      {{ $t("auth.login.firstTime") }}
      <RouterLink to="/register" class="font-medium text-lime-700 hover:text-lime-800">
        {{ $t("auth.login.createAccount") }}
      </RouterLink>
    </p>
  </div>
</template>
