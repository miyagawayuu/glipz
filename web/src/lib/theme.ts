export type ThemePreference = "system" | "light" | "dark" | "custom";
export type ResolvedTheme = "light" | "dark";
export type CustomTheme = {
  background: string;
  surface: string;
  surfaceMuted: string;
  text: string;
  textMuted: string;
  accentSoft: string;
  accentStrong: string;
  accentText: string;
  accentTextStrong: string;
};

const THEME_STORAGE_KEY = "glipz-theme";
const CUSTOM_THEME_STORAGE_KEY = "glipz-custom-theme";
const DARK_MEDIA_QUERY = "(prefers-color-scheme: dark)";

export const DEFAULT_CUSTOM_THEME: CustomTheme = {
  background: "#ffffff",
  surface: "#ffffff",
  surfaceMuted: "#f5f5f5",
  text: "#111827",
  textMuted: "#525252",
  accentSoft: "#ecfccb",
  accentStrong: "#84cc16",
  accentText: "#3f6212",
  accentTextStrong: "#365314",
};

const CUSTOM_THEME_CSS_VARS: Record<keyof CustomTheme, string> = {
  background: "--theme-bg",
  surface: "--theme-surface",
  surfaceMuted: "--theme-surface-muted",
  text: "--theme-text",
  textMuted: "--theme-text-muted",
  accentSoft: "--theme-accent-bg",
  accentStrong: "--theme-accent-bg-strong",
  accentText: "--theme-accent-text",
  accentTextStrong: "--theme-accent-text-strong",
};

function isThemePreference(value: string | null): value is ThemePreference {
  return value === "system" || value === "light" || value === "dark" || value === "custom";
}

function normalizeHexColor(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  const match = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(trimmed);
  if (!match) return null;
  const raw = match[1];
  if (raw.length === 3) {
    return `#${raw.split("").map((ch) => `${ch}${ch}`).join("")}`.toLowerCase();
  }
  return `#${raw}`.toLowerCase();
}

function hexToRgbTriplet(hex: string): string {
  const normalized = normalizeHexColor(hex) ?? "#000000";
  const raw = normalized.slice(1);
  return `${parseInt(raw.slice(0, 2), 16)} ${parseInt(raw.slice(2, 4), 16)} ${parseInt(raw.slice(4, 6), 16)}`;
}

function relativeLuminance(hex: string): number {
  const normalized = normalizeHexColor(hex) ?? "#ffffff";
  const raw = normalized.slice(1);
  const channels = [raw.slice(0, 2), raw.slice(2, 4), raw.slice(4, 6)].map((part) => {
    const value = parseInt(part, 16) / 255;
    return value <= 0.03928 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function normalizeCustomTheme(value: unknown): CustomTheme {
  if (!isPlainObject(value)) return { ...DEFAULT_CUSTOM_THEME };
  const next = { ...DEFAULT_CUSTOM_THEME };
  for (const key of Object.keys(DEFAULT_CUSTOM_THEME) as Array<keyof CustomTheme>) {
    next[key] = normalizeHexColor(value[key]) ?? DEFAULT_CUSTOM_THEME[key];
  }
  return next;
}

export function readStoredThemePreference(): ThemePreference {
  if (typeof window === "undefined") return "system";
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  return isThemePreference(stored) ? stored : "system";
}

export function persistThemePreference(value: ThemePreference): void {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(THEME_STORAGE_KEY, value);
}

export function readStoredCustomTheme(): CustomTheme {
  if (typeof window === "undefined") return { ...DEFAULT_CUSTOM_THEME };
  try {
    return normalizeCustomTheme(JSON.parse(window.localStorage.getItem(CUSTOM_THEME_STORAGE_KEY) ?? "null"));
  } catch {
    return { ...DEFAULT_CUSTOM_THEME };
  }
}

export function persistCustomTheme(value: CustomTheme): void {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(CUSTOM_THEME_STORAGE_KEY, JSON.stringify(normalizeCustomTheme(value)));
}

export function systemThemeMediaQuery(): MediaQueryList | null {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return null;
  return window.matchMedia(DARK_MEDIA_QUERY);
}

export function resolveTheme(value: ThemePreference): ResolvedTheme {
  if (value === "light") return "light";
  if (value === "dark") return "dark";
  if (value === "custom") return relativeLuminance(readStoredCustomTheme().background) < 0.4 ? "dark" : "light";
  return systemThemeMediaQuery()?.matches ? "dark" : "light";
}

export function applyTheme(value: ThemePreference): ResolvedTheme {
  const resolved = resolveTheme(value);
  if (typeof document !== "undefined") {
    document.documentElement.classList.toggle("dark", resolved === "dark");
    document.documentElement.classList.toggle("custom-theme", value === "custom");
    document.documentElement.style.colorScheme = resolved;
    for (const variable of Object.values(CUSTOM_THEME_CSS_VARS)) {
      document.documentElement.style.removeProperty(variable);
    }
    if (value === "custom") {
      const customTheme = readStoredCustomTheme();
      for (const [key, variable] of Object.entries(CUSTOM_THEME_CSS_VARS) as Array<[keyof CustomTheme, string]>) {
        document.documentElement.style.setProperty(variable, hexToRgbTriplet(customTheme[key]));
      }
      document.documentElement.style.setProperty("--color-frame", hexToRgbTriplet(customTheme.surfaceMuted));
    } else {
      document.documentElement.style.removeProperty("--color-frame");
    }
  }
  return resolved;
}

export function initTheme(): ThemePreference {
  const value = readStoredThemePreference();
  applyTheme(value);
  return value;
}
