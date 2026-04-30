<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { getLocaleTag, type AppLocale } from "../../../i18n";

type TodayFact = {
  id: string;
  date: string;
};

const facts: TodayFact[] = [
  { id: "newYear", date: "01-01" },
  { id: "saferInternet", date: "02-11" },
  { id: "earthDay", date: "04-22" },
  { id: "mayDay", date: "05-01" },
  { id: "environmentDay", date: "06-05" },
  { id: "moonLanding", date: "07-20" },
  { id: "peaceDay", date: "09-21" },
  { id: "humanRightsDay", date: "12-10" },
];

const { locale, t } = useI18n();
const today = new Date();

const localeTag = computed(() => getLocaleTag(locale.value as AppLocale));
const monthKey = computed(() => String(today.getMonth() + 1));
const todayKey = computed(() => {
  const month = String(today.getMonth() + 1).padStart(2, "0");
  const day = String(today.getDate()).padStart(2, "0");
  return `${month}-${day}`;
});
const dateLabel = computed(() =>
  new Intl.DateTimeFormat(localeTag.value, {
    month: "long",
    day: "numeric",
    weekday: "short",
  }).format(today),
);
const todayFacts = computed(() => facts.filter((fact) => fact.date === todayKey.value));
const fallbackTitle = computed(() => t(`plugins.today.months.${monthKey.value}.title`));
const fallbackBody = computed(() => t(`plugins.today.months.${monthKey.value}.body`));
</script>

<template>
  <div class="bg-amber-50/70 px-3 py-3">
    <p class="text-xs font-semibold uppercase tracking-wide text-amber-700">
      {{ t("plugins.today.eyebrow") }}
    </p>
    <p class="mt-1 text-sm font-semibold text-neutral-900">{{ dateLabel }}</p>
  </div>
  <div class="space-y-3 px-3 py-3">
    <section
      v-for="fact in todayFacts"
      :key="fact.id"
      class="rounded-xl border border-amber-100 bg-white px-3 py-2"
    >
      <p class="text-sm font-semibold text-neutral-900">
        {{ t(`plugins.today.facts.${fact.id}.title`) }}
      </p>
      <p class="mt-1 text-sm leading-relaxed text-neutral-600">
        {{ t(`plugins.today.facts.${fact.id}.body`) }}
      </p>
    </section>
    <section v-if="!todayFacts.length" class="rounded-xl border border-amber-100 bg-white px-3 py-2">
      <p class="text-sm font-semibold text-neutral-900">{{ fallbackTitle }}</p>
      <p class="mt-1 text-sm leading-relaxed text-neutral-600">{{ fallbackBody }}</p>
    </section>
  </div>
</template>
