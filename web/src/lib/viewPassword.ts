// Backend constants are defined in Go with iota after `None = 0`,
// so the persisted values are Text=2, Media=4, All=8.
export const VIEW_PASSWORD_SCOPE_TEXT = 2;
export const VIEW_PASSWORD_SCOPE_MEDIA = 4;
export const VIEW_PASSWORD_SCOPE_ALL = 8;

export type ViewPasswordTextRange = {
  start: number;
  end: number;
};

export function scopeProtectsText(scope: number): boolean {
  return scope === VIEW_PASSWORD_SCOPE_ALL || (scope & VIEW_PASSWORD_SCOPE_TEXT) !== 0;
}

export function scopeProtectsMedia(scope: number): boolean {
  return scope === VIEW_PASSWORD_SCOPE_ALL || (scope & VIEW_PASSWORD_SCOPE_MEDIA) !== 0;
}

export function buildViewPasswordScope(opts: {
  protectAll: boolean;
  protectText: boolean;
  protectMedia: boolean;
}): number {
  if (opts.protectAll) return VIEW_PASSWORD_SCOPE_ALL;
  let scope = 0;
  if (opts.protectText) scope |= VIEW_PASSWORD_SCOPE_TEXT;
  if (opts.protectMedia) scope |= VIEW_PASSWORD_SCOPE_MEDIA;
  return scope;
}

export function normalizeViewPasswordRanges(ranges: ViewPasswordTextRange[]): ViewPasswordTextRange[] {
  const filtered = ranges
    .filter((x) => Number.isInteger(x.start) && Number.isInteger(x.end) && x.start >= 0 && x.end > x.start)
    .sort((a, b) => (a.start === b.start ? a.end - b.end : a.start - b.start));
  const out: ViewPasswordTextRange[] = [];
  for (const rg of filtered) {
    const last = out[out.length - 1];
    if (!last || rg.start > last.end) {
      out.push({ ...rg });
      continue;
    }
    last.end = Math.max(last.end, rg.end);
  }
  return out;
}

export function codeUnitOffsetToRuneIndex(text: string, offset: number): number {
  return Array.from(text.slice(0, Math.max(0, offset))).length;
}

export function sliceRunes(text: string, start: number, end: number): string {
  return Array.from(text).slice(start, end).join("");
}

export type ViewPasswordScopeLabels = {
  all: string;
  text: string;
  media: string;
  sep: string;
};

export function viewPasswordScopeSummary(scope: number, labels: ViewPasswordScopeLabels): string {
  const parts: string[] = [];
  if (scope === VIEW_PASSWORD_SCOPE_ALL) return labels.all;
  if (scopeProtectsText(scope)) parts.push(labels.text);
  if (scopeProtectsMedia(scope)) parts.push(labels.media);
  return parts.join(labels.sep);
}
