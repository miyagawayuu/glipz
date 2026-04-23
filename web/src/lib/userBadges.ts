export const USER_BADGE_ORDER = ["operator", "verified", "bot", "ai"] as const;

export type UserBadge = typeof USER_BADGE_ORDER[number];

const USER_BADGE_SET = new Set<string>(USER_BADGE_ORDER);

export function normalizeUserBadges(input?: readonly string[] | null): UserBadge[] {
  if (!input?.length) return [];
  const seen = new Set<string>();
  for (const raw of input) {
    const badge = String(raw ?? "").trim().toLowerCase();
    if (!USER_BADGE_SET.has(badge)) continue;
    seen.add(badge);
  }
  return USER_BADGE_ORDER.filter((badge): badge is UserBadge => seen.has(badge));
}
