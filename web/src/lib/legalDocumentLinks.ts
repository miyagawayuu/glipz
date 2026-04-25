export type LegalDocumentKey = "terms" | "privacy" | "nsfw";

export type LegalDocumentURLSettings = {
  terms_url?: string;
  privacy_policy_url?: string;
  nsfw_guidelines_url?: string;
};

export function fallbackLegalDocumentPath(key: LegalDocumentKey): string {
  switch (key) {
    case "terms":
      return "/legal/terms";
    case "privacy":
      return "/legal/privacy";
    case "nsfw":
      return "/legal/nsfw-guidelines";
  }
}

export function configuredLegalDocumentURL(settings: LegalDocumentURLSettings, key: LegalDocumentKey): string {
  const raw =
    key === "terms"
      ? settings.terms_url
      : key === "privacy"
        ? settings.privacy_policy_url
        : settings.nsfw_guidelines_url;
  const url = raw?.trim() ?? "";
  if (!/^https?:\/\//i.test(url)) return "";
  return url;
}

export function legalDocumentLink(settings: LegalDocumentURLSettings, key: LegalDocumentKey): { href: string; external: boolean } {
  const externalURL = configuredLegalDocumentURL(settings, key);
  if (externalURL) return { href: externalURL, external: true };
  return { href: fallbackLegalDocumentPath(key), external: false };
}
