import {
  createTimelineID,
  normalizeTimelineSettings,
  type TimelineDefinition,
  type TimelineSettings,
} from "./timelineSettings";

const SHARE_PREFIX = "GLIPZTL1.";
const MAX_SHARE_CODE_LENGTH = 24_000;

function bytesToBase64URL(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function base64URLToBytes(input: string): Uint8Array {
  const padded = input.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(input.length / 4) * 4, "=");
  const binary = atob(padded);
  return Uint8Array.from(binary, (ch) => ch.charCodeAt(0));
}

export function exportTimelineSettingsCode(settings: TimelineSettings): string {
  const normalized = normalizeTimelineSettings(settings);
  const payload = JSON.stringify({
    version: normalized.version,
    defaultTimelineId: normalized.defaultTimelineId,
    timelines: normalized.timelines,
  });
  return `${SHARE_PREFIX}${bytesToBase64URL(new TextEncoder().encode(payload))}`;
}

export function importTimelineSettingsCode(code: string): TimelineSettings {
  const trimmed = code.trim();
  if (!trimmed.startsWith(SHARE_PREFIX)) throw new Error("invalid_timeline_settings_code");
  if (trimmed.length > MAX_SHARE_CODE_LENGTH) throw new Error("timeline_settings_code_too_large");
  const raw = trimmed.slice(SHARE_PREFIX.length);
  const payload = new TextDecoder().decode(base64URLToBytes(raw));
  const parsed = JSON.parse(payload) as unknown;
  const normalized = normalizeTimelineSettings(parsed);
  const idMap = new Map<string, string>();
  const timelines: TimelineDefinition[] = normalized.timelines.map((timeline) => {
    if (timeline.kind === "builtin") return timeline;
    const nextID = createTimelineID();
    idMap.set(timeline.id, nextID);
    return { ...timeline, id: nextID };
  });
  return normalizeTimelineSettings({
    ...normalized,
    defaultTimelineId: idMap.get(normalized.defaultTimelineId) ?? normalized.defaultTimelineId,
    timelines,
  });
}
