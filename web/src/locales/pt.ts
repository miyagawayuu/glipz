import type { AppMessageSchema } from "./ja";

type DeepPartial<T> = {
  [K in keyof T]?: T[K] extends Array<infer U>
    ? Array<DeepPartial<U>>
    : T[K] extends Record<string, unknown>
      ? DeepPartial<T[K]>
      : T[K];
};

export const ptOverrides: DeepPartial<AppMessageSchema> = {
  app: {
    loading: "Carregando…",
    loadingShort: "Carregando...",
    locale: {
      heading: "Idioma",
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
    justNow: "agora mesmo",
  },
};
