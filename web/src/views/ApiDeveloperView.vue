<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { getAccessToken } from "../auth";
import { api, apiBase } from "../lib/api";
import {
  formatApiDeveloperAuthorizationCodeCurl,
  formatApiDeveloperAuthorizeUrlSample,
  formatApiDeveloperClientCredentialsCurl,
  formatApiDeveloperMediaUploadCurl,
  formatApiDeveloperPostsExampleCurl,
} from "../lib/apiDeveloperCurlSnippets";

type OAuthClientItem = {
  client_id: string;
  name: string;
  redirect_uris: string[];
  created_at: string;
};

type PATItem = {
  id: string;
  label: string;
  token_prefix: string;
  created_at: string;
};

const oauthClients = ref<OAuthClientItem[]>([]);
const pats = ref<PATItem[]>([]);
const err = ref("");
const busy = ref(false);

const newClientName = ref("");
const newClientRedirects = ref("");
const newClientSecretOnce = ref("");
const newClientIdOnce = ref("");

const newPatLabel = ref("");
const newPatTokenOnce = ref("");

const { t } = useI18n();

const docBase = computed(() => {
  const b = apiBase();
  if (b) return b;
  if (typeof window !== "undefined") return window.location.origin;
  return "";
});

const clientCredentialsCurl = computed(() => formatApiDeveloperClientCredentialsCurl(docBase.value));
const authorizationCodeCurl = computed(() => formatApiDeveloperAuthorizationCodeCurl(docBase.value));
const mediaUploadCurl = computed(() => formatApiDeveloperMediaUploadCurl(docBase.value));
const authorizeUrlSampleText = computed(() => formatApiDeveloperAuthorizeUrlSample(docBase.value));
const postsExampleCurl = computed(() => formatApiDeveloperPostsExampleCurl(docBase.value));

async function loadLists() {
  const token = getAccessToken();
  if (!token) return;
  err.value = "";
  try {
    const [oc, pt] = await Promise.all([
      api<{ items: OAuthClientItem[] }>("/api/v1/me/oauth-clients", { method: "GET", token }),
      api<{ items: PATItem[] }>("/api/v1/me/personal-access-tokens", { method: "GET", token }),
    ]);
    oauthClients.value = oc.items ?? [];
    pats.value = pt.items ?? [];
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.apiDeveloper.errors.loadFailed");
  }
}

async function createOAuthClient() {
  const token = getAccessToken();
  if (!token) return;
  busy.value = true;
  err.value = "";
  newClientSecretOnce.value = "";
  newClientIdOnce.value = "";
  try {
    const lines = newClientRedirects.value
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean);
    const res = await api<{ client_id: string; client_secret: string; name: string }>(
      "/api/v1/me/oauth-clients",
      {
        method: "POST",
        token,
        json: { name: newClientName.value.trim(), redirect_uris: lines },
      },
    );
    newClientIdOnce.value = res.client_id;
    newClientSecretOnce.value = res.client_secret;
    newClientName.value = "";
    newClientRedirects.value = "";
    await loadLists();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.apiDeveloper.errors.createFailed");
  } finally {
    busy.value = false;
  }
}

async function deleteOAuthClient(id: string) {
  if (!confirm(t("views.apiDeveloper.oauth.deleteConfirm"))) return;
  const token = getAccessToken();
  if (!token) return;
  busy.value = true;
  err.value = "";
  try {
    await api(`/api/v1/me/oauth-clients/${encodeURIComponent(id)}`, { method: "DELETE", token });
    await loadLists();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.apiDeveloper.errors.deleteFailed");
  } finally {
    busy.value = false;
  }
}

async function createPAT() {
  const token = getAccessToken();
  if (!token) return;
  busy.value = true;
  err.value = "";
  newPatTokenOnce.value = "";
  try {
    const res = await api<{ token: string }>("/api/v1/me/personal-access-tokens", {
      method: "POST",
      token,
      json: { label: newPatLabel.value.trim() || t("views.apiDeveloper.pat.defaultLabel") },
    });
    newPatTokenOnce.value = res.token;
    newPatLabel.value = "";
    await loadLists();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.apiDeveloper.errors.createFailed");
  } finally {
    busy.value = false;
  }
}

async function deletePAT(id: string) {
  if (!confirm(t("views.apiDeveloper.pat.revokeConfirm"))) return;
  const token = getAccessToken();
  if (!token) return;
  busy.value = true;
  err.value = "";
  try {
    await api(`/api/v1/me/personal-access-tokens/${encodeURIComponent(id)}`, { method: "DELETE", token });
    await loadLists();
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.apiDeveloper.errors.deleteFailed");
  } finally {
    busy.value = false;
  }
}

function copyText(t: string) {
  void navigator.clipboard.writeText(t).catch(() => {});
}

onMounted(() => void loadLists());
</script>

<template>
  <div class="min-h-0 flex-1 overflow-y-auto bg-white px-4 py-6 text-neutral-900">
    <header class="mb-6 border-b border-neutral-200 pb-4">
      <h1 class="text-xl font-bold tracking-tight">{{ t("views.apiDeveloper.title") }}</h1>
      <p class="mt-2 text-sm text-neutral-600">
        {{ t("views.apiDeveloper.introPart1") }}<strong class="font-medium text-neutral-800">{{ t("views.apiDeveloper.introBearer") }}</strong>{{ t("views.apiDeveloper.introPart2") }}
      </p>
      <p class="mt-3 text-sm text-neutral-600">
        {{ t("views.apiDeveloper.openApiBanner") }}
        <RouterLink to="/legal/api-guidelines" class="ml-1 font-medium text-lime-700 hover:text-lime-800">{{
          t("views.apiDeveloper.openApiLinkLabel")
        }}</RouterLink>
      </p>
    </header>

    <p v-if="err" class="mb-4 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">{{ err }}</p>

    <section class="mb-10 space-y-3 rounded-xl border border-neutral-200 bg-neutral-50/80 p-4 text-sm">
      <h2 class="text-sm font-semibold text-neutral-800">{{ t("views.apiDeveloper.endpoints.heading") }}</h2>
      <p class="text-neutral-600">
        {{ t("views.apiDeveloper.endpoints.leadBefore") }}<code class="rounded bg-white px-1 py-0.5 text-xs">{{
          docBase || t("views.apiDeveloper.endpoints.sameOrigin")
        }}</code>{{ t("views.apiDeveloper.endpoints.leadAfter") }}
      </p>
      <ul class="list-inside list-disc space-y-1 text-neutral-600">
        <li>
          <code class="text-xs">POST {{ docBase }}/api/v1/oauth/token</code> {{ t("views.apiDeveloper.endpoints.oauthToken") }}
        </li>
        <li>
          <code class="text-xs">POST {{ docBase }}/api/v1/posts</code> {{ t("views.apiDeveloper.endpoints.postsCreate") }}
        </li>
        <li>
          <code class="text-xs">PATCH {{ docBase }}/api/v1/posts/{postID}</code> {{ t("views.apiDeveloper.endpoints.postsPatch") }}
        </li>
        <li>
          <code class="text-xs">DELETE {{ docBase }}/api/v1/posts/{postID}</code> {{ t("views.apiDeveloper.endpoints.postsDelete") }}
        </li>
        <li>
          <code class="text-xs">POST {{ docBase }}/api/v1/media/upload</code> {{ t("views.apiDeveloper.endpoints.mediaUpload") }}
        </li>
      </ul>
    </section>

    <section class="mb-10 rounded-xl border border-lime-200/80 bg-lime-50/40 p-4">
      <h2 class="text-sm font-semibold text-lime-900">{{ t("views.apiDeveloper.pat.heading") }}</h2>
      <p class="mt-2 text-sm text-neutral-700">
        {{ t("views.apiDeveloper.pat.lead") }}
      </p>
      <div class="mt-3 flex flex-wrap items-end gap-2">
        <label class="min-w-[12rem] flex-1">
          <span class="text-xs font-medium text-neutral-600">{{ t("views.apiDeveloper.pat.label") }}</span>
          <input
            v-model="newPatLabel"
            type="text"
            maxlength="80"
            class="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm"
            :placeholder="t('views.apiDeveloper.pat.placeholder')"
          />
        </label>
        <button
          type="button"
          class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
          :disabled="busy"
          @click="createPAT"
        >
          {{ t("views.apiDeveloper.pat.issue") }}
        </button>
      </div>
      <div v-if="newPatTokenOnce" class="mt-3 rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm">
        <p class="font-medium text-amber-900">{{ t("views.apiDeveloper.pat.onceWarning") }}</p>
        <pre class="mt-2 overflow-x-auto whitespace-pre-wrap break-all text-xs text-neutral-800">{{ newPatTokenOnce }}</pre>
        <button type="button" class="mt-2 text-xs font-medium text-lime-800 hover:underline" @click="copyText(newPatTokenOnce)">
          {{ t("views.apiDeveloper.pat.copy") }}
        </button>
      </div>
      <ul v-if="pats.length" class="mt-4 divide-y divide-neutral-200 rounded-lg border border-neutral-200 bg-white text-sm">
        <li v-for="p in pats" :key="p.id" class="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
          <div>
            <span class="font-medium">{{ p.label }}</span>
            <span class="ml-2 text-xs text-neutral-500">{{ p.token_prefix }}</span>
          </div>
          <button type="button" class="text-xs text-red-700 hover:underline" :disabled="busy" @click="deletePAT(p.id)">
            {{ t("views.apiDeveloper.pat.revoke") }}
          </button>
        </li>
      </ul>
    </section>

    <section class="mb-10 rounded-xl border border-violet-200/80 bg-violet-50/40 p-4">
      <h2 class="text-sm font-semibold text-violet-900">{{ t("views.apiDeveloper.oauth.heading") }}</h2>
      <p class="mt-2 text-sm text-neutral-700">
        <strong>client_credentials</strong>: {{ t("views.apiDeveloper.oauth.leadLine1") }}<br />
        <strong>authorization_code</strong>: {{ t("views.apiDeveloper.oauth.leadLine2") }}
      </p>
      <div class="mt-3 space-y-2">
        <label class="block">
          <span class="text-xs font-medium text-neutral-600">{{ t("views.apiDeveloper.oauth.appName") }}</span>
          <input
            v-model="newClientName"
            type="text"
            maxlength="120"
            class="mt-1 w-full max-w-md rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm"
            :placeholder="t('views.apiDeveloper.oauth.appNamePlaceholder')"
          />
        </label>
        <label class="block">
          <span class="text-xs font-medium text-neutral-600">{{ t("views.apiDeveloper.oauth.redirectsLabel") }}</span>
          <textarea
            v-model="newClientRedirects"
            rows="3"
            class="mt-1 w-full max-w-xl rounded-lg border border-neutral-200 bg-white px-3 py-2 font-mono text-xs"
            :placeholder="t('views.apiDeveloper.oauth.redirectsPlaceholder')"
          />
        </label>
        <button
          type="button"
          class="rounded-full bg-violet-700 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-800 disabled:opacity-50"
          :disabled="busy"
          @click="createOAuthClient"
        >
          {{ t("views.apiDeveloper.oauth.register") }}
        </button>
      </div>
      <div v-if="newClientIdOnce && newClientSecretOnce" class="mt-3 rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm">
        <p class="font-medium text-amber-900">{{ t("views.apiDeveloper.oauth.secretOnce") }}</p>
        <p class="mt-1 text-xs text-neutral-700">
          {{ t("views.apiDeveloper.oauth.clientIdPrefix") }} <code class="break-all">{{ newClientIdOnce }}</code>
        </p>
        <p class="mt-1 text-xs text-neutral-700">
          {{ t("views.apiDeveloper.oauth.clientSecretPrefix") }} <code class="break-all">{{ newClientSecretOnce }}</code>
        </p>
        <button
          type="button"
          class="mt-2 text-xs font-medium text-violet-800 hover:underline"
          @click="copyText(`client_id=${newClientIdOnce}\nclient_secret=${newClientSecretOnce}`)"
        >
          {{ t("views.apiDeveloper.oauth.copyIdSecret") }}
        </button>
      </div>
      <ul v-if="oauthClients.length" class="mt-4 divide-y divide-neutral-200 rounded-lg border border-neutral-200 bg-white text-sm">
        <li v-for="c in oauthClients" :key="c.client_id" class="px-3 py-2">
          <div class="flex flex-wrap items-start justify-between gap-2">
            <div>
              <p class="font-medium">{{ c.name }}</p>
              <p class="mt-0.5 font-mono text-xs text-neutral-600">{{ c.client_id }}</p>
              <p v-if="c.redirect_uris?.length" class="mt-1 text-xs text-neutral-500">
                {{ t("views.apiDeveloper.oauth.redirectPrefix") }} {{ c.redirect_uris.join(", ") }}
              </p>
            </div>
            <button type="button" class="shrink-0 text-xs text-red-700 hover:underline" :disabled="busy" @click="deleteOAuthClient(c.client_id)">
              {{ t("views.apiDeveloper.oauth.delete") }}
            </button>
          </div>
        </li>
      </ul>
    </section>

    <section class="mb-10 rounded-xl border border-neutral-200 bg-white p-4 text-sm">
      <h2 class="text-sm font-semibold text-neutral-900">{{ t("views.apiDeveloper.tokenClientCred.heading") }}</h2>
      <pre class="mt-2 overflow-x-auto rounded-lg bg-neutral-900 p-3 text-xs text-lime-100 whitespace-pre-wrap">{{ clientCredentialsCurl }}</pre>
    </section>

    <section class="mb-10 rounded-xl border border-neutral-200 bg-white p-4 text-sm">
      <h2 class="text-sm font-semibold text-neutral-900">{{ t("views.apiDeveloper.authCodeFlow.heading") }}</h2>
      <p class="mt-1 text-neutral-600">
        {{ t("views.apiDeveloper.authCodeFlow.lead") }}
      </p>
      <pre class="mt-2 overflow-x-auto rounded-lg bg-neutral-900 p-3 text-xs text-lime-100 whitespace-pre-wrap">{{ authorizeUrlSampleText }}</pre>
      <p class="mt-2 text-xs text-neutral-500">
        <RouterLink class="text-lime-700 hover:underline" to="/developer/oauth/authorize">{{
          t("views.apiDeveloper.authCodeFlow.openAuthorize")
        }}</RouterLink>
      </p>
      <pre class="mt-3 overflow-x-auto rounded-lg bg-neutral-900 p-3 text-xs text-lime-100 whitespace-pre-wrap">{{ authorizationCodeCurl }}</pre>
    </section>

    <section class="mb-10 rounded-xl border border-neutral-200 bg-white p-4 text-sm">
      <h2 class="text-sm font-semibold text-neutral-900">{{ t("views.apiDeveloper.mediaUpload.heading") }}</h2>
      <pre class="mt-2 overflow-x-auto rounded-lg bg-neutral-900 p-3 text-xs text-lime-100 whitespace-pre-wrap">{{ mediaUploadCurl }}</pre>
      <p class="mt-2 text-xs text-neutral-600">{{ t("views.apiDeveloper.mediaUpload.lead") }}</p>
    </section>

    <section class="rounded-xl border border-neutral-200 bg-white p-4 text-sm">
      <h2 class="text-sm font-semibold text-neutral-900">{{ t("views.apiDeveloper.postsExample.heading") }}</h2>
      <pre class="mt-2 overflow-x-auto rounded-lg bg-neutral-900 p-3 text-xs text-lime-100 whitespace-pre-wrap">{{ postsExampleCurl }}</pre>
    </section>
  </div>
</template>
