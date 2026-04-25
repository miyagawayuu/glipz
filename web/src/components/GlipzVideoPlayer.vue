<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "./Icon.vue";

const props = defineProps<{
  src: string;
}>();

const { t } = useI18n();
const videoRef = ref<HTMLVideoElement | null>(null);
const playing = ref(false);
const duration = ref(0);
const current = ref(0);
const volume = ref(1);
const muted = ref(false);
const seekUi = ref(0);

function formatTime(sec: number): string {
  if (!Number.isFinite(sec) || sec < 0) return "0:00";
  const s = Math.floor(sec % 60);
  const m = Math.floor(sec / 60);
  return `${m}:${s.toString().padStart(2, "0")}`;
}

const displayTime = computed(() => `${formatTime(current.value)} / ${formatTime(duration.value)}`);

function syncFromVideo() {
  const v = videoRef.value;
  if (!v) return;
  duration.value = Number.isFinite(v.duration) ? v.duration : 0;
  current.value = v.currentTime;
}

function onTimeUpdate() {
  current.value = videoRef.value?.currentTime ?? 0;
}

function onPlay() {
  playing.value = true;
}

function onPause() {
  playing.value = false;
}

function togglePlay() {
  const v = videoRef.value;
  if (!v) return;
  if (v.paused) void v.play().catch(() => {});
  else v.pause();
}

function onSeekInput(e: Event) {
  const v = videoRef.value;
  if (!v || !duration.value) return;
  const t0 = Number((e.target as HTMLInputElement).value);
  seekUi.value = t0;
  v.currentTime = (t0 / 1000) * duration.value;
}

function onVolumeInput(e: Event) {
  const v = videoRef.value;
  if (!v) return;
  const x = Number((e.target as HTMLInputElement).value) / 1000;
  volume.value = x;
  v.volume = x;
  muted.value = x === 0;
  v.muted = x === 0;
}

function toggleMute() {
  const v = videoRef.value;
  if (!v) return;
  muted.value = !muted.value;
  v.muted = muted.value;
}

function toggleFullscreen() {
  const v = videoRef.value;
  if (!v) return;
  if (document.fullscreenElement) void document.exitFullscreen().catch(() => {});
  else void v.requestFullscreen().catch(() => {});
}

watch(
  () => props.src,
  () => {
    playing.value = false;
    current.value = 0;
    duration.value = 0;
    seekUi.value = 0;
  },
);

watch([duration, current], () => {
  if (!duration.value) seekUi.value = 0;
  else seekUi.value = Math.round((current.value / duration.value) * 1000);
});

onMounted(() => {
  const v = videoRef.value;
  if (v) {
    v.volume = volume.value;
  }
});

onBeforeUnmount(() => {
  videoRef.value?.pause();
});
</script>

<template>
  <div
    class="overflow-hidden rounded-2xl border border-neutral-200 shadow-sm dark:border-neutral-600"
    role="region"
    :aria-label="t('components.glipzMedia.videoRegion')"
  >
    <div class="bg-neutral-950">
      <video
        ref="videoRef"
        :src="src"
        class="max-h-[510px] w-full object-contain sm:max-h-[70vh]"
        playsinline
        preload="metadata"
        @click="togglePlay"
        @timeupdate="onTimeUpdate"
        @loadedmetadata="syncFromVideo"
        @durationchange="syncFromVideo"
        @play="onPlay"
        @pause="onPause"
        @ended="playing = false"
      />
    </div>
    <div
      class="flex flex-col gap-2 border-t border-neutral-200 bg-white px-3 py-2.5 dark:border-neutral-600 dark:bg-neutral-800"
    >
      <div class="flex items-center gap-2">
        <button
          type="button"
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-lime-600 outline-none ring-offset-2 ring-offset-white hover:bg-lime-50 focus-visible:ring-2 focus-visible:ring-lime-500 dark:text-lime-400 dark:ring-offset-neutral-800 dark:hover:bg-neutral-700/80"
          :aria-label="playing ? t('components.glipzMedia.pause') : t('components.glipzMedia.play')"
          @click="togglePlay"
        >
          <Icon v-if="!playing" name="play" class="h-5 w-5" />
          <svg v-else class="h-5 w-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <rect x="6" y="5" width="4" height="14" rx="1" />
            <rect x="14" y="5" width="4" height="14" rx="1" />
          </svg>
        </button>
        <span class="min-w-0 shrink truncate text-xs tabular-nums text-neutral-600 dark:text-neutral-300">
          {{ displayTime }}
        </span>
        <input
          :value="seekUi"
          type="range"
          min="0"
          max="1000"
          class="h-1.5 min-w-0 flex-1 cursor-pointer accent-lime-500"
          :aria-label="t('components.glipzMedia.seek')"
          @input="onSeekInput"
        />
        <button
          type="button"
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-lime-600 outline-none ring-offset-2 ring-offset-white hover:bg-lime-50 focus-visible:ring-2 focus-visible:ring-lime-500 dark:text-lime-400 dark:ring-offset-neutral-800 dark:hover:bg-neutral-700/80"
          :aria-label="muted ? t('components.glipzMedia.unmute') : t('components.glipzMedia.mute')"
          @click="toggleMute"
        >
          <Icon :name="muted ? 'speakerOff' : 'speaker'" class="h-5 w-5" />
        </button>
        <input
          :value="Math.round(volume * 1000)"
          type="range"
          min="0"
          max="1000"
          class="hidden w-20 cursor-pointer accent-lime-500 sm:block"
          :aria-label="t('components.glipzMedia.volume')"
          @input="onVolumeInput"
        />
        <button
          type="button"
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-lime-600 outline-none ring-offset-2 ring-offset-white hover:bg-lime-50 focus-visible:ring-2 focus-visible:ring-lime-500 dark:text-lime-400 dark:ring-offset-neutral-800 dark:hover:bg-neutral-700/80"
          :aria-label="t('components.glipzMedia.fullscreen')"
          @click="toggleFullscreen"
        >
          <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" aria-hidden="true">
            <path d="M4 9V4h5M15 4h5v5M20 15v5h-5M9 20H4v-5" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>
      </div>
    </div>
  </div>
</template>
