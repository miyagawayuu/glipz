<script setup lang="ts">
import type { Ref } from "vue";
import { computed, inject, onActivated, onMounted, provide, ref } from "vue";
import { RouterLink } from "vue-router";
import { useI18n } from "vue-i18n";
import DMSettingsPanel from "../components/DMSettingsPanel.vue";
import FanclubGumroadSettings from "../components/FanclubGumroadSettings.vue";
import FanclubPatreonSettings from "../components/FanclubPatreonSettings.vue";
import PaymentPayPalSettings from "../components/PaymentPayPalSettings.vue";
import Icon from "../components/Icon.vue";
import SecuritySettingsPanel from "../components/SecuritySettingsPanel.vue";
import { securitySettingsKey, useSecuritySettings } from "../composables/useSecuritySettings";
import type { ThemePreference } from "../lib/theme";
import { readStoredThemePreference } from "../lib/theme";
import { setLocale, type AppLocale } from "../i18n";

type AppMe = {
  handle: string;
  is_site_admin?: boolean;
  fanclub_patreon_enabled?: boolean;
  fanclub_gumroad_enabled?: boolean;
  payment_paypal_enabled?: boolean;
} | null;

const { t, locale } = useI18n();
const securitySettings = useSecuritySettings();
provide(securitySettingsKey, securitySettings);
const appMe = inject<Ref<AppMe> | null>("appMe", null);
const setThemePreferenceSilent = inject<(next: ThemePreference) => void>("setThemePreferenceSilent", () => {});

const themePreference = ref<ThemePreference>(readStoredThemePreference());

const profilePath = computed(() => (appMe?.value?.handle ? `/@${appMe.value.handle}` : "/feed"));
const showAdmin = computed(() => Boolean(appMe?.value?.is_site_admin));
const showFanclubGumroad = computed(() => Boolean(appMe?.value?.fanclub_gumroad_enabled));
const showFanclubPatreon = computed(() => Boolean(appMe?.value?.fanclub_patreon_enabled));
const showFanclub = computed(() => showFanclubGumroad.value || showFanclubPatreon.value);
const showPayments = computed(() => Boolean(appMe?.value?.payment_paypal_enabled));

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
const selectedThemeDescription = computed(() =>
  themeOptions.value.find((opt) => opt.value === themePreference.value)?.description ?? "",
);

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

      <section v-if="showFanclub">
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.fanclub") }}
        </h2>
        <FanclubGumroadSettings v-if="showFanclubGumroad" />
        <FanclubPatreonSettings v-if="showFanclubPatreon" />
      </section>

      <section v-if="showPayments">
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.payments") }}
        </h2>
        <PaymentPayPalSettings />
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
            <select
              class="mt-2 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              :value="themePreference"
              @change="selectTheme(($event.target as HTMLSelectElement).value as ThemePreference)"
            >
              <option
                v-for="opt in themeOptions"
                :key="opt.value"
                :value="opt.value"
              >
                {{ opt.label }}
              </option>
            </select>
            <p class="mt-2 text-xs text-neutral-500">{{ selectedThemeDescription }}</p>
          </div>
          <div class="border-t border-neutral-200 pt-4">
            <p class="text-sm font-medium text-neutral-900">{{ $t("app.locale.heading") }}</p>
            <select
              class="mt-2 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
              :value="locale"
              @change="selectLocale(($event.target as HTMLSelectElement).value as AppLocale)"
            >
              <option
                v-for="opt in localeOptions"
                :key="opt.value"
                :value="opt.value"
              >
                {{ opt.label }}
              </option>
            </select>
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
