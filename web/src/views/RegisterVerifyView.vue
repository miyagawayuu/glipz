<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { setAccessToken } from "../auth";
import { api } from "../lib/api";

const route = useRoute();
const router = useRouter();
const { t } = useI18n();
const loading = ref(true);
const success = ref(false);
const message = ref(t("auth.verify.verifying"));

function messageForError(code: string): string {
  switch (code) {
    case "invalid_token":
      return t("auth.verify.errors.invalidToken");
    case "expired_token":
      return t("auth.verify.errors.expiredToken");
    case "already_verified":
      return t("auth.verify.errors.alreadyVerified");
    case "email_taken":
      return t("auth.verify.errors.emailTaken");
    default:
      return t("auth.verify.errors.failed");
  }
}

function clearTokenFromURL() {
  const query = { ...route.query };
  delete query.token;
  void router.replace({ path: route.path, query });
}

onMounted(async () => {
  const token = typeof route.query.token === "string" ? route.query.token.trim() : "";
  if (!token) {
    loading.value = false;
    message.value = t("auth.verify.missing");
    return;
  }
  clearTokenFromURL();
  try {
    const res = await api<{ access_token: string }>("/api/v1/auth/register/verify", {
      method: "POST",
      json: { token },
    });
    setAccessToken(res.access_token);
    success.value = true;
    message.value = t("auth.verify.success");
    await router.replace("/feed");
  } catch (error: unknown) {
    loading.value = false;
    message.value = messageForError(error instanceof Error ? error.message : "");
  }
});
</script>

<template>
  <div class="mx-auto max-w-md space-y-6">
    <div>
      <h1 class="text-2xl font-semibold text-neutral-900">{{ $t("auth.verify.title") }}</h1>
      <p class="mt-1 text-sm text-neutral-600">
        {{ $t("auth.verify.description") }}
      </p>
    </div>
    <div
      class="rounded-xl border p-4 text-sm"
      :class="
        success
          ? 'border-lime-200 bg-lime-50 text-neutral-700'
          : 'border-neutral-200 bg-white text-neutral-700'
      "
    >
      <p>{{ message }}</p>
      <p v-if="loading" class="mt-2 text-neutral-500">{{ $t("auth.verify.wait") }}</p>
      <RouterLink v-else-if="!success" to="/register" class="mt-3 inline-block font-medium text-lime-700 hover:text-lime-800">
        {{ $t("auth.verify.backToRegister") }}
      </RouterLink>
    </div>
  </div>
</template>
