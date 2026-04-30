<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { useI18n } from "vue-i18n";
import { getAccessToken } from "../auth";
import {
  createCustomTimeline,
  defaultTimelineFilters,
  fetchTimelineSettings,
  readTimelineSettings,
  resetTimelineSettingsOnServer,
  saveTimelineSettings,
  timelineDisplayLabel,
  type TimelineDefinition,
  type TimelineRankingConstraints,
  type TimelineRankingWeights,
  type TimelineSettings,
  type TimelineSort,
  type TimelineSource,
} from "../lib/timelineSettings";
import { exportTimelineSettingsCode, importTimelineSettingsCode } from "../lib/timelineSettingsShare";

const { t } = useI18n();

const settings = ref<TimelineSettings>(readTimelineSettings());
const newTimelineName = ref("");
const shareCode = ref("");
const importCode = ref("");
const message = ref("");
const loading = ref(false);
const saving = ref(false);

const timelineSourceOptions = computed<Array<{ value: TimelineSource; label: string }>>(() => [
  { value: "all", label: t("views.feed.scopeAll") },
  { value: "recommended", label: t("views.feed.scopeRecommended") },
  { value: "following", label: t("views.feed.scopeFollowing") },
]);

const timelineSortOptions = computed<Array<{ value: TimelineSort; label: string }>>(() => [
  { value: "recent", label: t("views.timelineSettings.sortRecent") },
  { value: "recommended", label: t("views.timelineSettings.sortRecommended") },
]);

const rankingWeightControls: Array<{ key: keyof TimelineRankingWeights; labelKey: string; helpKey: string }> = [
  { key: "recency", labelKey: "views.timelineSettings.ranking.recency", helpKey: "views.timelineSettings.ranking.recencyHelp" },
  { key: "popularity", labelKey: "views.timelineSettings.ranking.popularity", helpKey: "views.timelineSettings.ranking.popularityHelp" },
  { key: "affinity", labelKey: "views.timelineSettings.ranking.affinity", helpKey: "views.timelineSettings.ranking.affinityHelp" },
  { key: "federated", labelKey: "views.timelineSettings.ranking.federated", helpKey: "views.timelineSettings.ranking.federatedHelp" },
];

const rankingConstraintControls: Array<{ key: keyof TimelineRankingConstraints; labelKey: string; helpKey: string }> = [
  { key: "diversity", labelKey: "views.timelineSettings.ranking.diversity", helpKey: "views.timelineSettings.ranking.diversityHelp" },
];

async function save(next = settings.value): Promise<boolean> {
  const token = getAccessToken();
  if (!token) return false;
  settings.value = {
    ...next,
    timelines: next.timelines.map((timeline) => ({
      ...timeline,
      filters: {
        ...timeline.filters,
        ranking: {
          ...timeline.filters.ranking,
          weights: { ...timeline.filters.ranking.weights },
          constraints: { ...timeline.filters.ranking.constraints },
        },
      },
    })),
  };
  saving.value = true;
  try {
    settings.value = await saveTimelineSettings(token, settings.value);
    return true;
  } catch {
    showMessage("views.timelineSettings.saveFailed");
    return false;
  } finally {
    saving.value = false;
  }
}

function showMessage(key: string) {
  message.value = t(key);
}

function labelFor(timeline: TimelineDefinition): string {
  return timelineDisplayLabel(timeline, t);
}

function updateTimeline(id: string, patch: Partial<TimelineDefinition>) {
  const timelines = settings.value.timelines.map((timeline) =>
    timeline.id === id ? { ...timeline, ...patch, filters: { ...(patch.filters ?? timeline.filters) } } : timeline,
  );
  const defaultTimelineId = timelines.some((timeline) => timeline.id === settings.value.defaultTimelineId && timeline.enabled)
    ? settings.value.defaultTimelineId
    : (timelines.find((timeline) => timeline.enabled)?.id ?? "all");
  void save({ ...settings.value, defaultTimelineId, timelines });
}

function updateFilters(id: string, patch: Partial<TimelineDefinition["filters"]>) {
  const timeline = settings.value.timelines.find((item) => item.id === id);
  if (!timeline) return;
  updateTimeline(id, { filters: { ...timeline.filters, ...patch } });
}

function updateRankingWeights(id: string, patch: Partial<TimelineRankingWeights>) {
  const timeline = settings.value.timelines.find((item) => item.id === id);
  if (!timeline) return;
  updateFilters(id, {
    ranking: {
      ...timeline.filters.ranking,
      mode: "weighted",
      weights: { ...timeline.filters.ranking.weights, ...patch },
    },
  });
}

function updateRankingConstraints(id: string, patch: Partial<TimelineRankingConstraints>) {
  const timeline = settings.value.timelines.find((item) => item.id === id);
  if (!timeline) return;
  updateFilters(id, {
    ranking: {
      ...timeline.filters.ranking,
      mode: "weighted",
      constraints: { ...timeline.filters.ranking.constraints, ...patch },
    },
  });
}

function moveTimeline(id: string, direction: -1 | 1) {
  const index = settings.value.timelines.findIndex((timeline) => timeline.id === id);
  const nextIndex = index + direction;
  if (index < 0 || nextIndex < 0 || nextIndex >= settings.value.timelines.length) return;
  const timelines = settings.value.timelines.slice();
  const [item] = timelines.splice(index, 1);
  timelines.splice(nextIndex, 0, item);
  void save({ ...settings.value, timelines });
}

async function addTimeline() {
  const name = newTimelineName.value.trim() || t("views.timelineSettings.newTimelineDefaultName");
  const timeline = createCustomTimeline(name);
  if (await save({ ...settings.value, timelines: [...settings.value.timelines, timeline], defaultTimelineId: timeline.id })) {
    newTimelineName.value = "";
    showMessage("views.timelineSettings.saved");
  }
}

async function deleteTimeline(id: string) {
  const timelines = settings.value.timelines.filter((timeline) => timeline.id !== id || timeline.kind === "builtin");
  if (await save({ ...settings.value, timelines })) {
    showMessage("views.timelineSettings.deleted");
  }
}

function resetFilters(id: string) {
  const timeline = settings.value.timelines.find((item) => item.id === id);
  if (!timeline) return;
  updateTimeline(id, {
    filters: defaultTimelineFilters(timeline.kind === "builtin" ? (timeline.id as TimelineSource) : timeline.filters.baseScope),
  });
}

function listToText(value: string[]): string {
  return value.join(", ");
}

function textToList(value: string): string[] {
  return value
    .split(/[,\n]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function exportSettings() {
  shareCode.value = exportTimelineSettingsCode(settings.value);
  showMessage("views.timelineSettings.exported");
}

async function importSettings() {
  try {
    const imported = importTimelineSettingsCode(importCode.value);
    if (await save(imported)) {
      importCode.value = "";
      showMessage("views.timelineSettings.imported");
    }
  } catch {
    showMessage("views.timelineSettings.importFailed");
  }
}

async function resetAll() {
  const token = getAccessToken();
  if (!token) return;
  saving.value = true;
  try {
    settings.value = await resetTimelineSettingsOnServer(token);
    shareCode.value = "";
    importCode.value = "";
    showMessage("views.timelineSettings.resetDone");
  } catch {
    showMessage("views.timelineSettings.saveFailed");
  } finally {
    saving.value = false;
  }
}

onMounted(async () => {
  const token = getAccessToken();
  if (!token) return;
  loading.value = true;
  try {
    settings.value = await fetchTimelineSettings(token);
  } catch {
    showMessage("views.timelineSettings.loadFailed");
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div class="w-full px-4 py-8">
    <RouterLink to="/settings" class="text-sm font-medium text-lime-800 hover:underline">
      {{ $t("views.timelineSettings.backToSettings") }}
    </RouterLink>
    <div class="mt-4">
      <h1 class="text-2xl font-bold text-neutral-900">{{ $t("views.timelineSettings.title") }}</h1>
      <p class="mt-2 text-sm leading-relaxed text-neutral-600">{{ $t("views.timelineSettings.lead") }}</p>
    </div>

    <p v-if="message" class="mt-4 rounded-xl border border-lime-200 bg-lime-50 px-4 py-3 text-sm text-lime-900">
      {{ message }}
    </p>
    <p v-if="loading || saving" class="mt-4 text-sm text-neutral-500">
      {{ loading ? $t("views.timelineSettings.loading") : $t("views.timelineSettings.saving") }}
    </p>

    <section class="mt-6 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
      <div class="flex flex-wrap items-end gap-3">
        <label class="min-w-0 flex-1">
          <span class="text-sm font-medium text-neutral-900">{{ $t("views.timelineSettings.newTimelineLabel") }}</span>
          <input
            v-model="newTimelineName"
            type="text"
            maxlength="40"
            class="mt-2 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500/30 focus:border-lime-400 focus:ring-2"
            :placeholder="$t('views.timelineSettings.newTimelinePlaceholder')"
            @keydown.enter.prevent="addTimeline"
          />
        </label>
        <button
          type="button"
          class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700"
          @click="addTimeline"
        >
          {{ $t("views.timelineSettings.addTimeline") }}
        </button>
      </div>
    </section>

    <section class="mt-6 space-y-4">
      <article
        v-for="(timeline, index) in settings.timelines"
        :key="timeline.id"
        class="rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm"
      >
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0 flex-1">
            <input
              v-if="timeline.kind === 'custom'"
              :value="timeline.label"
              type="text"
              maxlength="40"
              class="w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm font-semibold text-neutral-900 outline-none ring-lime-500/30 focus:border-lime-400 focus:ring-2"
            @change="updateTimeline(timeline.id, { label: ($event.target as HTMLInputElement).value })"
            />
            <h2 v-else class="text-base font-semibold text-neutral-900">{{ labelFor(timeline) }}</h2>
            <p class="mt-1 text-xs text-neutral-500">{{ $t(`views.timelineSettings.kind.${timeline.kind}`) }}</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              type="button"
              class="rounded-full border border-neutral-200 px-3 py-1.5 text-xs font-semibold text-neutral-700 hover:bg-neutral-50 disabled:opacity-40"
              :disabled="index === 0"
              @click="moveTimeline(timeline.id, -1)"
            >
              {{ $t("views.timelineSettings.moveUp") }}
            </button>
            <button
              type="button"
              class="rounded-full border border-neutral-200 px-3 py-1.5 text-xs font-semibold text-neutral-700 hover:bg-neutral-50 disabled:opacity-40"
              :disabled="index === settings.timelines.length - 1"
              @click="moveTimeline(timeline.id, 1)"
            >
              {{ $t("views.timelineSettings.moveDown") }}
            </button>
          </div>
        </div>

        <div class="mt-4 flex flex-wrap gap-3">
          <label class="inline-flex items-center gap-2 text-sm text-neutral-800">
            <input
              type="checkbox"
              class="h-4 w-4 rounded border-neutral-200 text-lime-600"
              :checked="timeline.enabled"
              @change="updateTimeline(timeline.id, { enabled: ($event.target as HTMLInputElement).checked })"
            />
            <span>{{ $t("views.timelineSettings.enabled") }}</span>
          </label>
          <label class="inline-flex items-center gap-2 text-sm text-neutral-800">
            <input
              type="radio"
              class="h-4 w-4 border-neutral-200 text-lime-600"
              name="default-timeline"
              :checked="settings.defaultTimelineId === timeline.id"
              :disabled="!timeline.enabled"
              @change="void save({ ...settings, defaultTimelineId: timeline.id })"
            />
            <span>{{ $t("views.timelineSettings.defaultTimeline") }}</span>
          </label>
        </div>

        <div class="mt-4 grid gap-3 sm:grid-cols-2">
          <label class="block">
            <span class="text-xs font-medium text-neutral-700">{{ $t("views.timelineSettings.baseScope") }}</span>
            <select
              class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900"
              :value="timeline.filters.baseScope"
              :disabled="timeline.kind === 'builtin'"
              @change="updateFilters(timeline.id, { baseScope: ($event.target as HTMLSelectElement).value as TimelineSource })"
            >
              <option v-for="option in timelineSourceOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>
          <label class="block">
            <span class="text-xs font-medium text-neutral-700">{{ $t("views.timelineSettings.sortMode") }}</span>
            <select
              class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900"
              :value="timeline.sort"
              :disabled="timeline.kind === 'builtin'"
              @change="updateTimeline(timeline.id, { sort: ($event.target as HTMLSelectElement).value as TimelineSort })"
            >
              <option v-for="option in timelineSortOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>
          <label class="block">
            <span class="text-xs font-medium text-neutral-700">{{ $t("views.timelineSettings.keywords") }}</span>
            <input
              type="text"
              class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900"
              :value="listToText(timeline.filters.keywords)"
              :placeholder="$t('views.timelineSettings.listPlaceholder')"
              @change="updateFilters(timeline.id, { keywords: textToList(($event.target as HTMLInputElement).value) })"
            />
          </label>
          <label class="block">
            <span class="text-xs font-medium text-neutral-700">{{ $t("views.timelineSettings.includeUsers") }}</span>
            <input
              type="text"
              class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900"
              :value="listToText(timeline.filters.includeUsers)"
              :placeholder="$t('views.timelineSettings.usersPlaceholder')"
              @change="updateFilters(timeline.id, { includeUsers: textToList(($event.target as HTMLInputElement).value) })"
            />
          </label>
          <label class="block">
            <span class="text-xs font-medium text-neutral-700">{{ $t("views.timelineSettings.excludeUsers") }}</span>
            <input
              type="text"
              class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900"
              :value="listToText(timeline.filters.excludeUsers)"
              :placeholder="$t('views.timelineSettings.usersPlaceholder')"
              @change="updateFilters(timeline.id, { excludeUsers: textToList(($event.target as HTMLInputElement).value) })"
            />
          </label>
          <label class="block sm:col-span-2">
            <span class="text-xs font-medium text-neutral-700">{{ $t("views.timelineSettings.communities") }}</span>
            <input
              type="text"
              class="mt-1 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900"
              :value="listToText(timeline.filters.communities)"
              :placeholder="$t('views.timelineSettings.communitiesPlaceholder')"
              @change="updateFilters(timeline.id, { communities: textToList(($event.target as HTMLInputElement).value) })"
            />
          </label>
        </div>

        <section
          v-if="timeline.kind === 'custom' && timeline.sort === 'recommended'"
          class="mt-4 rounded-2xl border border-lime-100 bg-lime-50/50 p-4"
        >
          <div>
            <h3 class="text-sm font-semibold text-neutral-900">{{ $t("views.timelineSettings.ranking.heading") }}</h3>
            <p class="mt-1 text-xs leading-relaxed text-neutral-600">{{ $t("views.timelineSettings.ranking.lead") }}</p>
          </div>

          <div class="mt-4 grid gap-4 sm:grid-cols-2">
            <label v-for="control in rankingWeightControls" :key="control.key" class="block">
              <span class="flex items-center justify-between gap-3 text-xs font-medium text-neutral-700">
                <span>{{ $t(control.labelKey) }}</span>
                <span>{{ timeline.filters.ranking.weights[control.key] }}</span>
              </span>
              <input
                type="range"
                min="0"
                max="100"
                step="5"
                class="mt-2 w-full accent-lime-600"
                :value="timeline.filters.ranking.weights[control.key]"
                @change="updateRankingWeights(timeline.id, { [control.key]: Number(($event.target as HTMLInputElement).value) })"
              />
              <span class="mt-1 block text-xs text-neutral-500">{{ $t(control.helpKey) }}</span>
            </label>

            <label v-for="control in rankingConstraintControls" :key="control.key" class="block">
              <span class="flex items-center justify-between gap-3 text-xs font-medium text-neutral-700">
                <span>{{ $t(control.labelKey) }}</span>
                <span>{{ timeline.filters.ranking.constraints[control.key] }}</span>
              </span>
              <input
                type="range"
                min="0"
                max="100"
                step="5"
                class="mt-2 w-full accent-lime-600"
                :value="timeline.filters.ranking.constraints[control.key]"
                @change="updateRankingConstraints(timeline.id, { [control.key]: Number(($event.target as HTMLInputElement).value) })"
              />
              <span class="mt-1 block text-xs text-neutral-500">{{ $t(control.helpKey) }}</span>
            </label>
          </div>
        </section>

        <div class="mt-4 flex flex-wrap gap-3">
          <label class="inline-flex items-center gap-2 text-sm text-neutral-800">
            <input type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-lime-600" :checked="timeline.filters.includeReposts" @change="updateFilters(timeline.id, { includeReposts: ($event.target as HTMLInputElement).checked })" />
            <span>{{ $t("views.timelineSettings.includeReposts") }}</span>
          </label>
          <label class="inline-flex items-center gap-2 text-sm text-neutral-800">
            <input type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-lime-600" :checked="timeline.filters.includeFederated" @change="updateFilters(timeline.id, { includeFederated: ($event.target as HTMLInputElement).checked })" />
            <span>{{ $t("views.timelineSettings.includeFederated") }}</span>
          </label>
          <label class="inline-flex items-center gap-2 text-sm text-neutral-800">
            <input type="checkbox" class="h-4 w-4 rounded border-neutral-200 text-lime-600" :checked="timeline.filters.includeNsfw" @change="updateFilters(timeline.id, { includeNsfw: ($event.target as HTMLInputElement).checked })" />
            <span>{{ $t("views.timelineSettings.includeNsfw") }}</span>
          </label>
        </div>

        <div class="mt-4 flex flex-wrap gap-2 border-t border-neutral-200 pt-4">
          <button type="button" class="rounded-full border border-neutral-200 px-3 py-1.5 text-xs font-semibold text-neutral-700 hover:bg-neutral-50" @click="resetFilters(timeline.id)">
            {{ $t("views.timelineSettings.resetFilters") }}
          </button>
          <button v-if="timeline.kind === 'custom'" type="button" class="rounded-full border border-red-200 px-3 py-1.5 text-xs font-semibold text-red-700 hover:bg-red-50" @click="deleteTimeline(timeline.id)">
            {{ $t("views.timelineSettings.deleteTimeline") }}
          </button>
        </div>
      </article>
    </section>

    <section class="mt-6 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm">
      <h2 class="text-base font-semibold text-neutral-900">{{ $t("views.timelineSettings.shareHeading") }}</h2>
      <p class="mt-1 text-sm text-neutral-600">{{ $t("views.timelineSettings.shareLead") }}</p>
      <div class="mt-4 flex flex-wrap gap-2">
        <button type="button" class="rounded-full bg-lime-600 px-4 py-2 text-sm font-semibold text-white hover:bg-lime-700" @click="exportSettings">
          {{ $t("views.timelineSettings.exportCode") }}
        </button>
        <button type="button" class="rounded-full border border-neutral-200 px-4 py-2 text-sm font-semibold text-neutral-700 hover:bg-neutral-50" @click="resetAll">
          {{ $t("views.timelineSettings.resetAll") }}
        </button>
      </div>
      <textarea
        v-if="shareCode"
        v-model="shareCode"
        rows="4"
        readonly
        class="mt-3 w-full rounded-xl border border-neutral-200 bg-neutral-50 px-3 py-2 text-xs text-neutral-800"
      />
      <label class="mt-4 block">
        <span class="text-sm font-medium text-neutral-900">{{ $t("views.timelineSettings.importLabel") }}</span>
        <textarea
          v-model="importCode"
          rows="4"
          class="mt-2 w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-xs text-neutral-900"
          :placeholder="$t('views.timelineSettings.importPlaceholder')"
        />
      </label>
      <button type="button" class="mt-3 rounded-full border border-neutral-200 px-4 py-2 text-sm font-semibold text-neutral-700 hover:bg-neutral-50" @click="importSettings">
        {{ $t("views.timelineSettings.importCode") }}
      </button>
    </section>
  </div>
</template>
