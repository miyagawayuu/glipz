import { shallowRef, type Component, type Ref } from "vue";
import type { RouteLocationNormalizedLoaded } from "vue-router";
import { useSidebarWidgetSettings } from "./sidebarWidgetSettings";

export type SidebarWidgetPlacement = "right-sidebar";

export interface SidebarWidgetViewer {
  id: string;
  email: string;
  handle: string;
  display_name: string;
  avatar_url: string | null;
  is_site_admin?: boolean;
  fanclub_patreon_enabled?: boolean;
}

export interface SidebarWidgetContext {
  placement: SidebarWidgetPlacement;
  route: RouteLocationNormalizedLoaded;
  viewer: SidebarWidgetViewer | null;
  appVersion: string;
}

export interface SidebarWidgetDefinition {
  id: string;
  title: string;
  titleKey?: string;
  description?: string;
  descriptionKey?: string;
  placement?: SidebarWidgetPlacement;
  order?: number;
  component: Component;
  props?: Record<string, unknown> | ((context: SidebarWidgetContext) => Record<string, unknown>);
  enabled?: (context: SidebarWidgetContext) => boolean;
}

export interface SidebarWidgetPlugin {
  id: string;
  name: string;
  nameKey?: string;
  description?: string;
  descriptionKey?: string;
  widgets: SidebarWidgetDefinition[];
}

export interface RegisteredSidebarWidget extends SidebarWidgetDefinition {
  pluginId: string;
  pluginName: string;
  pluginNameKey?: string;
  pluginOrder: number;
  placement: SidebarWidgetPlacement;
}

const sidebarWidgetPlugins = shallowRef<SidebarWidgetPlugin[]>([]);
const sidebarWidgetSettings = useSidebarWidgetSettings();

function pluginSetting(pluginId: string) {
  return sidebarWidgetSettings.value.plugins.find((item) => item.id === pluginId);
}

function normalizedWidget(plugin: SidebarWidgetPlugin, widget: SidebarWidgetDefinition, pluginIndex: number): RegisteredSidebarWidget {
  const setting = pluginSetting(plugin.id);
  const defaultOrder = sidebarWidgetSettings.value.plugins.length + pluginIndex;
  return {
    ...widget,
    pluginId: plugin.id,
    pluginName: plugin.name,
    pluginNameKey: plugin.nameKey,
    pluginOrder: setting?.order ?? defaultOrder,
    placement: widget.placement ?? "right-sidebar",
  };
}

function sortWidgets(a: RegisteredSidebarWidget, b: RegisteredSidebarWidget): number {
  const pluginOrderDiff = a.pluginOrder - b.pluginOrder;
  if (pluginOrderDiff !== 0) return pluginOrderDiff;
  const orderDiff = (a.order ?? 100) - (b.order ?? 100);
  if (orderDiff !== 0) return orderDiff;
  return `${a.pluginName}:${a.title}`.localeCompare(`${b.pluginName}:${b.title}`);
}

export function registerSidebarWidgetPlugin(plugin: SidebarWidgetPlugin): () => void {
  const next = sidebarWidgetPlugins.value.filter((item) => item.id !== plugin.id);
  sidebarWidgetPlugins.value = [...next, plugin];

  return () => {
    sidebarWidgetPlugins.value = sidebarWidgetPlugins.value.filter((item) => item.id !== plugin.id);
  };
}

export function useSidebarWidgetPlugins(): Readonly<Ref<readonly SidebarWidgetPlugin[]>> {
  return sidebarWidgetPlugins;
}

export function getSidebarWidgets(context: SidebarWidgetContext): RegisteredSidebarWidget[] {
  return sidebarWidgetPlugins.value
    .filter((plugin) => pluginSetting(plugin.id)?.enabled ?? false)
    .flatMap((plugin, pluginIndex) => plugin.widgets.map((widget) => normalizedWidget(plugin, widget, pluginIndex)))
    .filter((widget) => widget.placement === context.placement)
    .filter((widget) => !widget.enabled || widget.enabled(context))
    .sort(sortWidgets);
}
