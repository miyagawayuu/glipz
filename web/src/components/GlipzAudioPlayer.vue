<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "./Icon.vue";

const props = defineProps<{
  src: string;
}>();

const { t } = useI18n();
const audioRef = ref<HTMLAudioElement | null>(null);
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

function syncFromAudio() {
  const a = audioRef.value;
  if (!a) return;
  duration.value = Number.isFinite(a.duration) ? a.duration : 0;
  current.value = a.currentTime;
}

function onTimeUpdate() {
  current.value = audioRef.value?.currentTime ?? 0;
}

function togglePlay() {
  const a = audioRef.value;
  if (!a) return;
  if (a.paused) void a.play().catch(() => {});
  else a.pause();
}

function onSeekInput(e: Event) {
  const a = audioRef.value;
  if (!a || !duration.value) return;
  const t0 = Number((e.target as HTMLInputElement).value);
  seekUi.value = t0;
  a.currentTime = (t0 / 1000) * duration.value;
}

function onVolumeInput(e: Event) {
  const a = audioRef.value;
  if (!a) return;
  const x = Number((e.target as HTMLInputElement).value) / 1000;
  volume.value = x;
  a.volume = x;
  muted.value = x === 0;
  a.muted = x === 0;
}

function toggleMute() {
  const a = audioRef.value;
  if (!a) return;
  muted.value = !muted.value;
  a.muted = muted.value;
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
  const a = audioRef.value;
  if (a) a.volume = volume.value;
});

onBeforeUnmount(() => {
  audioRef.value?.pause();
});
</script>

<template>
  <div
    class="overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-600 dark:bg-neutral-800"
    role="region"
    :aria-label="t('components.glipzMedia.audioRegion')"
  >
    <audio
      ref="audioRef"
      :src="src"
      preload="metadata"
      class="hidden"
      @timeupdate="onTimeUpdate"
      @loadedmetadata="syncFromAudio"
      @durationchange="syncFromAudio"
      @play="playing = true"
      @pause="playing = false"
      @ended="playing = false"
    />
    <div class="flex flex-col gap-3 px-4 py-4">
      <div class="flex items-center justify-center py-1 text-lime-600 dark:text-lime-400">
        <Icon name="note" class="h-10 w-10 opacity-90" />
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-lime-600 outline-none ring-offset-2 ring-offset-white hover:bg-lime-50 focus-visible:ring-2 focus-visible:ring-lime-500 dark:text-lime-400 dark:ring-offset-neutral-800 dark:hover:bg-neutral-700/80"
          :aria-label="playing ? t('components.glipzMedia.pause') : t('components.glipzMedia.play')"
          @click="togglePlay"
        >
          <Icon v-if="!playing" name="play" class="h-6 w-6" />
          <svg v-else class="h-6 w-6" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <rect x="6" y="5" width="4" height="14" rx="1" />
            <rect x="14" y="5" width="4" height="14" rx="1" />
          </svg>
        </button>
        <span class="w-[88px] shrink-0 text-xs tabular-nums text-neutral-600 dark:text-neutral-300">{{ displayTime }}</span>
        <input
          :value="seekUi"
          type="range"
          min="0"
          max="1000"
          class="h-1.5 min-w-[120px] flex-1 cursor-pointer accent-lime-500 sm:min-w-[180px]"
          :aria-label="t('components.glipzMedia.seek')"
          @input="onSeekInput"
        />
        <button
          type="button"
          class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-lime-600 outline-none ring-offset-2 ring-offset-white hover:bg-lime-50 focus-visible:ring-2 focus-visible:ring-lime-500 dark:text-lime-400 dark:ring-offset-neutral-800 dark:hover:bg-neutral-700/80"
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
          class="w-24 cursor-pointer accent-lime-500"
          :aria-label="t('components.glipzMedia.volume')"
          @input="onVolumeInput"
        />
      </div>
    </div>
  </div>
</template>
