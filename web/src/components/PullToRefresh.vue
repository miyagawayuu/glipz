<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";

const props = withDefaults(defineProps<{
  onRefresh: () => Promise<void> | void;
  disabled?: boolean;
  threshold?: number;
  maxPull?: number;
  holdDistance?: number;
}>(), {
  disabled: false,
  threshold: 72,
  maxPull: 120,
  holdDistance: 56,
});

const rootEl = ref<HTMLElement | null>(null);
const { t } = useI18n();
const touchActive = ref(false);
const pulling = ref(false);
const refreshing = ref(false);
const pullDistance = ref(0);

let startX = 0;
let startY = 0;
let scrollParent: HTMLElement | Window | null = null;

function getScrollableParent(el: HTMLElement | null): HTMLElement | Window {
  if (typeof window === "undefined" || !el) return window;
  let current = el.parentElement;
  while (current && current !== document.body) {
    const style = window.getComputedStyle(current);
    if ((style.overflowY === "auto" || style.overflowY === "scroll" || style.overflowY === "overlay") && current.scrollHeight > current.clientHeight) {
      return current;
    }
    current = current.parentElement;
  }
  return window;
}

function getScrollTop(target: HTMLElement | Window | null): number {
  if (!target) return 0;
  if (target === window) {
    return window.scrollY || document.documentElement.scrollTop || document.body.scrollTop || 0;
  }
  return target.scrollTop;
}

function resetPull() {
  touchActive.value = false;
  pulling.value = false;
  pullDistance.value = 0;
  scrollParent = null;
}

function onTouchStart(event: TouchEvent) {
  if (props.disabled || refreshing.value || event.touches.length !== 1) return;
  const target = event.target as HTMLElement | null;
  if (target?.closest("input, textarea, select, [contenteditable='true']")) return;
  scrollParent = getScrollableParent(rootEl.value);
  if (getScrollTop(scrollParent) > 0) return;
  startX = event.touches[0]?.clientX ?? 0;
  startY = event.touches[0]?.clientY ?? 0;
  touchActive.value = true;
  pulling.value = false;
}

function onTouchMove(event: TouchEvent) {
  if (!touchActive.value || props.disabled || refreshing.value) return;
  const touch = event.touches[0];
  if (!touch) return;
  const deltaX = touch.clientX - startX;
  const deltaY = touch.clientY - startY;
  if (deltaY <= 0) {
    resetPull();
    return;
  }
  if (Math.abs(deltaX) > deltaY) {
    resetPull();
    return;
  }
  if (getScrollTop(scrollParent) > 0) {
    resetPull();
    return;
  }
  pulling.value = true;
  pullDistance.value = Math.min(props.maxPull, deltaY * 0.45);
  if (event.cancelable) event.preventDefault();
}

async function triggerRefresh() {
  refreshing.value = true;
  pullDistance.value = props.holdDistance;
  try {
    await props.onRefresh();
  } finally {
    refreshing.value = false;
    pullDistance.value = 0;
  }
}

function onTouchEnd() {
  if (!touchActive.value) return;
  const shouldRefresh = pulling.value && pullDistance.value >= props.threshold;
  touchActive.value = false;
  pulling.value = false;
  if (shouldRefresh) {
    void triggerRefresh();
    return;
  }
  pullDistance.value = 0;
}

const indicatorText = computed(() => {
  if (refreshing.value) return t("components.pullToRefresh.refreshing");
  if (pullDistance.value >= props.threshold) return t("components.pullToRefresh.release");
  return t("components.pullToRefresh.pull");
});

const contentStyle = computed(() => ({
  transform: pullDistance.value > 0 ? `translateY(${pullDistance.value}px)` : undefined,
  transition: touchActive.value || refreshing.value ? "none" : "transform 180ms ease",
}));

const indicatorStyle = computed(() => ({
  height: `${pullDistance.value}px`,
  opacity: pullDistance.value > 0 || refreshing.value ? "1" : "0",
}));
</script>

<template>
  <div
    ref="rootEl"
    class="relative min-h-full"
    @touchstart="onTouchStart"
    @touchmove="onTouchMove"
    @touchcancel="resetPull"
    @touchend="onTouchEnd"
  >
    <div
      class="pointer-events-none absolute inset-x-0 top-0 z-10 flex items-end justify-center overflow-hidden"
      :style="indicatorStyle"
      aria-hidden="true"
    >
      <div class="mb-2 rounded-full border border-neutral-200 bg-white/95 px-3 py-1 text-xs font-medium text-neutral-500 shadow-sm">
        {{ indicatorText }}
      </div>
    </div>
    <div class="min-h-full" :style="contentStyle">
      <slot :refreshing="refreshing" />
    </div>
  </div>
</template>
