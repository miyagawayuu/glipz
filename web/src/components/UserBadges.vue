<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { normalizeUserBadges, type UserBadge } from "../lib/userBadges";
import operatorBadgeUrl from "../assets/badges/operator.svg";
import verifiedBadgeUrl from "../assets/badges/verified.svg";
import botBadgeUrl from "../assets/badges/bot.svg";
import aiBadgeUrl from "../assets/badges/ai.svg";

const props = withDefaults(defineProps<{
  badges?: readonly string[] | null;
  size?: "sm" | "xs";
}>(), {
  badges: () => [],
  size: "sm",
});

const { t } = useI18n();

const items = computed(() => normalizeUserBadges(props.badges));

function badgeClass(badge: UserBadge): string {
  if (badge === "operator") return "drop-shadow-[0_1px_1px_rgba(0,0,0,0.18)]";
  if (badge === "verified") return "drop-shadow-[0_1px_1px_rgba(29,161,242,0.25)]";
  if (badge === "bot") return "drop-shadow-[0_1px_1px_rgba(29,161,242,0.25)]";
  return "drop-shadow-[0_1px_1px_rgba(29,161,242,0.25)]";
}

function badgeStyle() {
  return {
    width: "16px",
    height: "16px",
  };
}

function badgeLabel(badge: UserBadge): string {
  return t(`badges.${badge}`);
}

function badgeIcon(badge: UserBadge): string {
  if (badge === "operator") return operatorBadgeUrl;
  if (badge === "verified") return verifiedBadgeUrl;
  if (badge === "bot") return botBadgeUrl;
  return aiBadgeUrl;
}
</script>

<template>
  <span v-if="items.length" class="inline-flex flex-wrap items-center gap-[0.25em] align-middle">
    <span
      v-for="badge in items"
      :key="badge"
      class="inline-flex shrink-0 items-center justify-center align-middle"
      :class="badgeClass(badge)"
      :style="badgeStyle()"
      :title="badgeLabel(badge)"
      :aria-label="badgeLabel(badge)"
    >
      <img :src="badgeIcon(badge)" :alt="badgeLabel(badge)" class="block h-full w-full" />
      <span class="sr-only">{{ badgeLabel(badge) }}</span>
    </span>
  </span>
</template>
