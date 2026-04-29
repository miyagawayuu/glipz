<script setup lang="ts">
import { computed, inject, onActivated, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import SettingsBackLink from "../components/SettingsBackLink.vue";
import type { CustomTheme, ThemePreference } from "../lib/theme";
import {
  DEFAULT_CUSTOM_THEME,
  applyTheme,
  persistCustomTheme,
  readStoredCustomTheme,
  readStoredThemePreference,
} from "../lib/theme";
import { setLocale, supportedLocaleOptions, type AppLocale } from "../i18n";

const { t, locale } = useI18n();
const setThemePreferenceSilent = inject<(next: ThemePreference) => void>("setThemePreferenceSilent", () => {});
const themePreference = ref<ThemePreference>(readStoredThemePreference());
const customTheme = ref<CustomTheme>(readStoredCustomTheme());

const themeOptions = computed(() =>
  (["system", "light", "dark", "custom"] as const).map((value) => ({
    value,
    label:
      value === "system"
        ? t("app.theme.system")
        : value === "light"
          ? t("app.theme.light")
          : value === "dark"
            ? t("app.theme.dark")
            : t("app.theme.custom"),
    description:
      value === "system"
        ? t("app.theme.systemDescription")
        : value === "light"
          ? t("app.theme.lightDescription")
          : value === "dark"
            ? t("app.theme.darkDescription")
            : t("app.theme.customDescription"),
  })),
);
const localeOptions = computed(() => [
  ...supportedLocaleOptions.map((option) => ({ value: option.value, label: t(option.labelKey) })),
]);
const customThemeColorFields = computed<Array<{ key: keyof CustomTheme; label: string }>>(() => [
  { key: "background", label: t("app.theme.customColors.background") },
  { key: "surface", label: t("app.theme.customColors.surface") },
  { key: "surfaceMuted", label: t("app.theme.customColors.surfaceMuted") },
  { key: "text", label: t("app.theme.customColors.text") },
  { key: "textMuted", label: t("app.theme.customColors.textMuted") },
  { key: "accentSoft", label: t("app.theme.customColors.accentSoft") },
  { key: "accentStrong", label: t("app.theme.customColors.accentStrong") },
  { key: "accentText", label: t("app.theme.customColors.accentText") },
  { key: "accentTextStrong", label: t("app.theme.customColors.accentTextStrong") },
]);
const selectedThemeDescription = computed(() =>
  themeOptions.value.find((opt) => opt.value === themePreference.value)?.description ?? "",
);

function selectTheme(next: ThemePreference) {
  themePreference.value = next;
  setThemePreferenceSilent(next);
}

function normalizeColorInput(value: string): string | null {
  const match = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(value.trim());
  if (!match) return null;
  const raw = match[1];
  if (raw.length === 3) {
    return `#${raw.split("").map((ch) => `${ch}${ch}`).join("")}`.toLowerCase();
  }
  return `#${raw}`.toLowerCase();
}

function updateCustomThemeColor(key: keyof CustomTheme, value: string) {
  const normalized = normalizeColorInput(value);
  if (!normalized) return;
  customTheme.value = { ...customTheme.value, [key]: normalized };
  persistCustomTheme(customTheme.value);
  if (themePreference.value !== "custom") {
    selectTheme("custom");
    return;
  }
  applyTheme("custom");
}

function resetCustomTheme() {
  customTheme.value = { ...DEFAULT_CUSTOM_THEME };
  persistCustomTheme(customTheme.value);
  selectTheme("custom");
  applyTheme("custom");
}

function selectLocale(next: AppLocale) {
  setLocale(next);
}

function syncThemeFromStorage() {
  themePreference.value = readStoredThemePreference();
  customTheme.value = readStoredCustomTheme();
}

onMounted(syncThemeFromStorage);
onActivated(syncThemeFromStorage);
</script>

<template>
  <div class="w-full px-4 py-8">
    <SettingsBackLink />
    <div class="mt-4">
      <h1 class="text-2xl font-bold text-neutral-900">{{ $t("routes.appearanceSettings") }}</h1>
    </div>

    <section class="mt-6">
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
            <option v-for="opt in themeOptions" :key="opt.value" :value="opt.value">
              {{ opt.label }}
            </option>
          </select>
          <p class="mt-2 text-xs text-neutral-500">{{ selectedThemeDescription }}</p>
        </div>
        <div class="border-t border-neutral-200 pt-4">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="text-sm font-medium text-neutral-900">{{ $t("app.theme.customBuilderHeading") }}</p>
              <p class="mt-1 text-xs text-neutral-500">{{ $t("app.theme.customBuilderLead") }}</p>
            </div>
            <button
              type="button"
              class="rounded-full border border-neutral-200 px-3 py-1.5 text-xs font-semibold text-neutral-700 transition hover:bg-neutral-50"
              @click="resetCustomTheme"
            >
              {{ $t("app.theme.resetCustom") }}
            </button>
          </div>
          <div
            class="mt-4 rounded-2xl border border-neutral-200 p-4"
            :style="{ backgroundColor: customTheme.background, color: customTheme.text }"
          >
            <div
              class="rounded-xl border p-3"
              :style="{ backgroundColor: customTheme.surface, borderColor: customTheme.surfaceMuted }"
            >
              <p class="text-sm font-semibold">{{ $t("app.theme.previewTitle") }}</p>
              <p class="mt-1 text-xs" :style="{ color: customTheme.textMuted }">
                {{ $t("app.theme.previewBody") }}
              </p>
              <div class="mt-3 flex flex-wrap gap-2">
                <span
                  class="rounded-full px-3 py-1 text-xs font-semibold"
                  :style="{ backgroundColor: customTheme.accentSoft, color: customTheme.accentText }"
                >
                  {{ $t("app.theme.previewBadge") }}
                </span>
                <span
                  class="rounded-full px-3 py-1 text-xs font-semibold"
                  :style="{ backgroundColor: customTheme.accentStrong, color: customTheme.surface }"
                >
                  {{ $t("app.theme.previewButton") }}
                </span>
              </div>
            </div>
          </div>
          <div class="mt-4 grid gap-3 sm:grid-cols-2">
            <label
              v-for="field in customThemeColorFields"
              :key="field.key"
              class="block rounded-xl border border-neutral-200 bg-white p-3"
            >
              <span class="text-xs font-medium text-neutral-700">{{ field.label }}</span>
              <span class="mt-2 flex items-center gap-2">
                <input
                  type="color"
                  class="h-9 w-11 shrink-0 cursor-pointer rounded-lg border border-neutral-200 bg-white p-1"
                  :value="customTheme[field.key]"
                  @input="updateCustomThemeColor(field.key, ($event.target as HTMLInputElement).value)"
                />
                <input
                  type="text"
                  class="min-w-0 flex-1 rounded-lg border border-neutral-200 bg-white px-2 py-1.5 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
                  :value="customTheme[field.key]"
                  pattern="#?[0-9a-fA-F]{3}([0-9a-fA-F]{3})?"
                  @change="updateCustomThemeColor(field.key, ($event.target as HTMLInputElement).value)"
                />
              </span>
            </label>
          </div>
        </div>
        <div class="border-t border-neutral-200 pt-4">
          <p class="text-sm font-medium text-neutral-900">{{ $t("app.locale.heading") }}</p>
          <select
            class="mt-2 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 transition focus:border-lime-400 focus:ring-2 focus:ring-lime-400/40"
            :value="locale"
            @change="selectLocale(($event.target as HTMLSelectElement).value as AppLocale)"
          >
            <option v-for="opt in localeOptions" :key="opt.value" :value="opt.value">
              {{ opt.label }}
            </option>
          </select>
        </div>
      </div>
    </section>
  </div>
</template>
