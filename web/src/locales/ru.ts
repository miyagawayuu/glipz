import type { AppMessageSchema } from "./ja";

type DeepPartial<T> = {
  [K in keyof T]?: T[K] extends Array<infer U>
    ? Array<DeepPartial<U>>
    : T[K] extends Record<string, unknown>
      ? DeepPartial<T[K]>
      : T[K];
};

export const ruOverrides: DeepPartial<AppMessageSchema> = {
  app: {
    loading: "Загрузка…",
    loadingShort: "Загрузка...",
    locale: {
      heading: "Язык",
      ja: "日本語",
      en: "English",
      zh: "中文（简体）",
      ko: "한국어",
      ru: "Русский",
      es: "Español",
      pt: "Português",
      de: "Deutsch",
    },
  },
  time: {
    justNow: "только что",
  },
};
