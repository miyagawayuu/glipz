<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { APP_VERSION } from "../lib/appInfo";
import { useBackLink } from "../lib/useBackLink";
const { tm } = useI18n();
const overviewPoints = computed(() => tm("legalPrivacy.overviewPoints") as string[]);
const collectedItems = computed(() => tm("legalPrivacy.collectedItems") as string[]);
const purposeItems = computed(() => tm("legalPrivacy.purposeItems") as string[]);
const sections = computed(() => tm("legalPrivacy.sections") as Array<{ title: string; paragraphs: string[] }>);
const supplementParagraphs = computed(() => tm("legalPrivacy.supplementParagraphs") as string[]);

const backLink = useBackLink({ fallbackTo: "/register" });
</script>

<template>
  <div class="w-full min-w-0 px-4 py-8 text-neutral-900 sm:px-6 lg:px-8">
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-8">
      <RouterLink :to="backLink.to" class="text-sm font-medium text-lime-700 hover:text-lime-800" @click="backLink.onClick">{{ backLink.label }}</RouterLink>

      <section class="overflow-hidden rounded-[2rem] border border-lime-200 bg-white dark:border-lime-800/70 dark:bg-neutral-950">
        <div class="grid gap-8 px-6 py-10 sm:px-8 lg:grid-cols-[minmax(0,1.1fr)_24rem] lg:items-center lg:px-10">
          <div class="max-w-3xl">
            <p class="text-sm font-semibold uppercase tracking-[0.2em] text-lime-700">{{ $t("legalPrivacy.badge") }}</p>
            <h1 class="mt-4 text-4xl font-bold tracking-tight text-neutral-900 sm:text-5xl">
              {{ ($tm("legalPrivacy.title") as string[])[0] }}
              <br />
              {{ ($tm("legalPrivacy.title") as string[])[1] }}
            </h1>
            <p class="mt-5 max-w-2xl text-sm leading-7 text-neutral-700 sm:text-base">
              {{ $t("legalPrivacy.description") }}
            </p>
            <div class="mt-6 flex flex-wrap gap-2 text-xs text-neutral-600">
              <span class="rounded-full border border-neutral-200 bg-white px-3 py-1 dark:border-neutral-200 dark:bg-neutral-900">App {{ APP_VERSION }}</span>
              <span class="rounded-full border border-neutral-200 bg-white px-3 py-1 dark:border-neutral-200 dark:bg-neutral-900">{{ $t("common.labels.updated", { date: "2026-04-19" }) }}</span>
            </div>
          </div>

          <div class="rounded-3xl border border-neutral-200 bg-white/90 p-5 shadow-sm dark:border-neutral-200 dark:bg-neutral-900/90">
            <p class="text-sm font-semibold text-neutral-900">{{ $t("legalPrivacy.overviewTitle") }}</p>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="point in overviewPoints" :key="point" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span>{{ point }}</span>
              </li>
            </ul>
          </div>
        </div>
      </section>

      <section class="rounded-[2rem] border border-neutral-200 bg-white px-6 py-8 dark:border-neutral-200 dark:bg-neutral-950 sm:px-8">
        <div class="space-y-8">
          <section>
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("legalPrivacy.collectedTitle") }}</h2>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in collectedItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span>{{ item }}</span>
              </li>
            </ul>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-xl font-semibold text-neutral-900">{{ $t("legalPrivacy.purposeTitle") }}</h2>
            <ul class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <li v-for="item in purposeItems" :key="item" class="flex gap-3">
                <span class="mt-2 h-2 w-2 shrink-0 rounded-full bg-lime-500" />
                <span>{{ item }}</span>
              </li>
            </ul>
          </section>

          <section
            v-for="section in sections"
            :key="section.title"
            class="border-t border-neutral-200 pt-8 dark:border-neutral-200"
          >
            <h2 class="text-lg font-semibold text-neutral-900">{{ section.title }}</h2>
            <div class="mt-4 space-y-3 text-sm leading-7 text-neutral-700">
              <p v-for="paragraph in section.paragraphs" :key="paragraph">{{ paragraph }}</p>
            </div>
          </section>

          <section class="border-t border-neutral-200 pt-8 dark:border-neutral-200">
            <h2 class="text-2xl font-semibold text-neutral-900">{{ $t("legalPrivacy.supplementTitle") }}</h2>
            <p v-for="paragraph in supplementParagraphs" :key="paragraph" class="mt-3 text-sm leading-7 text-neutral-700">
              {{ paragraph }}
            </p>
          </section>
        </div>
      </section>
    </div>
  </div>
</template>
