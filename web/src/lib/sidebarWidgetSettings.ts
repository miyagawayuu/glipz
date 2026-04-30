import { ref, type Ref } from "vue";

export type SidebarPluginSetting = {
  id: string;
  enabled: boolean;
  order: number;
};

export type SidebarWidgetSettings = {
  version: 1;
  plugins: SidebarPluginSetting[];
  collapsedWidgets: string[];
};

export const SIDEBAR_WIDGET_SETTINGS_STORAGE_KEY = "glipz.sidebarWidgetSettings";

const settingsRef = ref<SidebarWidgetSettings>(readSidebarWidgetSettings());

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function normalizePluginID(value: unknown): string {
  const raw = typeof value === "string" ? value.trim() : "";
  return raw.slice(0, 120);
}

function normalizeOrder(value: unknown, fallback: number): number {
  const next = typeof value === "number" ? value : Number(value);
  if (!Number.isFinite(next)) return fallback;
  return Math.max(0, Math.round(next));
}

function normalizeWidgetKey(value: unknown): string {
  const raw = typeof value === "string" ? value.trim() : "";
  return raw.slice(0, 160);
}

export function normalizeSidebarWidgetSettings(value: unknown): SidebarWidgetSettings {
  const rawPlugins = isPlainObject(value) && Array.isArray(value.plugins) ? value.plugins : [];
  const rawCollapsedWidgets = isPlainObject(value) && Array.isArray(value.collapsedWidgets) ? value.collapsedWidgets : [];
  const seen = new Set<string>();
  const plugins: SidebarPluginSetting[] = [];
  const collapsedWidgets: string[] = [];

  rawPlugins.forEach((item, index) => {
    if (!isPlainObject(item)) return;
    const id = normalizePluginID(item.id);
    if (!id || seen.has(id)) return;
    seen.add(id);
    plugins.push({
      id,
      enabled: item.enabled !== false,
      order: normalizeOrder(item.order, index),
    });
  });

  for (const item of rawCollapsedWidgets) {
    const key = normalizeWidgetKey(item);
    if (key && !collapsedWidgets.includes(key)) collapsedWidgets.push(key);
  }

  return { version: 1, plugins, collapsedWidgets };
}

export function readSidebarWidgetSettings(): SidebarWidgetSettings {
  if (typeof window === "undefined") return { version: 1, plugins: [], collapsedWidgets: [] };
  try {
    return normalizeSidebarWidgetSettings(
      JSON.parse(window.localStorage.getItem(SIDEBAR_WIDGET_SETTINGS_STORAGE_KEY) ?? "null"),
    );
  } catch {
    return { version: 1, plugins: [], collapsedWidgets: [] };
  }
}

export function persistSidebarWidgetSettings(settings: SidebarWidgetSettings): SidebarWidgetSettings {
  const normalized = normalizeSidebarWidgetSettings(settings);
  settingsRef.value = normalized;
  if (typeof window !== "undefined") {
    window.localStorage.setItem(SIDEBAR_WIDGET_SETTINGS_STORAGE_KEY, JSON.stringify(normalized));
  }
  return normalized;
}

export function useSidebarWidgetSettings(): Readonly<Ref<SidebarWidgetSettings>> {
  return settingsRef;
}

export function upsertSidebarPluginSetting(pluginID: string, patch: Partial<Omit<SidebarPluginSetting, "id">>): SidebarWidgetSettings {
  const current = settingsRef.value.plugins;
  const existing = current.find((item) => item.id === pluginID);
  const fallback: SidebarPluginSetting = {
    id: pluginID,
    enabled: false,
    order: current.length,
  };
  const nextPlugin = { ...(existing ?? fallback), ...patch, id: pluginID };
  const plugins = existing
    ? current.map((item) => (item.id === pluginID ? nextPlugin : item))
    : [...current, nextPlugin];
  return persistSidebarWidgetSettings({ ...settingsRef.value, plugins });
}

export function reorderSidebarPlugins(pluginIDs: string[]): SidebarWidgetSettings {
  const known = new Set(pluginIDs);
  const ordered = pluginIDs.map((id, order) => {
    const existing = settingsRef.value.plugins.find((item) => item.id === id);
    return {
      id,
      enabled: existing?.enabled ?? false,
      order,
    };
  });
  const unknown = settingsRef.value.plugins
    .filter((item) => !known.has(item.id))
    .map((item, index) => ({ ...item, order: ordered.length + index }));

  return persistSidebarWidgetSettings({ ...settingsRef.value, plugins: [...ordered, ...unknown] });
}

export function setSidebarWidgetCollapsed(widgetKey: string, collapsed: boolean): SidebarWidgetSettings {
  const normalizedKey = normalizeWidgetKey(widgetKey);
  if (!normalizedKey) return settingsRef.value;
  const current = settingsRef.value.collapsedWidgets;
  const collapsedWidgets = collapsed
    ? current.includes(normalizedKey) ? current : [...current, normalizedKey]
    : current.filter((item) => item !== normalizedKey);
  return persistSidebarWidgetSettings({ ...settingsRef.value, collapsedWidgets });
}

if (typeof window !== "undefined") {
  window.addEventListener("storage", (event) => {
    if (event.key !== null && event.key !== SIDEBAR_WIDGET_SETTINGS_STORAGE_KEY) return;
    settingsRef.value = readSidebarWidgetSettings();
  });
}
