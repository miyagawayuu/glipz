<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { APP_VERSION } from "../lib/appInfo";
import {
  getSidebarWidgets,
  useSidebarWidgetPlugins,
  type SidebarWidgetContext,
  type SidebarWidgetPlacement,
  type SidebarWidgetViewer,
  type RegisteredSidebarWidget,
} from "../lib/sidebarWidgetPlugins";
import { setSidebarWidgetCollapsed, useSidebarWidgetSettings } from "../lib/sidebarWidgetSettings";

const props = defineProps<{
  placement: SidebarWidgetPlacement;
  viewer: SidebarWidgetViewer | null;
}>();

const route = useRoute();
const { t } = useI18n();
const pluginRegistry = useSidebarWidgetPlugins();
const widgetSettings = useSidebarWidgetSettings();

const widgetContext = computed<SidebarWidgetContext>(() => ({
  placement: props.placement,
  route,
  viewer: props.viewer,
  appVersion: APP_VERSION,
}));

const widgets = computed(() => {
  pluginRegistry.value;
  widgetSettings.value;
  return getSidebarWidgets(widgetContext.value);
});

function widgetKey(widget: RegisteredSidebarWidget): string {
  return `${widget.pluginId}:${widget.id}`;
}

function widgetPanelID(widget: RegisteredSidebarWidget): string {
  return `sidebar-widget-${widgetKey(widget).replace(/[^a-zA-Z0-9_-]/g, "-")}`;
}

function widgetTitle(widget: RegisteredSidebarWidget): string {
  return widget.titleKey ? t(widget.titleKey) : widget.title;
}

function widgetPluginName(widget: RegisteredSidebarWidget): string {
  return widget.pluginNameKey ? t(widget.pluginNameKey) : widget.pluginName;
}

function isCollapsed(widget: RegisteredSidebarWidget): boolean {
  return widgetSettings.value.collapsedWidgets.includes(widgetKey(widget));
}

function toggleCollapsed(widget: RegisteredSidebarWidget) {
  setSidebarWidgetCollapsed(widgetKey(widget), !isCollapsed(widget));
}

function widgetProps(widget: RegisteredSidebarWidget): Record<string, unknown> {
  if (typeof widget.props === "function") return widget.props(widgetContext.value);
  return widget.props ?? {};
}
</script>

<template>
  <section v-if="widgets.length" class="flex flex-col gap-3" :aria-label="$t('app.sidebarWidgets.region')">
    <article
      v-for="widget in widgets"
      :key="widgetKey(widget)"
      class="overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm"
    >
      <button
        type="button"
        class="flex w-full items-center justify-between gap-3 px-3 py-3 text-left hover:bg-neutral-50"
        :aria-expanded="!isCollapsed(widget)"
        :aria-controls="widgetPanelID(widget)"
        @click="toggleCollapsed(widget)"
      >
        <span class="min-w-0">
          <span class="block truncate text-sm font-semibold text-neutral-900">{{ widgetTitle(widget) }}</span>
          <span class="block truncate text-[11px] text-neutral-400">{{ widgetPluginName(widget) }}</span>
        </span>
        <span
          class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-neutral-200 text-base leading-none text-neutral-500"
          aria-hidden="true"
        >
          {{ isCollapsed(widget) ? "+" : "−" }}
        </span>
        <span class="sr-only">
          {{ isCollapsed(widget) ? $t("app.sidebarWidgets.expand") : $t("app.sidebarWidgets.collapse") }}
        </span>
      </button>
      <div
        v-show="!isCollapsed(widget)"
        :id="widgetPanelID(widget)"
        class="border-t border-neutral-100"
      >
        <component :is="widget.component" v-bind="widgetProps(widget)" />
      </div>
    </article>
  </section>
</template>
