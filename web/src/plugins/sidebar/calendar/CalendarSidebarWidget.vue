<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { getLocaleTag, type AppLocale } from "../../../i18n";

interface CalendarDay {
  key: string;
  day: number;
  inMonth: boolean;
  isToday: boolean;
}

const { locale, t } = useI18n();
const viewedMonth = ref(startOfMonth(new Date()));

const localeTag = computed(() => getLocaleTag(locale.value as AppLocale));
const monthTitle = computed(() =>
  new Intl.DateTimeFormat(localeTag.value, {
    year: "numeric",
    month: "long",
  }).format(viewedMonth.value),
);
const weekdayLabels = computed(() => {
  const baseSunday = new Date(Date.UTC(2026, 1, 1));
  return Array.from({ length: 7 }, (_, index) =>
    new Intl.DateTimeFormat(localeTag.value, { weekday: "short" }).format(addDays(baseSunday, index)),
  );
});
const days = computed<CalendarDay[]>(() => {
  const month = viewedMonth.value;
  const gridStart = addDays(month, -month.getDay());
  const today = stripTime(new Date());

  return Array.from({ length: 42 }, (_, index) => {
    const date = addDays(gridStart, index);
    const inMonth = date.getMonth() === month.getMonth();
    return {
      key: date.toISOString(),
      day: date.getDate(),
      inMonth,
      isToday: sameDate(date, today),
    };
  });
});

function startOfMonth(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), 1, 12);
}

function stripTime(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate(), 12);
}

function addDays(date: Date, daysToAdd: number): Date {
  const next = new Date(date);
  next.setDate(next.getDate() + daysToAdd);
  return next;
}

function sameDate(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear()
    && a.getMonth() === b.getMonth()
    && a.getDate() === b.getDate();
}

function moveMonth(offset: number) {
  viewedMonth.value = new Date(viewedMonth.value.getFullYear(), viewedMonth.value.getMonth() + offset, 1, 12);
}

function showToday() {
  viewedMonth.value = startOfMonth(new Date());
}
</script>

<template>
  <div>
    <div class="flex items-center justify-between gap-2 bg-lime-50/70 px-3 py-3">
      <h2 class="text-base font-bold text-neutral-900">{{ monthTitle }}</h2>
      <div class="flex items-center gap-1">
        <button
          type="button"
          class="inline-flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold text-neutral-600 hover:bg-white hover:text-lime-700"
          :aria-label="t('plugins.calendar.previousMonth')"
          @click="moveMonth(-1)"
        >
          ‹
        </button>
        <button
          type="button"
          class="inline-flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold text-neutral-600 hover:bg-white hover:text-lime-700"
          :aria-label="t('plugins.calendar.nextMonth')"
          @click="moveMonth(1)"
        >
          ›
        </button>
      </div>
    </div>
    <div class="px-3 pb-3 pt-2">
      <div class="grid grid-cols-7 gap-1 text-center text-[11px] font-semibold text-neutral-400">
        <span v-for="label in weekdayLabels" :key="label">{{ label }}</span>
      </div>
      <div class="mt-2 grid grid-cols-7 gap-1 text-center text-sm">
        <span
          v-for="day in days"
          :key="day.key"
          class="inline-flex aspect-square items-center justify-center rounded-full"
          :class="[
            day.inMonth ? 'text-neutral-800' : 'text-neutral-300',
            day.isToday ? 'bg-lime-600 font-bold text-white shadow-sm shadow-lime-900/20' : '',
          ]"
        >
          {{ day.day }}
        </span>
      </div>
      <button
        type="button"
        class="mt-3 w-full rounded-full border border-lime-200 px-3 py-2 text-sm font-semibold text-lime-800 hover:bg-lime-50"
        @click="showToday"
      >
        {{ t("plugins.calendar.today") }}
      </button>
    </div>
  </div>
</template>
