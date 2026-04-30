import { createI18n } from "vue-i18n";
import { APP_NAME } from "../lib/appInfo";
import { enOverrides } from "../locales/en";
import { jaMessages, type AppMessageSchema } from "../locales/ja";
import { zhOverrides } from "../locales/zh";
import { koOverrides } from "../locales/ko";
import { ruOverrides } from "../locales/ru";
import { esOverrides } from "../locales/es";
import { ptOverrides } from "../locales/pt";

export type AppLocale = "ja" | "en" | "zh" | "ko" | "ru" | "es" | "pt";

type DeepPartial<T> = {
  [K in keyof T]?: T[K] extends Array<infer U>
    ? Array<DeepPartial<U>>
    : T[K] extends Record<string, unknown>
      ? DeepPartial<T[K]>
      : T[K];
};

const STORAGE_KEY = "glipz.locale";
const FALLBACK_LOCALE: AppLocale = "ja";
export const supportedLocaleOptions: Array<{ value: AppLocale; labelKey: string }> = [
  { value: "ja", labelKey: "app.locale.ja" },
  { value: "en", labelKey: "app.locale.en" },
  { value: "zh", labelKey: "app.locale.zh" },
  { value: "ko", labelKey: "app.locale.ko" },
  { value: "ru", labelKey: "app.locale.ru" },
  { value: "es", labelKey: "app.locale.es" },
  { value: "pt", labelKey: "app.locale.pt" },
];
const supportedLocales = supportedLocaleOptions.map((option) => option.value);
const localeTags: Record<AppLocale, string> = {
  ja: "ja-JP",
  en: "en-US",
  zh: "zh-CN",
  ko: "ko-KR",
  ru: "ru-RU",
  es: "es-ES",
  pt: "pt-BR",
};

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function mergeDeep<T>(base: T, override?: DeepPartial<T>): T {
  if (!override) return structuredClone(base);
  if (Array.isArray(base)) {
    return ((override as unknown[])?.length ? override : base) as T;
  }
  if (!isPlainObject(base)) {
    return (override ?? base) as T;
  }

  const result: Record<string, unknown> = { ...base };
  for (const [key, value] of Object.entries(override as Record<string, unknown>)) {
    if (value === undefined) continue;
    const current = result[key];
    if (Array.isArray(current)) {
      result[key] = Array.isArray(value) && value.length ? value : current;
      continue;
    }
    if (isPlainObject(current) && isPlainObject(value)) {
      result[key] = mergeDeep(current, value);
      continue;
    }
    result[key] = value;
  }
  return result as T;
}

function normalizeLocale(input: string | null | undefined): AppLocale | null {
  if (!input) return null;
  const lowered = input.toLowerCase();
  const matched = supportedLocales.find((locale) => lowered === locale || lowered.startsWith(`${locale}-`));
  return matched ?? null;
}

export function getStoredLocale(): AppLocale | null {
  if (typeof window === "undefined") return null;
  return normalizeLocale(window.localStorage.getItem(STORAGE_KEY));
}

function detectBrowserLocale(): AppLocale {
  if (typeof navigator === "undefined") return FALLBACK_LOCALE;
  const candidates = [navigator.language, ...(navigator.languages ?? [])];
  for (const candidate of candidates) {
    const locale = normalizeLocale(candidate);
    if (locale) return locale;
  }
  return FALLBACK_LOCALE;
}

export function getInitialLocale(): AppLocale {
  return getStoredLocale() ?? detectBrowserLocale();
}

function setStoredLocale(locale: AppLocale) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(STORAGE_KEY, locale);
}

function applyDocumentLanguage(locale: AppLocale) {
  if (typeof document === "undefined") return;
  document.documentElement.lang = locale;
}

const messages: Record<AppLocale, AppMessageSchema> = {
  ja: jaMessages,
  en: mergeDeep(jaMessages, enOverrides),
  zh: mergeDeep(jaMessages, zhOverrides),
  ko: mergeDeep(jaMessages, koOverrides),
  ru: mergeDeep(jaMessages, ruOverrides),
  es: mergeDeep(jaMessages, esOverrides),
  pt: mergeDeep(jaMessages, ptOverrides),
};

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: getInitialLocale(),
  fallbackLocale: FALLBACK_LOCALE,
  messages,
  missingWarn: false,
  fallbackWarn: false,
});

applyDocumentLanguage(i18n.global.locale.value as AppLocale);

export function getLocale(): AppLocale {
  return i18n.global.locale.value as AppLocale;
}

export function getLocaleTag(locale = getLocale()): string {
  return localeTags[locale] ?? localeTags[FALLBACK_LOCALE];
}

export function setLocale(locale: AppLocale) {
  i18n.global.locale.value = locale;
  setStoredLocale(locale);
  applyDocumentLanguage(locale);
}

export function translate(key: string, params?: Record<string, unknown>): string {
  return i18n.global.t(key, params ?? {}) as string;
}

export function translateObject<T>(key: string): T {
  return i18n.global.tm(key) as T;
}

export function formatDateTime(
  value: string | number | Date,
  options?: Intl.DateTimeFormatOptions,
): string {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat(getLocaleTag(), options).format(date);
}

export function formatRelativeTime(iso: string): string {
  if (!iso) return "";
  const target = new Date(iso);
  if (Number.isNaN(target.getTime())) return "";

  const diffSeconds = Math.floor((Date.now() - target.getTime()) / 1000);
  if (diffSeconds < 45) {
    return translate("time.justNow");
  }

  const rtf = new Intl.RelativeTimeFormat(getLocaleTag(), { numeric: "auto", style: "short" });
  if (diffSeconds < 3600) {
    return rtf.format(-Math.floor(diffSeconds / 60), "minute");
  }
  if (diffSeconds < 86_400) {
    return rtf.format(-Math.floor(diffSeconds / 3600), "hour");
  }
  if (diffSeconds < 604_800) {
    return rtf.format(-Math.floor(diffSeconds / 86_400), "day");
  }

  return formatDateTime(target, { month: "short", day: "numeric" });
}

export function formatScheduledAt(iso: string): string {
  return translate("time.scheduledAt", {
    date: formatDateTime(iso, { dateStyle: "short", timeStyle: "short" }),
  });
}

export function formatUpdatedAt(iso: string): string {
  return translate("time.updatedAt", {
    date: formatDateTime(iso, { dateStyle: "short", timeStyle: "short" }),
  });
}

export function applyDocumentTitle(titleKey?: string) {
  if (typeof document === "undefined") return;
  const appName = APP_NAME;
  if (!titleKey) {
    document.title = appName;
    return;
  }
  const pageTitle = translate(titleKey);
  document.title = pageTitle === appName ? appName : `${pageTitle} | ${appName}`;
}
