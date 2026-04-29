import { api } from "./api";

export type BuiltInTimelineID = "all" | "recommended" | "following";
export type TimelineSource = BuiltInTimelineID;
export type TimelineSort = "recent" | "recommended";
export type TimelineDefinitionKind = "builtin" | "custom";

export type TimelineFilters = {
  baseScope: TimelineSource;
  keywords: string[];
  includeUsers: string[];
  excludeUsers: string[];
  communities: string[];
  includeReposts: boolean;
  includeFederated: boolean;
  includeNsfw: boolean;
};

export type TimelineDefinition = {
  id: string;
  kind: TimelineDefinitionKind;
  label: string;
  enabled: boolean;
  filters: TimelineFilters;
  sort: TimelineSort;
};

export type TimelineSettings = {
  version: 1;
  defaultTimelineId: string;
  timelines: TimelineDefinition[];
};

export const TIMELINE_SETTINGS_STORAGE_KEY = "glipz.timelineSettings";
const TIMELINE_SETTINGS_MIGRATED_STORAGE_KEY = "glipz.timelineSettings.migrated";

export const DEFAULT_TIMELINE_SETTINGS: TimelineSettings = {
  version: 1,
  defaultTimelineId: "all",
  timelines: [
    {
      id: "all",
      kind: "builtin",
      label: "views.feed.scopeAll",
      enabled: true,
      sort: "recent",
      filters: {
        baseScope: "all",
        keywords: [],
        includeUsers: [],
        excludeUsers: [],
        communities: [],
        includeReposts: true,
        includeFederated: true,
        includeNsfw: true,
      },
    },
    {
      id: "recommended",
      kind: "builtin",
      label: "views.feed.scopeRecommended",
      enabled: true,
      sort: "recommended",
      filters: {
        baseScope: "recommended",
        keywords: [],
        includeUsers: [],
        excludeUsers: [],
        communities: [],
        includeReposts: true,
        includeFederated: true,
        includeNsfw: true,
      },
    },
    {
      id: "following",
      kind: "builtin",
      label: "views.feed.scopeFollowing",
      enabled: true,
      sort: "recent",
      filters: {
        baseScope: "following",
        keywords: [],
        includeUsers: [],
        excludeUsers: [],
        communities: [],
        includeReposts: true,
        includeFederated: true,
        includeNsfw: true,
      },
    },
  ],
};

const BUILTIN_IDS: BuiltInTimelineID[] = ["all", "recommended", "following"];

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function normalizeID(value: unknown, fallback: string): string {
  const raw = typeof value === "string" ? value.trim() : "";
  if (!raw) return fallback;
  return raw.replace(/[^a-zA-Z0-9_-]/g, "").slice(0, 48) || fallback;
}

function normalizeLabel(value: unknown, fallback: string): string {
  const raw = typeof value === "string" ? value.trim() : "";
  return (raw || fallback).slice(0, 40);
}

function normalizeStringList(value: unknown, limit = 16): string[] {
  if (!Array.isArray(value)) return [];
  const out: string[] = [];
  for (const item of value) {
    const normalized = String(item ?? "").trim().replace(/^@/, "").slice(0, 80);
    if (normalized && !out.includes(normalized)) out.push(normalized);
    if (out.length >= limit) break;
  }
  return out;
}

function normalizeBaseScope(value: unknown): TimelineSource {
  return value === "following" || value === "recommended" || value === "all" ? value : "all";
}

function normalizeSort(value: unknown, baseScope: TimelineSource): TimelineSort {
  if (value === "recommended" || baseScope === "recommended") return "recommended";
  return "recent";
}

export function createTimelineID(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `custom-${crypto.randomUUID().slice(0, 8)}`;
  }
  return `custom-${Math.random().toString(36).slice(2, 10)}`;
}

export function defaultTimelineFilters(baseScope: TimelineSource = "all"): TimelineFilters {
  return {
    baseScope,
    keywords: [],
    includeUsers: [],
    excludeUsers: [],
    communities: [],
    includeReposts: true,
    includeFederated: true,
    includeNsfw: true,
  };
}

export function createCustomTimeline(label: string): TimelineDefinition {
  return {
    id: createTimelineID(),
    kind: "custom",
    label: normalizeLabel(label, "Custom"),
    enabled: true,
    sort: "recent",
    filters: defaultTimelineFilters("all"),
  };
}

export function timelineDisplayLabel(definition: TimelineDefinition, translate: (key: string) => string): string {
  return definition.kind === "builtin" ? translate(definition.label) : definition.label;
}

function normalizeTimelineDefinition(value: unknown, fallback?: TimelineDefinition): TimelineDefinition | null {
  if (!isPlainObject(value)) return fallback ? { ...fallback, filters: { ...fallback.filters } } : null;
  const fallbackID = fallback?.id ?? createTimelineID();
  const id = normalizeID(value.id, fallbackID);
  const builtin = BUILTIN_IDS.includes(id as BuiltInTimelineID);
  const kind: TimelineDefinitionKind = builtin ? "builtin" : "custom";
  const baseScope = normalizeBaseScope(isPlainObject(value.filters) ? value.filters.baseScope : undefined);
  const filters: TimelineFilters = {
    baseScope: builtin ? (id as BuiltInTimelineID) : baseScope,
    keywords: normalizeStringList(isPlainObject(value.filters) ? value.filters.keywords : undefined),
    includeUsers: normalizeStringList(isPlainObject(value.filters) ? value.filters.includeUsers : undefined),
    excludeUsers: normalizeStringList(isPlainObject(value.filters) ? value.filters.excludeUsers : undefined),
    communities: normalizeStringList(isPlainObject(value.filters) ? value.filters.communities : undefined),
    includeReposts: isPlainObject(value.filters) ? value.filters.includeReposts !== false : true,
    includeFederated: isPlainObject(value.filters) ? value.filters.includeFederated !== false : true,
    includeNsfw: isPlainObject(value.filters) ? value.filters.includeNsfw !== false : true,
  };
  return {
    id,
    kind,
    label: kind === "builtin" ? (fallback?.label ?? `views.feed.scope${id[0].toUpperCase()}${id.slice(1)}`) : normalizeLabel(value.label, fallback?.label ?? "Custom"),
    enabled: value.enabled !== false,
    filters,
    sort: normalizeSort(value.sort, filters.baseScope),
  };
}

export function normalizeTimelineSettings(value: unknown): TimelineSettings {
  const defaults = DEFAULT_TIMELINE_SETTINGS;
  if (!isPlainObject(value)) return structuredClone(defaults);
  const raw = Array.isArray(value.timelines) ? value.timelines : [];
  const byID = new Map<string, TimelineDefinition>();
  for (const fallback of defaults.timelines) {
    const rawDefinition = raw.find((item) => isPlainObject(item) && item.id === fallback.id);
    const normalized = normalizeTimelineDefinition(rawDefinition, fallback) ?? fallback;
    byID.set(fallback.id, normalized);
  }
  for (const rawDefinition of raw) {
    const normalized = normalizeTimelineDefinition(rawDefinition);
    if (!normalized || normalized.kind === "builtin") continue;
    byID.set(normalized.id, normalized);
  }
  const timelines = Array.from(byID.values());
  const defaultTimelineId =
    typeof value.defaultTimelineId === "string" && timelines.some((item) => item.id === value.defaultTimelineId && item.enabled)
      ? value.defaultTimelineId
      : (timelines.find((item) => item.enabled)?.id ?? "all");
  return { version: 1, defaultTimelineId, timelines };
}

export function readTimelineSettings(): TimelineSettings {
  if (typeof window === "undefined") return structuredClone(DEFAULT_TIMELINE_SETTINGS);
  try {
    return normalizeTimelineSettings(JSON.parse(window.localStorage.getItem(TIMELINE_SETTINGS_STORAGE_KEY) ?? "null"));
  } catch {
    return structuredClone(DEFAULT_TIMELINE_SETTINGS);
  }
}

export function persistTimelineSettings(settings: TimelineSettings): void {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(TIMELINE_SETTINGS_STORAGE_KEY, JSON.stringify(normalizeTimelineSettings(settings)));
}

function readLocalTimelineSettingsForMigration(): TimelineSettings | null {
  if (typeof window === "undefined") return null;
  if (window.localStorage.getItem(TIMELINE_SETTINGS_MIGRATED_STORAGE_KEY) === "1") return null;
  const raw = window.localStorage.getItem(TIMELINE_SETTINGS_STORAGE_KEY);
  if (!raw) return null;
  try {
    return normalizeTimelineSettings(JSON.parse(raw));
  } catch {
    return null;
  }
}

function markLocalTimelineSettingsMigrated() {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(TIMELINE_SETTINGS_MIGRATED_STORAGE_KEY, "1");
  window.localStorage.removeItem(TIMELINE_SETTINGS_STORAGE_KEY);
}

export async function saveTimelineSettings(token: string, settings: TimelineSettings): Promise<TimelineSettings> {
  const normalized = normalizeTimelineSettings(settings);
  const res = await api<{ settings: unknown }>("/api/v1/me/timeline-settings", {
    method: "PUT",
    token,
    json: { settings: normalized },
  });
  return normalizeTimelineSettings(res.settings);
}

export async function fetchTimelineSettings(token: string): Promise<TimelineSettings> {
  const res = await api<{ settings: unknown | null }>("/api/v1/me/timeline-settings", {
    method: "GET",
    token,
  });
  if (res.settings) return normalizeTimelineSettings(res.settings);
  const local = readLocalTimelineSettingsForMigration();
  if (local) {
    const saved = await saveTimelineSettings(token, local);
    markLocalTimelineSettingsMigrated();
    return saved;
  }
  return structuredClone(DEFAULT_TIMELINE_SETTINGS);
}

export async function resetTimelineSettingsOnServer(token: string): Promise<TimelineSettings> {
  await api("/api/v1/me/timeline-settings", {
    method: "DELETE",
    token,
  });
  return structuredClone(DEFAULT_TIMELINE_SETTINGS);
}

export function resetTimelineSettings(): TimelineSettings {
  const next = structuredClone(DEFAULT_TIMELINE_SETTINGS);
  persistTimelineSettings(next);
  return next;
}

export function enabledTimelines(settings: TimelineSettings): TimelineDefinition[] {
  const enabled = settings.timelines.filter((timeline) => timeline.enabled);
  return enabled.length ? enabled : DEFAULT_TIMELINE_SETTINGS.timelines.filter((timeline) => timeline.enabled);
}
