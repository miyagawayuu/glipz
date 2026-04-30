<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import AuthLogo from "../components/AuthLogo.vue";
import { api } from "../lib/api";
import { clearTokens, getMfaToken, setAccessToken } from "../auth";

const router = useRouter();
const { t } = useI18n();
const code = ref("");
const err = ref("");
const loading = ref(false);

onMounted(() => {
  if (!getMfaToken()) {
    router.replace("/login");
  }
});

async function submit() {
  err.value = "";
  const mfa = getMfaToken();
  if (!mfa) {
    await router.replace("/login");
    return;
  }
  loading.value = true;
  try {
    await api<{ status: string }>("/api/v1/auth/mfa/verify", {
      method: "POST",
      json: { code: code.value },
    });
    clearTokens();
    setAccessToken();
    await router.push("/feed");
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : "";
    err.value = msg === "account_suspended" ? t("auth.mfa.suspended") : msg || t("auth.mfa.failed");
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="mx-auto max-w-md space-y-6">
    <AuthLogo />
    <div>
      <h1 class="text-2xl font-semibold text-neutral-900">{{ $t("auth.mfa.title") }}</h1>
      <p class="mt-1 text-sm text-neutral-600">{{ $t("auth.mfa.description") }}</p>
    </div>
    <form class="space-y-4" @submit.prevent="submit">
      <div>
        <label class="block text-sm font-medium text-neutral-700">{{ $t("auth.mfa.code") }}</label>
        <input
          v-model="code"
          type="text"
          inputmode="numeric"
          pattern="[0-9]*"
          maxlength="8"
          required
          autocomplete="one-time-code"
          class="mt-1 w-full rounded-md border border-lime-200 bg-white px-3 py-2 text-neutral-900 outline-none ring-lime-500 focus:ring-2"
        />
      </div>
      <p v-if="err" class="text-sm text-red-600">{{ err }}</p>
      <button
        type="submit"
        class="w-full rounded-md bg-lime-500 py-2 font-medium text-white hover:bg-lime-600 disabled:opacity-50"
        :disabled="loading"
      >
        {{ $t("auth.mfa.submit") }}
      </button>
    </form>
  </div>
</template>
