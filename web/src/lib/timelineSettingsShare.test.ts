import { describe, expect, it } from "vitest";

import { exportTimelineSettingsCode, importTimelineSettingsCode } from "./timelineSettingsShare";
import { DEFAULT_TIMELINE_SETTINGS, createCustomTimeline, type TimelineSettings } from "./timelineSettings";

describe("timelineSettingsShare", () => {
  it("round-trips settings and remaps custom timeline ids", () => {
    const custom = createCustomTimeline("Release Watch");
    custom.id = "custom-shared";
    custom.filters.keywords = ["glipz"];
    const settings: TimelineSettings = {
      ...structuredClone(DEFAULT_TIMELINE_SETTINGS),
      defaultTimelineId: custom.id,
      timelines: [...structuredClone(DEFAULT_TIMELINE_SETTINGS.timelines), custom],
    };

    const imported = importTimelineSettingsCode(exportTimelineSettingsCode(settings));
    const importedCustom = imported.timelines.find((timeline) => timeline.kind === "custom" && timeline.label === "Release Watch");

    expect(importedCustom).toBeTruthy();
    expect(importedCustom?.id).not.toBe("custom-shared");
    expect(imported.defaultTimelineId).toBe(importedCustom?.id);
    expect(importedCustom?.filters.keywords).toEqual(["glipz"]);
  });

  it("rejects invalid prefixes", () => {
    expect(() => importTimelineSettingsCode("NOT_A_GLIPZ_CODE")).toThrow("invalid_timeline_settings_code");
  });

  it("rejects oversized share codes", () => {
    expect(() => importTimelineSettingsCode(`GLIPZTL1.${"x".repeat(24_000)}`)).toThrow("timeline_settings_code_too_large");
  });
});
