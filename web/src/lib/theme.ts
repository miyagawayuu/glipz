export type ThemePreference = "system" | "light" | "dark";
export type ResolvedTheme = "light" | "dark";

const THEME_STORAGE_KEY = "glipz-theme";
const DARK_MEDIA_QUERY = "(prefers-color-scheme: dark)";

function isThemePreference(value: string | null): value is ThemePreference {
  return value === "system" || value === "light" || value === "dark";
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

export function systemThemeMediaQuery(): MediaQueryList | null {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return null;
  return window.matchMedia(DARK_MEDIA_QUERY);
}

export function resolveTheme(value: ThemePreference): ResolvedTheme {
  if (value === "light") return "light";
  if (value === "dark") return "dark";
  return systemThemeMediaQuery()?.matches ? "dark" : "light";
}

export function applyTheme(value: ThemePreference): ResolvedTheme {
  const resolved = resolveTheme(value);
  if (typeof document !== "undefined") {
    document.documentElement.classList.toggle("dark", resolved === "dark");
    document.documentElement.style.colorScheme = resolved;
  }
  return resolved;
}

export function initTheme(): ThemePreference {
  const value = readStoredThemePreference();
  applyTheme(value);
  return value;
}
