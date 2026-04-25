import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import DOMPurify from "dompurify";
import { marked } from "marked";
import { apiBase } from "./api";

export type LegalDocName = "terms" | "privacy" | "nsfw-guidelines";

type LegalDocResponse = {
  name: LegalDocName;
  locale?: string;
  markdown: string;
  updated_at?: string;
};

marked.setOptions({ gfm: true, breaks: true });

export function renderLegalMarkdown(md: string): string {
  const raw = marked.parse(md || "", { async: false });
  const html = typeof raw === "string" ? raw : String(raw);
  return DOMPurify.sanitize(html, {
    ADD_ATTR: ["class", "href", "target", "rel", "alt", "loading", "width", "height"],
  });
}

async function fetchLegalDoc(doc: LegalDocName, locale: string): Promise<LegalDocResponse | null> {
  const params = new URLSearchParams();
  if (locale) params.set("locale", locale);
  const query = params.toString();
  const res = await fetch(`${apiBase()}/api/v1/legal-docs/${encodeURIComponent(doc)}${query ? `?${query}` : ""}`, {
    method: "GET",
    headers: { Accept: "application/json" },
  });
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(res.statusText);
  return (await res.json()) as LegalDocResponse;
}

export function useLegalMarkdownDoc(doc: LegalDocName) {
  const { locale } = useI18n();
  const html = ref("");
  const updatedAt = ref("");
  const error = ref("");
  let requestID = 0;

  watch(
    () => String(locale.value || ""),
    async (nextLocale) => {
      const currentID = ++requestID;
      error.value = "";
      try {
        const result = await fetchLegalDoc(doc, nextLocale);
        if (currentID !== requestID) return;
        html.value = result?.markdown ? renderLegalMarkdown(result.markdown) : "";
        updatedAt.value = result?.updated_at || "";
      } catch (err) {
        if (currentID !== requestID) return;
        html.value = "";
        updatedAt.value = "";
        error.value = err instanceof Error ? err.message : "Failed to load legal document";
      }
    },
    { immediate: true },
  );

  const updatedDate = computed(() => (updatedAt.value ? updatedAt.value.slice(0, 10) : ""));

  return {
    customDocHtml: html,
    customDocUpdatedDate: updatedDate,
    customDocError: error,
  };
}
