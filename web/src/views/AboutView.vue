<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRouter } from "vue-router";
import PostTimeline from "../components/PostTimeline.vue";
import { getOperatorAnnouncements } from "../data/operatorAnnouncements";
import { APP_VERSION, FEDERATION_PROTOCOL_VERSION } from "../lib/appInfo";
import { connectFeedStream, fetchFeedItem, fetchPublicFeedItems, type FeedPubPayload } from "../lib/feedStream";
import { fetchPublicInstanceSettings, type OperatorAnnouncement } from "../lib/instanceSettings";
import { legalDocumentLink, type LegalDocumentURLSettings } from "../lib/legalDocumentLinks";
import type { TimelinePost } from "../types/timeline";

const router = useRouter();
const { t, tm } = useI18n();
const publicTimelineItems = ref<TimelinePost[]>([]);
const publicTimelineLoading = ref(true);
const publicTimelineError = ref("");
let disconnectPublicFeed: (() => void) | null = null;
const heroHighlights = computed(() => tm("about.heroHighlights") as string[]);
const primaryFeatures = computed(() => tm("about.features") as Array<{ title: string; body: string }>);
const trustPoints = computed(() => {
  const points = tm("about.publicInfo.points") as string[];
  return points.map((point) =>
    point
      .replace(APP_VERSION, APP_VERSION)
      .replace(FEDERATION_PROTOCOL_VERSION, FEDERATION_PROTOCOL_VERSION),
  );
});
const legalDocumentUrls = ref<LegalDocumentURLSettings>({});
const quickLinks = computed(() =>
  (tm("about.quickLinks") as Array<{ to: string; title: string; body: string }>).map((item) => {
    if (item.to === "/legal/terms") {
      const link = legalDocumentLink(legalDocumentUrls.value, "terms");
      return { ...item, to: link.href, external: link.external };
    }
    if (item.to === "/legal/privacy") {
      const link = legalDocumentLink(legalDocumentUrls.value, "privacy");
      return { ...item, to: link.href, external: link.external };
    }
    if (item.to === "/legal/nsfw-guidelines") {
      const link = legalDocumentLink(legalDocumentUrls.value, "nsfw");
      return { ...item, to: link.href, external: link.external };
    }
    return { ...item, external: false };
  }),
);
const operatorAnnouncements = ref<OperatorAnnouncement[]>(getOperatorAnnouncements());

async function loadOperatorAnnouncements() {
  try {
    const settings = await fetchPublicInstanceSettings();
    legalDocumentUrls.value = settings;
    operatorAnnouncements.value = settings.operator_announcements.length
      ? settings.operator_announcements
      : getOperatorAnnouncements();
  } catch {
    operatorAnnouncements.value = getOperatorAnnouncements();
  }
}

async function loadPublicTimeline() {
  publicTimelineLoading.value = true;
  publicTimelineError.value = "";
  try {
    publicTimelineItems.value = await fetchPublicFeedItems();
  } catch (e: unknown) {
    publicTimelineError.value = e instanceof Error ? e.message : t("notifications.generic");
  } finally {
    publicTimelineLoading.value = false;
  }
}

function stopPublicTimelineStream() {
  disconnectPublicFeed?.();
  disconnectPublicFeed = null;
}

async function handlePublicFeedPayload(p: FeedPubPayload) {
  if (p.kind === "post_deleted") {
    publicTimelineItems.value = publicTimelineItems.value.filter((it) => it.id !== p.post_id);
    return;
  }
  if (p.kind !== "post_created" && p.kind !== "post_updated") return;
  const row = await fetchFeedItem(p.post_id, null);
  if (!row || row.visibility !== "public") {
    publicTimelineItems.value = publicTimelineItems.value.filter((it) => it.id !== p.post_id);
    return;
  }
  const idx = publicTimelineItems.value.findIndex((it) => it.id === row.id);
  if (idx >= 0) {
    const next = publicTimelineItems.value.slice();
    next[idx] = row;
    publicTimelineItems.value = next;
    return;
  }
  publicTimelineItems.value = [row, ...publicTimelineItems.value].slice(0, 30);
}

function startPublicTimelineStream() {
  stopPublicTimelineStream();
  disconnectPublicFeed = connectFeedStream({
    scope: "all",
    public: true,
    onPayload: (p) => void handlePublicFeedPayload(p),
  });
}

function goLogin() {
  void router.push("/login");
}

onMounted(async () => {
  await loadOperatorAnnouncements();
  await loadPublicTimeline();
  startPublicTimelineStream();
});

onBeforeUnmount(() => {
  stopPublicTimelineStream();
});
</script>

<template>
  <div class="w-full min-w-0 px-4 py-8 text-neutral-900 sm:px-6 lg:px-8">
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-8">
      <section class="overflow-hidden rounded-[2rem] border border-lime-200 bg-white dark:border-lime-800/70 dark:bg-neutral-950">
        <div class="grid gap-8 px-6 py-10 sm:px-8 lg:grid-cols-[minmax(0,1.1fr)_24rem] lg:items-center lg:px-10">
          <div class="max-w-3xl">
            <p class="text-sm font-semibold uppercase tracking-[0.2em] text-lime-700">{{ $t("about.badge") }}</p>
            <h1 class="mt-4 text-4xl font-bold tracking-tight text-neutral-900 sm:text-5xl">
              {{ ($tm("about.title") as string[])[0] }}
              <br />
              {{ ($tm("about.title") as string[])[1] }}
            </h1>
            <p class="mt-5 max-w-2xl text-sm leading-7 text-neutral-700 sm:text-base">
              {{ $t("about.description") }}
            </p>
            <div class="mt-6 max-w-2xl rounded-3xl border border-lime-300 bg-lime-50/80 p-5 dark:border-lime-800/70 dark:bg-lime-950/20">
              <p class="text-xs font-semibold uppercase tracking-[0.18em] text-lime-700 dark:text-lime-300">
                {{ $t("about.activityPubDifference.badge") }}
              </p>
              <h2 class="mt-2 text-xl font-bold text-lime-900 dark:text-lime-200">
                {{ $t("about.activityPubDifference.title") }}
              </h2>
              <p class="mt-3 text-sm leading-7 text-lime-900/90 dark:text-lime-100/90 sm:text-base">
                {{ $t("about.activityPubDifference.body") }}
              </p>
            </div>
            <div class="mt-6 flex flex-wrap gap-3">
              <RouterLink
                to="/register"
                class="inline-flex items-center justify-center rounded-full bg-lime-500 px-5 py-2.5 text-sm font-semibold text-white hover:bg-lime-600"
              >
                {{ $t("common.actions.createAccount") }}
              </RouterLink>
              <RouterLink
                to="/login"
                class="inline-flex items-center justify-center rounded-full border border-neutral-200 bg-white px-5 py-2.5 text-sm font-semibold text-neutral-800 hover:bg-neutral-50"
              >
                {{ $t("common.actions.login") }}
              </RouterLink>
            </div>
            <div class="mt-6 flex flex-wrap gap-2">
              <span
                v-for="item in heroHighlights"
                :key="item"
                class="inline-flex items-center rounded-full border border-neutral-200 bg-white px-3 py-1.5 text-xs font-medium text-neutral-700"
              >
                {{ item }}
              </span>
            </div>
          </div>

          <div class="flex h-[26rem] min-h-0 flex-col overflow-hidden rounded-3xl border border-neutral-200 bg-white/90 shadow-sm dark:border-neutral-200 dark:bg-neutral-900/90">
            <div class="flex flex-wrap items-center justify-between gap-3 border-b border-neutral-200 px-5 py-4 dark:border-neutral-200">
              <div>
                <p class="text-sm font-semibold text-neutral-900">{{ $t("about.publicTimeline.title") }}</p>
                <p class="mt-1 text-xs text-neutral-600">{{ $t("about.publicTimeline.description") }}</p>
              </div>
              <RouterLink to="/register" class="text-sm font-medium text-lime-700 hover:text-lime-800 hover:underline">
                {{ $t("common.actions.joinAndPost") }}
              </RouterLink>
            </div>
            <div class="min-h-0 flex-1">
              <p v-if="publicTimelineError" class="px-5 py-4 text-sm text-red-600">{{ publicTimelineError }}</p>
              <p v-else-if="publicTimelineLoading && !publicTimelineItems.length" class="px-5 py-4 text-sm text-neutral-600">
                {{ $t("about.publicTimeline.loading") }}
              </p>
              <p v-else-if="!publicTimelineItems.length" class="px-5 py-4 text-sm text-neutral-600">
                {{ $t("about.publicTimeline.empty") }}
              </p>
              <div v-else class="h-full overflow-y-auto">
                <PostTimeline
                  :items="publicTimelineItems"
                  :action-busy="null"
                  :embed-thread-replies="false"
                  @reply="goLogin"
                  @toggle-reaction="goLogin"
                  @toggle-bookmark="goLogin"
                  @toggle-repost="goLogin"
                  @share="goLogin"
                />
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="grid gap-4 md:grid-cols-3">
        <article
          v-for="feature in primaryFeatures"
          :key="feature.title"
          class="rounded-3xl border border-neutral-200 bg-white p-6"
        >
          <h2 class="text-lg font-semibold text-neutral-900">{{ feature.title }}</h2>
          <p class="mt-3 text-sm leading-7 text-neutral-700">{{ feature.body }}</p>
        </article>
      </section>

      <section class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <article class="rounded-3xl border border-neutral-200 bg-white p-6">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="text-xl font-semibold text-neutral-900">{{ $t("about.publicInfo.title") }}</h2>
              <p class="mt-2 text-sm leading-7 text-neutral-600">
                {{ $t("about.publicInfo.description") }}
              </p>
            </div>
          </div>
          <ul class="mt-5 space-y-3 text-sm leading-7 text-neutral-700">
            <li v-for="point in trustPoints" :key="point" class="flex gap-3">
              <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
              <span>{{ point }}</span>
            </li>
          </ul>
          <div class="mt-5 grid gap-2 text-xs text-neutral-600 sm:grid-cols-2">
            <div class="rounded-2xl border border-neutral-200 bg-neutral-50 px-3 py-2 dark:border-neutral-200 dark:bg-neutral-800">
              <span class="block text-[11px] uppercase tracking-[0.15em] text-neutral-500">{{ $t("common.labels.app") }}</span>
              <span class="mt-1 block font-medium text-neutral-800">{{ APP_VERSION }}</span>
            </div>
            <div class="rounded-2xl border border-neutral-200 bg-neutral-50 px-3 py-2 dark:border-neutral-200 dark:bg-neutral-800">
              <span class="block text-[11px] uppercase tracking-[0.15em] text-neutral-500">{{ $t("common.labels.federation") }}</span>
              <span class="mt-1 block font-medium text-neutral-800">{{ FEDERATION_PROTOCOL_VERSION }}</span>
            </div>
          </div>
          <div class="mt-5 grid gap-3">
            <template v-for="item in quickLinks" :key="item.to">
              <a
                v-if="item.external"
                :href="item.to"
                target="_blank"
                rel="noopener noreferrer"
                class="rounded-2xl border border-neutral-200 px-4 py-4 transition hover:border-lime-300 hover:bg-lime-50/60"
              >
                <p class="text-sm font-semibold text-neutral-900">{{ item.title }}</p>
                <p class="mt-1 text-sm leading-6 text-neutral-600">{{ item.body }}</p>
              </a>
              <RouterLink
                v-else
                :to="item.to"
                class="rounded-2xl border border-neutral-200 px-4 py-4 transition hover:border-lime-300 hover:bg-lime-50/60"
              >
                <p class="text-sm font-semibold text-neutral-900">{{ item.title }}</p>
                <p class="mt-1 text-sm leading-6 text-neutral-600">{{ item.body }}</p>
              </RouterLink>
            </template>
          </div>
        </article>

        <article class="rounded-3xl border border-neutral-200 bg-white p-6">
          <h2 class="text-xl font-semibold text-neutral-900">{{ $t("about.publicInfo.noticesTitle") }}</h2>
          <div v-if="operatorAnnouncements.length" class="mt-5 space-y-3">
            <div
              v-for="item in operatorAnnouncements.slice(0, 3)"
              :key="item.id"
              class="rounded-2xl border border-neutral-200 bg-neutral-50/80 p-4"
            >
              <p class="text-sm font-semibold text-neutral-900">{{ item.title }}</p>
              <p class="mt-2 text-sm leading-7 text-neutral-700">{{ item.body }}</p>
              <p class="mt-2 text-xs text-neutral-500">{{ item.date }}</p>
            </div>
          </div>
          <p v-else class="mt-5 text-sm text-neutral-600">{{ $t("about.publicInfo.empty") }}</p>
        </article>
      </section>

      <section class="rounded-[2rem] border border-lime-200 bg-lime-50/70 px-6 py-8 text-center dark:border-lime-800/70 dark:bg-neutral-900 sm:px-8">
        <h2 class="text-2xl font-semibold text-neutral-900">{{ $t("about.cta.title") }}</h2>
        <p class="mx-auto mt-3 max-w-2xl text-sm leading-7 text-neutral-700 sm:text-base">
          {{ $t("about.cta.description") }}
        </p>
        <div class="mt-6 flex flex-wrap items-center justify-center gap-3">
          <RouterLink
            to="/register"
            class="inline-flex items-center justify-center rounded-full bg-lime-500 px-5 py-2.5 text-sm font-semibold text-white hover:bg-lime-600"
          >
            {{ $t("common.actions.startNow") }}
          </RouterLink>
          <RouterLink
            to="/login"
            class="inline-flex items-center justify-center rounded-full border border-neutral-200 bg-white px-5 py-2.5 text-sm font-semibold text-neutral-800 hover:bg-neutral-50"
          >
            {{ $t("common.actions.login") }}
          </RouterLink>
        </div>
      </section>
    </div>
  </div>
</template>
