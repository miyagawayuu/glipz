<script setup lang="ts">
import type { Ref } from "vue";
import { computed, inject, onActivated, onMounted, provide, ref } from "vue";
import { RouterLink } from "vue-router";
import { useI18n } from "vue-i18n";
import DMSettingsPanel from "../components/DMSettingsPanel.vue";
import Icon from "../components/Icon.vue";
import SecuritySettingsPanel from "../components/SecuritySettingsPanel.vue";
import { securitySettingsKey, useSecuritySettings } from "../composables/useSecuritySettings";
import type { ThemePreference } from "../lib/theme";
import { readStoredThemePreference } from "../lib/theme";
import { setLocale, type AppLocale } from "../i18n";

type AppMe = { handle: string; is_site_admin?: boolean } | null;

const { t, locale } = useI18n();
const securitySettings = useSecuritySettings();
provide(securitySettingsKey, securitySettings);
const appMe = inject<Ref<AppMe> | null>("appMe", null);
const setThemePreferenceSilent = inject<(next: ThemePreference) => void>("setThemePreferenceSilent", () => {});

const themePreference = ref<ThemePreference>(readStoredThemePreference());

const profilePath = computed(() => (appMe?.value?.handle ? `/@${appMe.value.handle}` : "/feed"));
const showAdmin = computed(() => Boolean(appMe?.value?.is_site_admin));

const themeOptions = computed(() =>
  (["system", "light", "dark"] as const).map((value) => ({
    value,
    label:
      value === "system"
        ? t("app.theme.system")
        : value === "light"
          ? t("app.theme.light")
          : t("app.theme.dark"),
    description:
      value === "system"
        ? t("app.theme.systemDescription")
        : value === "light"
          ? t("app.theme.lightDescription")
          : t("app.theme.darkDescription"),
  })),
);

const localeOptions = computed(() => [
  { value: "ja" as const, label: t("app.locale.ja") },
  { value: "en" as const, label: t("app.locale.en") },
]);

function selectTheme(next: ThemePreference) {
  themePreference.value = next;
  setThemePreferenceSilent(next);
}

function selectLocale(next: AppLocale) {
  setLocale(next);
}

function syncThemeFromStorage() {
  themePreference.value = readStoredThemePreference();
}

onMounted(syncThemeFromStorage);
onActivated(syncThemeFromStorage);
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="flex h-14 items-center gap-3">
      <h1 class="truncate text-lg font-bold">{{ $t("views.settings.title") }}</h1>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="px-4 py-4">
      <h1 class="text-xl font-bold">{{ $t("views.settings.title") }}</h1>
      <p class="mt-1 text-sm text-neutral-600">{{ $t("views.settings.lead") }}</p>
    </div>
  </Teleport>

  <div class="mx-auto max-w-2xl px-4 py-8">
    <div class="space-y-10">
      <section>
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.account") }}
        </h2>
        <div class="mt-3 divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
          <RouterLink
            :to="profilePath"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.profileEdit") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/bookmarks"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.bookmarks") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/settings/custom-emojis"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.customEmojis") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
        </div>
        <p class="mt-2 text-xs text-neutral-500">{{ $t("views.settings.hints.account") }}</p>
      </section>

      <section>
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.posts") }}
        </h2>
        <div class="mt-3 divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
          <RouterLink
            to="/feed/scheduled"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.scheduledPosts") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
        </div>
      </section>

      <section>
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.directMessages") }}
        </h2>
        <div class="mt-3">
          <DMSettingsPanel />
        </div>
      </section>

      <SecuritySettingsPanel />

      <section>
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.developer") }}
        </h2>
        <div class="mt-3 divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
          <RouterLink
            to="/developer/api"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.apiDeveloper") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/legal/api-guidelines"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.apiReferencePublic") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
        </div>
      </section>

      <section>
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.appearance") }}
        </h2>
        <div class="mt-3 space-y-3 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
          <div>
            <p class="text-sm font-medium text-neutral-900">{{ $t("app.theme.heading") }}</p>
            <div class="mt-2 space-y-1">
              <button
                v-for="opt in themeOptions"
                :key="opt.value"
                type="button"
                class="flex w-full items-start justify-between gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition-colors"
                :class="
                  themePreference === opt.value
                    ? 'bg-lime-50 text-lime-900'
                    : 'text-neutral-800 hover:bg-neutral-50'
                "
                @click="selectTheme(opt.value)"
              >
                <span class="min-w-0">
                  <span class="block font-medium">{{ opt.label }}</span>
                  <span
                    class="mt-0.5 block text-xs"
                    :class="themePreference === opt.value ? 'text-lime-800' : 'text-neutral-500'"
                  >{{ opt.description }}</span>
                </span>
                <span
                  class="mt-0.5 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full border"
                  :class="
                    themePreference === opt.value
                      ? 'border-lime-600 bg-lime-600 text-white'
                      : 'border-neutral-200 text-transparent'
                  "
                  aria-hidden="true"
                >
                  <Icon name="check" class="h-3.5 w-3.5" stroke-width="2" />
                </span>
              </button>
            </div>
          </div>
          <div class="border-t border-neutral-200 pt-4">
            <p class="text-sm font-medium text-neutral-900">{{ $t("app.locale.heading") }}</p>
            <div class="mt-2 space-y-1">
              <button
                v-for="opt in localeOptions"
                :key="opt.value"
                type="button"
                class="flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition-colors"
                :class="locale === opt.value ? 'bg-lime-50 text-lime-900' : 'text-neutral-800 hover:bg-neutral-50'"
                @click="selectLocale(opt.value)"
              >
                <span class="font-medium">{{ opt.label }}</span>
                <span
                  class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full border"
                  :class="
                    locale === opt.value
                      ? 'border-lime-600 bg-lime-600 text-white'
                      : 'border-neutral-200 text-transparent'
                  "
                  aria-hidden="true"
                >
                  <Icon name="check" class="h-3.5 w-3.5" stroke-width="2" />
                </span>
              </button>
            </div>
          </div>
        </div>
      </section>

      <section v-if="showAdmin">
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.admin") }}
        </h2>
        <div class="mt-3 divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
          <RouterLink
            to="/admin/reports"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.adminReports") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/admin/federation"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.adminFederation") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/admin/user-badges"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.adminUserBadges") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/admin/custom-emojis"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.adminCustomEmojis") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
        </div>
      </section>
    </div>
  </div>
</template>
