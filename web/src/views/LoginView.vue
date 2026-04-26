<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { api } from "../lib/api";
import { setAccessToken, setMfaToken } from "../auth";
import { safeRelativeRoute } from "../lib/redirect";

const router = useRouter();
const route = useRoute();
const { t } = useI18n();
const email = ref("");
const password = ref("");
const err = ref("");
const loading = ref(false);

async function submit() {
  err.value = "";
  loading.value = true;
  try {
    const res = await api<{
      mfa_required: boolean;
    }>("/api/v1/auth/login", {
      method: "POST",
      json: { email: email.value, password: password.value },
    });
    if (res.mfa_required) {
      setMfaToken("1");
      await router.push("/mfa");
      return;
    }
    setAccessToken();
    const next = safeRelativeRoute(route.query.next, "/feed");
    await router.push(next);
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
