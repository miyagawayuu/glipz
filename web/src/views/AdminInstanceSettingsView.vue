<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";
import type { OperatorAnnouncement } from "../lib/instanceSettings";

type SiteSettings = {
  registrations_enabled: boolean;
  server_name: string;
  server_description: string;
  admin_name: string;
  admin_email: string;
  terms_url: string;
  privacy_policy_url: string;
  nsfw_guidelines_url: string;
  federation_policy_summary: string;
  operator_announcements: OperatorAnnouncement[];
};

type SiteSettingsResponse = Partial<Omit<SiteSettings, "operator_announcements">> & {
  operator_announcements?: Partial<OperatorAnnouncement>[] | null;
};

const { t } = useI18n();
const loading = ref(true);
const saving = ref(false);
const err = ref("");
const notice = ref("");
const settings = ref<SiteSettings>({
  registrations_enabled: true,
  server_name: "",
  server_description: "",
  admin_name: "",
  admin_email: "",
  terms_url: "",
  privacy_policy_url: "",
  nsfw_guidelines_url: "",
  federation_policy_summary: "",
  operator_announcements: [],
});

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function normalizeAnnouncement(row: Partial<OperatorAnnouncement> | null | undefined, index: number): OperatorAnnouncement {
  const id = stringValue(row?.id).trim() || `notice-${index + 1}`;
  return {
    id,
    title: stringValue(row?.title).trim(),
    body: stringValue(row?.body).trim(),
    date: stringValue(row?.date).trim(),
  };
}

function normalizeSettings(raw: SiteSettingsResponse | null | undefined): SiteSettings {
  return {
    registrations_enabled: !!raw?.registrations_enabled,
    server_name: stringValue(raw?.server_name),
    server_description: stringValue(raw?.server_description),
    admin_name: stringValue(raw?.admin_name),
    admin_email: stringValue(raw?.admin_email),
    terms_url: stringValue(raw?.terms_url),
    privacy_policy_url: stringValue(raw?.privacy_policy_url),
    nsfw_guidelines_url: stringValue(raw?.nsfw_guidelines_url),
    federation_policy_summary: stringValue(raw?.federation_policy_summary),
    operator_announcements: Array.isArray(raw?.operator_announcements)
      ? raw.operator_announcements.map((row, index) => normalizeAnnouncement(row, index))
      : [],
  };
}

async function loadSettings() {
  const token = getAccessToken();
  if (!token) return;
  loading.value = true;
  err.value = "";
  try {
    const res = await api<{ settings: SiteSettingsResponse }>("/api/v1/admin/instance-settings", { method: "GET", token });
    settings.value = normalizeSettings(res.settings);
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.adminInstanceSettings.errors.loadFailed");
  } finally {
    loading.value = false;
  }
}

async function saveSettings() {
  const token = getAccessToken();
  if (!token) return;
  saving.value = true;
  err.value = "";
  notice.value = "";
  try {
    const payload: SiteSettings = {
      registrations_enabled: settings.value.registrations_enabled,
      server_name: settings.value.server_name.trim(),
      server_description: settings.value.server_description.trim(),
      admin_name: settings.value.admin_name.trim(),
      admin_email: settings.value.admin_email.trim(),
      terms_url: settings.value.terms_url.trim(),
      privacy_policy_url: settings.value.privacy_policy_url.trim(),
      nsfw_guidelines_url: settings.value.nsfw_guidelines_url.trim(),
      federation_policy_summary: settings.value.federation_policy_summary.trim(),
      operator_announcements: settings.value.operator_announcements
        .map((row, index) => normalizeAnnouncement(row, index))
        .filter((row) => row.title || row.body || row.date),
    };
    const res = await api<{ settings: SiteSettingsResponse }>("/api/v1/admin/instance-settings", {
      method: "PATCH",
      token,
      json: payload,
    });
    settings.value = normalizeSettings(res.settings);
    notice.value = t("views.adminInstanceSettings.saved");
  } catch (e: unknown) {
    err.value = e instanceof Error && e.message === "invalid_url"
      ? t("views.adminInstanceSettings.errors.invalidURL")
      : e instanceof Error
        ? e.message
        : t("views.adminInstanceSettings.errors.saveFailed");
  } finally {
    saving.value = false;
  }
}

function addAnnouncement() {
  settings.value.operator_announcements = [
    ...settings.value.operator_announcements,
    { id: "", title: "", body: "", date: new Date().toISOString().slice(0, 10) },
  ];
}

function removeAnnouncement(index: number) {
  settings.value.operator_announcements = settings.value.operator_announcements.filter((_, i) => i !== index);
}

onMounted(() => {
  void loadSettings();
});
</script>

<template>
  <div class="mx-auto max-w-5xl px-4 py-8">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-lime-700">{{ $t("views.adminShell.eyebrow") }}</p>
        <h1 class="mt-2 text-2xl font-bold text-neutral-900">{{ $t("views.adminInstanceSettings.title") }}</h1>
        <p class="mt-2 text-sm text-neutral-600">{{ $t("views.adminInstanceSettings.description") }}</p>
      </div>
      <button
        type="button"
        class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700 disabled:opacity-50"
        :disabled="saving"
        @click="saveSettings"
      >
        {{ saving ? $t("common.actions.saving") : $t("common.actions.save") }}
      </button>
    </header>

    <p v-if="err" class="mt-5 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">{{ err }}</p>
    <p v-if="notice" class="mt-5 rounded-xl border border-lime-200 bg-lime-50 px-4 py-3 text-sm text-lime-800">{{ notice }}</p>
    <p v-if="loading" class="mt-8 text-sm text-neutral-500">{{ $t("app.loading") }}</p>

    <template v-else>
      <section class="mt-6 rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
        <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.adminInstanceSettings.serverInfoHeading") }}</h2>
        <p class="mt-1 text-sm text-neutral-600">{{ $t("views.adminInstanceSettings.serverInfoHint") }}</p>
        <div class="mt-4 grid gap-4 md:grid-cols-2">
          <label class="block text-sm">
            <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.serverName") }}</span>
            <input
              v-model="settings.server_name"
              type="text"
              class="w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              :placeholder="$t('views.adminInstanceSettings.serverNamePlaceholder')"
            />
          </label>
          <label class="block text-sm">
            <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.adminName") }}</span>
            <input
              v-model="settings.admin_name"
              type="text"
              class="w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              :placeholder="$t('views.adminInstanceSettings.adminNamePlaceholder')"
            />
          </label>
        </div>
        <label class="mt-4 block text-sm">
          <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.serverDescription") }}</span>
          <textarea
            v-model="settings.server_description"
            rows="4"
            class="w-full rounded-2xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
            :placeholder="$t('views.adminInstanceSettings.serverDescriptionPlaceholder')"
          />
        </label>
        <label class="mt-4 block text-sm">
          <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.adminEmail") }}</span>
          <input
            v-model="settings.admin_email"
            type="email"
            class="w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
            :placeholder="$t('views.adminInstanceSettings.adminEmailPlaceholder')"
          />
        </label>
      </section>

      <section class="mt-6 rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
        <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.adminInstanceSettings.registrationHeading") }}</h2>
        <label class="mt-4 flex items-start gap-3 rounded-2xl bg-neutral-50 p-4">
          <input
            v-model="settings.registrations_enabled"
            type="checkbox"
            class="mt-0.5 h-4 w-4 rounded border-neutral-300 text-lime-600 focus:ring-lime-500"
          />
          <span>
            <span class="block text-sm font-semibold text-neutral-900">{{ $t("views.adminInstanceSettings.registrationsEnabled") }}</span>
            <span class="mt-1 block text-sm text-neutral-600">{{ $t("views.adminInstanceSettings.registrationsHint") }}</span>
          </span>
        </label>
      </section>

      <section class="mt-6 rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
        <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.adminInstanceSettings.legalUrlsHeading") }}</h2>
        <p class="mt-1 text-sm text-neutral-600">{{ $t("views.adminInstanceSettings.legalUrlsHint") }}</p>
        <div class="mt-4 grid gap-4">
          <label class="block text-sm">
            <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.termsUrl") }}</span>
            <input
              v-model="settings.terms_url"
              type="url"
              inputmode="url"
              class="w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              :placeholder="$t('views.adminInstanceSettings.legalUrlPlaceholder')"
            />
          </label>
          <label class="block text-sm">
            <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.privacyPolicyUrl") }}</span>
            <input
              v-model="settings.privacy_policy_url"
              type="url"
              inputmode="url"
              class="w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              :placeholder="$t('views.adminInstanceSettings.legalUrlPlaceholder')"
            />
          </label>
          <label class="block text-sm">
            <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.nsfwGuidelinesUrl") }}</span>
            <input
              v-model="settings.nsfw_guidelines_url"
              type="url"
              inputmode="url"
              class="w-full rounded-xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              :placeholder="$t('views.adminInstanceSettings.legalUrlPlaceholder')"
            />
          </label>
        </div>
      </section>

      <section class="mt-6 rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
        <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.adminInstanceSettings.federationHeading") }}</h2>
        <label class="mt-4 block text-sm">
          <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.policySummary") }}</span>
          <textarea
            v-model="settings.federation_policy_summary"
            rows="4"
            class="w-full rounded-2xl border border-neutral-200 bg-white px-4 py-3 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
            :placeholder="$t('views.adminInstanceSettings.policyPlaceholder')"
          />
        </label>
      </section>

      <section class="mt-6 rounded-3xl border border-neutral-200 bg-white p-5 shadow-sm">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.adminInstanceSettings.announcementsHeading") }}</h2>
            <p class="mt-1 text-sm text-neutral-600">{{ $t("views.adminInstanceSettings.announcementsHint") }}</p>
          </div>
          <button
            type="button"
            class="rounded-full border border-lime-200 px-4 py-2 text-sm font-semibold text-lime-800 hover:bg-lime-50"
            @click="addAnnouncement"
          >
            {{ $t("views.adminInstanceSettings.addAnnouncement") }}
          </button>
        </div>

        <p v-if="!settings.operator_announcements.length" class="mt-5 text-sm text-neutral-500">
          {{ $t("views.adminInstanceSettings.noAnnouncements") }}
        </p>
        <div v-else class="mt-5 space-y-4">
          <article
            v-for="(row, index) in settings.operator_announcements"
            :key="index"
            class="rounded-2xl border border-neutral-200 bg-neutral-50 p-4"
          >
            <div class="grid gap-3 md:grid-cols-2">
              <label class="block text-sm">
                <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.announcementID") }}</span>
                <input v-model="row.id" type="text" class="w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm" />
              </label>
              <label class="block text-sm">
                <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.announcementDate") }}</span>
                <input v-model="row.date" type="text" class="w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm" placeholder="YYYY-MM-DD" />
              </label>
            </div>
            <label class="mt-3 block text-sm">
              <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.announcementTitle") }}</span>
              <input v-model="row.title" type="text" class="w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm" />
            </label>
            <label class="mt-3 block text-sm">
              <span class="mb-1 block font-medium text-neutral-700">{{ $t("views.adminInstanceSettings.announcementBody") }}</span>
              <textarea v-model="row.body" rows="3" class="w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm" />
            </label>
            <div class="mt-3 flex justify-end">
              <button type="button" class="rounded-full px-3 py-1.5 text-xs font-semibold text-red-700 hover:bg-red-50" @click="removeAnnouncement(index)">
                {{ $t("common.actions.delete") }}
              </button>
            </div>
          </article>
        </div>
      </section>
    </template>
  </div>
</template>
