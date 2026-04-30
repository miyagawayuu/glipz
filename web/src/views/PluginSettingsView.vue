<script setup lang="ts">
import { computed } from "vue";
import { RouterLink } from "vue-router";
import { useI18n } from "vue-i18n";
import { useSidebarWidgetPlugins } from "../lib/sidebarWidgetPlugins";
import {
  reorderSidebarPlugins,
  upsertSidebarPluginSetting,
  useSidebarWidgetSettings,
} from "../lib/sidebarWidgetSettings";

const { t } = useI18n();
const registeredPlugins = useSidebarWidgetPlugins();
const widgetSettings = useSidebarWidgetSettings();

const plugins = computed(() =>
  registeredPlugins.value
    .map((plugin, index) => {
      const setting = widgetSettings.value.plugins.find((item) => item.id === plugin.id);
      return {
        ...plugin,
        enabled: setting?.enabled ?? false,
        order: setting?.order ?? widgetSettings.value.plugins.length + index,
      };
    })
    .sort((a, b) => {
      const orderDiff = a.order - b.order;
      if (orderDiff !== 0) return orderDiff;
      return pluginName(a).localeCompare(pluginName(b));
    }),
);

function pluginName(plugin: { name: string; nameKey?: string }): string {
  return plugin.nameKey ? t(plugin.nameKey) : plugin.name;
}

function pluginDescription(plugin: { description?: string; descriptionKey?: string }): string {
  if (plugin.descriptionKey) return t(plugin.descriptionKey);
  return plugin.description ?? "";
}

function togglePlugin(pluginID: string, enabled: boolean) {
  upsertSidebarPluginSetting(pluginID, { enabled });
}

function movePlugin(pluginID: string, direction: -1 | 1) {
  const ids = plugins.value.map((plugin) => plugin.id);
  const index = ids.indexOf(pluginID);
  const nextIndex = index + direction;
  if (index < 0 || nextIndex < 0 || nextIndex >= ids.length) return;
  const [id] = ids.splice(index, 1);
  ids.splice(nextIndex, 0, id);
  reorderSidebarPlugins(ids);
}
</script>

<template>
  <div class="w-full px-4 py-8">
    <RouterLink to="/settings" class="text-sm font-medium text-lime-800 hover:underline">
      {{ $t("views.settings.backToSettings") }}
    </RouterLink>
    <div class="mt-4">
      <h1 class="text-2xl font-bold text-neutral-900">{{ $t("views.pluginSettings.title") }}</h1>
      <p class="mt-2 text-sm leading-relaxed text-neutral-600">{{ $t("views.pluginSettings.lead") }}</p>
    </div>

    <section class="mt-6 space-y-4">
      <p v-if="!plugins.length" class="rounded-2xl border border-neutral-200 bg-white p-4 text-sm text-neutral-500 shadow-sm">
        {{ $t("views.pluginSettings.empty") }}
      </p>

      <article
        v-for="(plugin, index) in plugins"
        :key="plugin.id"
        class="rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm"
      >
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-base font-semibold text-neutral-900">{{ pluginName(plugin) }}</h2>
              <span
                class="rounded-full px-2 py-0.5 text-[11px] font-semibold"
                :class="plugin.enabled ? 'bg-lime-100 text-lime-800' : 'bg-neutral-100 text-neutral-500'"
              >
                {{ plugin.enabled ? $t("views.pluginSettings.enabledBadge") : $t("views.pluginSettings.disabledBadge") }}
              </span>
            </div>
            <p class="mt-1 text-xs text-neutral-500">{{ plugin.id }}</p>
            <p v-if="pluginDescription(plugin)" class="mt-2 text-sm leading-relaxed text-neutral-600">
              {{ pluginDescription(plugin) }}
            </p>
            <p class="mt-2 text-xs text-neutral-500">
              {{ $t("views.pluginSettings.widgetCount", { count: plugin.widgets.length }) }}
            </p>
          </div>

          <div class="flex flex-wrap gap-2">
            <button
              type="button"
              class="rounded-full border border-neutral-200 px-3 py-1.5 text-xs font-semibold text-neutral-700 hover:bg-neutral-50 disabled:opacity-40"
              :disabled="index === 0"
              @click="movePlugin(plugin.id, -1)"
            >
              {{ $t("views.pluginSettings.moveUp") }}
            </button>
            <button
              type="button"
              class="rounded-full border border-neutral-200 px-3 py-1.5 text-xs font-semibold text-neutral-700 hover:bg-neutral-50 disabled:opacity-40"
              :disabled="index === plugins.length - 1"
              @click="movePlugin(plugin.id, 1)"
            >
              {{ $t("views.pluginSettings.moveDown") }}
            </button>
          </div>
        </div>

        <label class="mt-4 inline-flex items-center gap-2 text-sm text-neutral-800">
          <input
            type="checkbox"
            class="h-4 w-4 rounded border-neutral-200 text-lime-600"
            :checked="plugin.enabled"
            @change="togglePlugin(plugin.id, ($event.target as HTMLInputElement).checked)"
          />
          <span>{{ $t("views.pluginSettings.showInSidebar") }}</span>
        </label>
      </article>
    </section>
  </div>
</template>
