import { describe, expect, it } from "vitest";

import {
  DEFAULT_TIMELINE_SETTINGS,
  defaultTimelineFilters,
  enabledTimelines,
  normalizeTimelineSettings,
  type TimelineSettings,
} from "./timelineSettings";

describe("normalizeTimelineSettings", () => {
  it("falls back to defaults for invalid input", () => {
    expect(normalizeTimelineSettings(null)).toEqual(DEFAULT_TIMELINE_SETTINGS);
  });

  it("restores built-ins and normalizes custom timelines", () => {
    const normalized = normalizeTimelineSettings({
      defaultTimelineId: "custombadid",
      timelines: [
        { id: "all", enabled: false, filters: { baseScope: "following" } },
        {
          id: "custom bad id!",
          kind: "custom",
          label: "  Custom Feed  ",
          enabled: true,
          sort: "recommended",
          filters: {
            baseScope: "following",
            keywords: [" glipz ", "glipz", "release"],
            includeUsers: ["@Alice"],
            excludeUsers: ["blocked"],
            communities: ["community-a"],
            includeReposts: false,
            includeFederated: false,
            includeNsfw: false,
            ranking: {
              mode: "weighted",
              presetId: "new preset!",
              weights: { recency: 150, popularity: -1, affinity: "20", federated: Number.NaN },
              constraints: { diversity: 101 },
            },
          },
        },
      ],
    });

    const custom = normalized.timelines.find((timeline) => timeline.kind === "custom");
    expect(normalized.timelines.map((timeline) => timeline.id).slice(0, 3)).toEqual(["all", "recommended", "following"]);
    expect(custom?.id).toBe("custombadid");
    expect(custom?.label).toBe("Custom Feed");
    expect(custom?.filters.keywords).toEqual(["glipz", "release"]);
    expect(custom?.filters.includeUsers).toEqual(["Alice"]);
    expect(custom?.filters.includeReposts).toBe(false);
    expect(custom?.filters.ranking.mode).toBe("weighted");
    expect(custom?.filters.ranking.weights.recency).toBe(100);
    expect(custom?.filters.ranking.weights.popularity).toBe(0);
    expect(custom?.filters.ranking.weights.affinity).toBe(20);
    expect(custom?.filters.ranking.constraints.diversity).toBe(100);
    expect(normalized.defaultTimelineId).toBe("custombadid");
  });
});

describe("enabledTimelines", () => {
  it("falls back to default enabled timelines when all saved timelines are disabled", () => {
    const settings: TimelineSettings = {
      version: 1,
      defaultTimelineId: "all",
      timelines: DEFAULT_TIMELINE_SETTINGS.timelines.map((timeline) => ({
        ...timeline,
        enabled: false,
        filters: defaultTimelineFilters(timeline.filters.baseScope),
      })),
    };

    expect(enabledTimelines(settings).map((timeline) => timeline.id)).toEqual(["all", "recommended", "following"]);
  });
});
