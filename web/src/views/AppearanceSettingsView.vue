<script setup lang="ts">
import { computed, inject, onActivated, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import SettingsBackLink from "../components/SettingsBackLink.vue";
import {
  readStoredThemeModePreference,
  readStoredThemePreference,
  THEME_PRESETS,
  type ThemeModePreference,
  type ThemePalette,
  type ThemePreference,
} from "../lib/theme";

const { t } = useI18n();
const setThemePreferenceSilent = inject<(next: ThemePreference) => void>("setThemePreferenceSilent", () => {});
const setThemeModePreferenceSilent = inject<(next: ThemeModePreference) => void>("setThemeModePreferenceSilent", () => {});
const themePreference = ref<ThemePreference>(readStoredThemePreference());
const themeModePreference = ref<ThemeModePreference>(readStoredThemeModePreference());

const themeOptions = computed(() =>
  THEME_PRESETS.map((preset) => ({
    ...preset,
    label: t(`app.theme.presets.${preset.value}.label`),
    description: t(`app.theme.presets.${preset.value}.description`),
  })),
);
const themeModeOptions = computed(() =>
  (["system", "light", "dark"] as const).map((value) => ({
    value,
    label: t(`app.theme.mode.${value}.label`),
    description: t(`app.theme.mode.${value}.description`),
  })),
);
const selectedThemeDescription = computed(() =>
  themeOptions.value.find((opt) => opt.value === themePreference.value)?.description ?? "",
);

function selectTheme(next: ThemePreference) {
  themePreference.value = next;
  setThemePreferenceSilent(next);
}

function selectThemeMode(next: ThemeModePreference) {
  themeModePreference.value = next;
  setThemeModePreferenceSilent(next);
}

function syncThemeFromStorage() {
  themePreference.value = readStoredThemePreference();
  themeModePreference.value = readStoredThemeModePreference();
}

function palettePreviewStyle(palette: ThemePalette) {
  return {
    backgroundColor: palette.background,
    borderColor: palette.frame,
    color: palette.text,
  };
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
          <p class="mt-1 text-xs text-neutral-500">{{ $t("app.theme.headingDescription") }}</p>
          <p class="mt-2 text-xs text-neutral-500">{{ selectedThemeDescription }}</p>
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <button
            v-for="opt in themeOptions"
            :key="opt.value"
            type="button"
            class="rounded-2xl border p-3 text-left transition hover:border-lime-400 hover:bg-neutral-50 focus:outline-none focus:ring-2 focus:ring-lime-400/40"
            :class="themePreference === opt.value ? 'border-lime-500 ring-2 ring-lime-500/20' : 'border-neutral-200'"
            :aria-pressed="themePreference === opt.value"
            @click="selectTheme(opt.value)"
          >
            <span class="flex items-start justify-between gap-3">
              <span>
                <span class="block text-sm font-semibold text-neutral-900">{{ opt.label }}</span>
                <span class="mt-1 block text-xs leading-relaxed text-neutral-500">{{ opt.description }}</span>
              </span>
              <span
                v-if="themePreference === opt.value"
                class="rounded-full bg-lime-600 px-2 py-0.5 text-[11px] font-semibold text-white"
              >
                {{ $t("app.theme.selected") }}
              </span>
            </span>
            <span class="mt-3 grid grid-cols-2 gap-2" aria-hidden="true">
              <span class="rounded-xl border p-2" :style="palettePreviewStyle(opt.light)">
                <span class="mb-2 block text-[11px] font-semibold">{{ $t("app.theme.lightPreview") }}</span>
                <span class="flex gap-1.5">
                  <span class="h-5 flex-1 rounded-full" :style="{ backgroundColor: opt.light.surfaceMuted }" />
                  <span class="h-5 w-8 rounded-full" :style="{ backgroundColor: opt.light.accentStrong }" />
                </span>
              </span>
              <span class="rounded-xl border p-2" :style="palettePreviewStyle(opt.dark)">
                <span class="mb-2 block text-[11px] font-semibold">{{ $t("app.theme.darkPreview") }}</span>
                <span class="flex gap-1.5">
                  <span class="h-5 flex-1 rounded-full" :style="{ backgroundColor: opt.dark.surfaceMuted }" />
                  <span class="h-5 w-8 rounded-full" :style="{ backgroundColor: opt.dark.accentStrong }" />
                </span>
              </span>
            </span>
          </button>
        </div>

        <div class="border-t border-neutral-200 pt-4">
          <p class="text-sm font-medium text-neutral-900">{{ $t("app.theme.modeHeading") }}</p>
          <p class="mt-1 text-xs text-neutral-500">{{ $t("app.theme.modeLead") }}</p>
          <div class="mt-3 grid gap-2 sm:grid-cols-3">
            <button
              v-for="opt in themeModeOptions"
              :key="opt.value"
              type="button"
              class="rounded-xl border p-3 text-left transition hover:border-lime-400 hover:bg-neutral-50 focus:outline-none focus:ring-2 focus:ring-lime-400/40"
              :class="themeModePreference === opt.value ? 'border-lime-500 ring-2 ring-lime-500/20' : 'border-neutral-200'"
              :aria-pressed="themeModePreference === opt.value"
              @click="selectThemeMode(opt.value)"
            >
              <span class="block text-sm font-semibold text-neutral-900">{{ opt.label }}</span>
              <span class="mt-1 block text-xs leading-relaxed text-neutral-500">{{ opt.description }}</span>
            </button>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
