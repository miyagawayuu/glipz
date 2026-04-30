export type ThemePreference = "default" | "pink" | "orange" | "blue" | "violet";
export type ThemeModePreference = "system" | "light" | "dark";
export type ResolvedTheme = "light" | "dark";
export type ThemePalette = {
  background: string;
  surface: string;
  surfaceMuted: string;
  text: string;
  textMuted: string;
  accentSoft: string;
  accentStrong: string;
  accentHover: string;
  accentContrast: string;
  accentText: string;
  accentTextStrong: string;
  frame: string;
};
export type ThemePreset = {
  value: ThemePreference;
  light: ThemePalette;
  dark: ThemePalette;
};

const THEME_STORAGE_KEY = "glipz-theme";
const THEME_MODE_STORAGE_KEY = "glipz-theme-mode";
const DARK_MEDIA_QUERY = "(prefers-color-scheme: dark)";

const THEME_CSS_VARS: Record<keyof ThemePalette, string> = {
  background: "--theme-bg",
  surface: "--theme-surface",
  surfaceMuted: "--theme-surface-muted",
  text: "--theme-text",
  textMuted: "--theme-text-muted",
  accentSoft: "--theme-accent-bg",
  accentStrong: "--theme-accent-bg-strong",
  accentHover: "--theme-accent-bg-hover",
  accentContrast: "--theme-accent-contrast",
  accentText: "--theme-accent-text",
  accentTextStrong: "--theme-accent-text-strong",
  frame: "--color-frame",
};

export const THEME_PRESETS: readonly ThemePreset[] = [
  {
    value: "default",
    light: {
      background: "#ffffff",
      surface: "#ffffff",
      surfaceMuted: "#f5f5f5",
      text: "#111827",
      textMuted: "#525252",
      accentSoft: "#ecfccb",
      accentStrong: "#84cc16",
      accentHover: "#4d7c0f",
      accentContrast: "#1f2a0c",
      accentText: "#3f6212",
      accentTextStrong: "#365314",
      frame: "#e5e5e5",
    },
    dark: {
      background: "#171717",
      surface: "#262626",
      surfaceMuted: "#262626",
      text: "#fafafa",
      textMuted: "#d4d4d4",
      accentSoft: "#365314",
      accentStrong: "#84cc16",
      accentHover: "#4d7c0f",
      accentContrast: "#1f2a0c",
      accentText: "#bef264",
      accentTextStrong: "#d9f99d",
      frame: "#404040",
    },
  },
  {
    value: "pink",
    light: {
      background: "#fff7fb",
      surface: "#ffffff",
      surfaceMuted: "#fdf2f8",
      text: "#18181b",
      textMuted: "#6b5562",
      accentSoft: "#fce7f3",
      accentStrong: "#db2777",
      accentHover: "#be185d",
      accentContrast: "#ffffff",
      accentText: "#9d174d",
      accentTextStrong: "#831843",
      frame: "#fbcfe8",
    },
    dark: {
      background: "#1f1119",
      surface: "#2a1722",
      surfaceMuted: "#3a1a2b",
      text: "#fff7fb",
      textMuted: "#f0c9dc",
      accentSoft: "#831843",
      accentStrong: "#ec4899",
      accentHover: "#be185d",
      accentContrast: "#2a0718",
      accentText: "#f9a8d4",
      accentTextStrong: "#fbcfe8",
      frame: "#6b2148",
    },
  },
  {
    value: "orange",
    light: {
      background: "#fffaf3",
      surface: "#ffffff",
      surfaceMuted: "#ffedd5",
      text: "#1c1917",
      textMuted: "#6b5a4b",
      accentSoft: "#fed7aa",
      accentStrong: "#ea580c",
      accentHover: "#c2410c",
      accentContrast: "#1c1917",
      accentText: "#9a3412",
      accentTextStrong: "#7c2d12",
      frame: "#fdba74",
    },
    dark: {
      background: "#1f160f",
      surface: "#2b1d13",
      surfaceMuted: "#3a2617",
      text: "#fff7ed",
      textMuted: "#f1c9a6",
      accentSoft: "#7c2d12",
      accentStrong: "#f97316",
      accentHover: "#c2410c",
      accentContrast: "#1f160f",
      accentText: "#fdba74",
      accentTextStrong: "#fed7aa",
      frame: "#7c3d18",
    },
  },
  {
    value: "blue",
    light: {
      background: "#f5fbff",
      surface: "#ffffff",
      surfaceMuted: "#e0f2fe",
      text: "#0f172a",
      textMuted: "#475569",
      accentSoft: "#dbeafe",
      accentStrong: "#2563eb",
      accentHover: "#1d4ed8",
      accentContrast: "#ffffff",
      accentText: "#1e40af",
      accentTextStrong: "#1e3a8a",
      frame: "#bfdbfe",
    },
    dark: {
      background: "#0f172a",
      surface: "#172033",
      surfaceMuted: "#1e293b",
      text: "#f8fafc",
      textMuted: "#cbd5e1",
      accentSoft: "#1e3a8a",
      accentStrong: "#3b82f6",
      accentHover: "#1d4ed8",
      accentContrast: "#071424",
      accentText: "#93c5fd",
      accentTextStrong: "#bfdbfe",
      frame: "#334155",
    },
  },
  {
    value: "violet",
    light: {
      background: "#fbf8ff",
      surface: "#ffffff",
      surfaceMuted: "#f3e8ff",
      text: "#18181b",
      textMuted: "#5f5570",
      accentSoft: "#ede9fe",
      accentStrong: "#7c3aed",
      accentHover: "#6d28d9",
      accentContrast: "#ffffff",
      accentText: "#5b21b6",
      accentTextStrong: "#4c1d95",
      frame: "#ddd6fe",
    },
    dark: {
      background: "#171321",
      surface: "#221a31",
      surfaceMuted: "#2e2245",
      text: "#faf7ff",
      textMuted: "#d8c7f4",
      accentSoft: "#4c1d95",
      accentStrong: "#8b5cf6",
      accentHover: "#6d28d9",
      accentContrast: "#160c28",
      accentText: "#c4b5fd",
      accentTextStrong: "#ddd6fe",
      frame: "#5b3a87",
    },
  },
];

function isThemePreference(value: string | null): value is ThemePreference {
  return THEME_PRESETS.some((preset) => preset.value === value);
}

function isThemeModePreference(value: string | null): value is ThemeModePreference {
  return value === "system" || value === "light" || value === "dark";
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

export function readStoredThemePreference(): ThemePreference {
  if (typeof window === "undefined") return "default";
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  return isThemePreference(stored) ? stored : "default";
}

export function persistThemePreference(value: ThemePreference): void {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(THEME_STORAGE_KEY, value);
}

export function readStoredThemeModePreference(): ThemeModePreference {
  if (typeof window === "undefined") return "system";
  const stored = window.localStorage.getItem(THEME_MODE_STORAGE_KEY);
  return isThemeModePreference(stored) ? stored : "system";
}

export function persistThemeModePreference(value: ThemeModePreference): void {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(THEME_MODE_STORAGE_KEY, value);
}

export function systemThemeMediaQuery(): MediaQueryList | null {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return null;
  return window.matchMedia(DARK_MEDIA_QUERY);
}

export function themePreset(value: ThemePreference): ThemePreset {
  return THEME_PRESETS.find((preset) => preset.value === value) ?? THEME_PRESETS[0];
}

export function resolveTheme(mode: ThemeModePreference = readStoredThemeModePreference()): ResolvedTheme {
  if (mode === "light" || mode === "dark") return mode;
  return systemThemeMediaQuery()?.matches ? "dark" : "light";
}

export function applyTheme(
  value: ThemePreference,
  mode: ThemeModePreference = readStoredThemeModePreference(),
): ResolvedTheme {
  const resolved = resolveTheme(mode);
  if (typeof document !== "undefined") {
    const palette = themePreset(value)[resolved];
    document.documentElement.classList.toggle("dark", resolved === "dark");
    document.documentElement.classList.add("theme-preset");
    document.documentElement.style.colorScheme = resolved;
    for (const [key, variable] of Object.entries(THEME_CSS_VARS) as Array<[keyof ThemePalette, string]>) {
      document.documentElement.style.setProperty(variable, hexToRgbTriplet(palette[key]));
    }
  }
  return resolved;
}

export function initTheme(): ThemePreference {
  const value = readStoredThemePreference();
  applyTheme(value, readStoredThemeModePreference());
  return value;
}
