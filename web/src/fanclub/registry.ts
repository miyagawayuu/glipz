export type FanclubProviderID = string;

export type FanclubLinkStatus = {
  member_linked: boolean;
  creator_linked: boolean;
};

export type FanclubProvider = {
  id: FanclubProviderID;
  /** i18n key used for labels (optional). */
  labelKey?: string;
  /** Fallback label when i18n is not used. */
  label?: string;
  /**
   * Query param key used by the OAuth callback redirect.
   * Example (current): `?patreon=member_ok`.
   * Example (future): `?fanclub=patreon&result=member_ok`.
   */
  returnQueryKey: string;
  /** API prefix for provider-specific endpoints (keeps current routes intact). */
  apiPrefix: string;
  supportsMember: boolean;
  supportsCreator: boolean;
  enabled: boolean;
};

export const fanclubProviderRegistry: FanclubProvider[] = [
  {
    id: "patreon",
    labelKey: "fanclub.providers.patreon",
    returnQueryKey: "fanclub",
    apiPrefix: "/api/v1/fanclub/patreon",
    supportsMember: true,
    supportsCreator: true,
    enabled: true,
  },
];

export function enabledFanclubProviders(): FanclubProvider[] {
  return fanclubProviderRegistry.filter((p) => p.enabled);
}

export function findFanclubProvider(id: FanclubProviderID): FanclubProvider | null {
  return fanclubProviderRegistry.find((p) => p.id === id) ?? null;
}

