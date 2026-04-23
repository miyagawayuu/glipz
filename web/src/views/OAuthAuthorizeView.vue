<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute } from "vue-router";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";

const { t } = useI18n();
const route = useRoute();
const err = ref("");
const busy = ref(false);
const doneRedirect = ref("");

const clientId = computed(() => String(route.query.client_id ?? "").trim());
const redirectURI = computed(() => String(route.query.redirect_uri ?? "").trim());
const stateQ = computed(() => String(route.query.state ?? ""));
const responseType = computed(() => String(route.query.response_type ?? "code").trim());
const scopeQ = computed(() => String(route.query.scope ?? "posts").trim());

const canSubmit = computed(() => {
  return clientId.value.length > 0 && redirectURI.value.length > 0 && responseType.value === "code";
});

async function authorize() {
  const token = getAccessToken();
  if (!token) {
    err.value = t("views.oauthAuthorize.loginRequired");
    return;
  }
  busy.value = true;
  err.value = "";
  doneRedirect.value = "";
  try {
    const res = await api<{ redirect_to: string }>("/api/v1/me/oauth-authorize", {
      method: "POST",
      token,
      json: {
        client_id: clientId.value,
        redirect_uri: redirectURI.value,
        state: stateQ.value,
        scope: scopeQ.value,
      },
    });
    if (res.redirect_to) {
      window.location.href = res.redirect_to;
      doneRedirect.value = res.redirect_to;
    }
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.oauthAuthorize.authorizeFailed");
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="mx-auto max-w-lg px-4 py-10 text-neutral-900">
    <h1 class="text-lg font-bold">{{ t("views.oauthAuthorize.title") }}</h1>
    <p v-if="!canSubmit" class="mt-3 text-sm text-red-700">
      {{ t("views.oauthAuthorize.queryError") }}
    </p>
    <template v-else>
      <p class="mt-3 text-sm text-neutral-700">
        {{ t("views.oauthAuthorize.lead") }}
      </p>
      <dl class="mt-4 space-y-2 rounded-xl border border-neutral-200 bg-neutral-50 p-3 text-sm">
        <div>
          <dt class="text-xs font-medium text-neutral-500">client_id</dt>
          <dd class="mt-0.5 break-all font-mono text-xs">{{ clientId }}</dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-neutral-500">redirect_uri</dt>
          <dd class="mt-0.5 break-all font-mono text-xs">{{ redirectURI }}</dd>
        </div>
        <div v-if="stateQ">
          <dt class="text-xs font-medium text-neutral-500">state</dt>
          <dd class="mt-0.5 break-all font-mono text-xs">{{ stateQ }}</dd>
        </div>
      </dl>
      <p v-if="err" class="mt-3 text-sm text-red-700">{{ err }}</p>
      <button
        type="button"
        class="mt-6 w-full rounded-full bg-lime-600 py-2.5 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
        :disabled="busy"
        @click="authorize"
      >
        {{ busy ? t("views.oauthAuthorize.allowBusy") : t("views.oauthAuthorize.allow") }}
      </button>
      <p v-if="doneRedirect" class="mt-3 text-xs text-neutral-500">
        {{ t("views.oauthAuthorize.redirectingBeforeLink") }}
        <a :href="doneRedirect" class="text-lime-700 underline">{{ t("views.oauthAuthorize.hereLink") }}</a>{{ t("views.oauthAuthorize.redirectingAfterLink") }}
      </p>
    </template>
  </div>
</template>
