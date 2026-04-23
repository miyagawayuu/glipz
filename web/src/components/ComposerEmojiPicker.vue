<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { getAccessToken } from "../auth";
import { customEmojiMap, ensureCustomEmojiCatalog, pickerCustomEmojisForHandle, unicodeReactionPickerCategories } from "../lib/customEmojis";
import type { CustomEmoji } from "../types/customEmoji";
import EmojiInline from "./EmojiInline.vue";

const props = withDefaults(defineProps<{
  disabled?: boolean;
  viewerHandle?: string | null;
}>(), {
  disabled: false,
  viewerHandle: null,
});

const emit = defineEmits<{
  select: [emoji: string];
}>();

type PickerTab = {
  id: string;
  label: string;
  kind: "unicode" | "custom";
  slug?: string;
};

const { t } = useI18n();
const DESKTOP_BREAKPOINT = 640;
const VIEWPORT_MARGIN = 16;
const PANEL_GAP = 8;
const DESKTOP_PANEL_WIDTH = 320;
const open = ref(false);
const rootEl = ref<HTMLElement | null>(null);
const panelEl = ref<HTMLElement | null>(null);
const panelStyle = ref<Record<string, string>>({});
const searchQuery = ref("");
const activeTab = ref("");
const emojiCatalog = customEmojiMap();
const standardReactionCategories = unicodeReactionPickerCategories();
const customReactionOptions = computed(() => pickerCustomEmojisForHandle(props.viewerHandle));
const normalizedSearch = computed(() => searchQuery.value.trim().toLowerCase());
const pickerTabs = computed<PickerTab[]>(() => [
  ...standardReactionCategories.map((group) => ({
    id: `unicode:${group.slug}`,
    label: categoryLabel(group.slug),
    kind: "unicode" as const,
    slug: group.slug,
  })),
  ...(customReactionOptions.value.length
    ? [{
        id: "custom",
        label: t("views.compose.emojiPickerCustom"),
        kind: "custom" as const,
      }]
    : []),
]);
const activeUnicodeGroup = computed(() => {
  if (!activeTab.value.startsWith("unicode:")) return null;
  const slug = activeTab.value.slice("unicode:".length);
  return standardReactionCategories.find((group) => group.slug === slug) ?? null;
});
const filteredStandardResults = computed(() => {
  const q = normalizedSearch.value;
  if (!q) return [];
  return standardReactionCategories
    .map((group) => ({
      ...group,
      emojis: group.emojis.filter((emoji) => {
        const haystack = `${emoji.name} ${emoji.slug} ${emoji.emoji}`.toLowerCase();
        return haystack.includes(q);
      }),
    }))
    .filter((group) => group.emojis.length > 0);
});
const filteredCustomResults = computed<CustomEmoji[]>(() => {
  const q = normalizedSearch.value;
  if (!q) return [];
  return customReactionOptions.value.filter((emoji) => {
    const haystack = `${emoji.shortcode} ${emoji.shortcode_name} ${emoji.owner_handle ?? ""}`.toLowerCase();
    return haystack.includes(q);
  });
});
const hasSearchResults = computed(() => filteredStandardResults.value.length > 0 || filteredCustomResults.value.length > 0);

function closePicker() {
  open.value = false;
}

function togglePicker() {
  if (props.disabled) return;
  open.value = !open.value;
}

function selectEmoji(emoji: string) {
  open.value = false;
  emit("select", emoji);
}

function categoryLabel(slug: string): string {
  return t(`components.postTimeline.reactionCategories.${slug}`);
}

function updatePanelPosition() {
  if (typeof window === "undefined" || !open.value) return;
  if (window.innerWidth < DESKTOP_BREAKPOINT) {
    panelStyle.value = {};
    return;
  }

  const anchor = rootEl.value;
  const panel = panelEl.value;
  if (!anchor || !panel) return;

  const anchorRect = anchor.getBoundingClientRect();
  const panelRect = panel.getBoundingClientRect();
  const width = Math.min(DESKTOP_PANEL_WIDTH, window.innerWidth - VIEWPORT_MARGIN * 2);
  const spaceAbove = anchorRect.top - VIEWPORT_MARGIN - PANEL_GAP;
  const spaceBelow = window.innerHeight - anchorRect.bottom - VIEWPORT_MARGIN - PANEL_GAP;
  const desiredHeight = Math.max(panelRect.height, 240);
  const placeAbove = spaceAbove >= desiredHeight || spaceAbove > spaceBelow;
  const availableHeight = Math.max(180, placeAbove ? spaceAbove : spaceBelow);
  const renderedHeight = Math.min(panelRect.height, availableHeight);

  let left = anchorRect.left;
  left = Math.min(left, window.innerWidth - VIEWPORT_MARGIN - width);
  left = Math.max(VIEWPORT_MARGIN, left);

  let top = placeAbove
    ? anchorRect.top - renderedHeight - PANEL_GAP
    : anchorRect.bottom + PANEL_GAP;
  top = Math.max(VIEWPORT_MARGIN, Math.min(top, window.innerHeight - VIEWPORT_MARGIN - renderedHeight));

  panelStyle.value = {
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
    width: `${Math.round(width)}px`,
    maxHeight: `${Math.round(availableHeight)}px`,
  };
}

watch(open, async (isOpen) => {
  if (!isOpen) {
    panelStyle.value = {};
    return;
  }
  searchQuery.value = "";
  activeTab.value = pickerTabs.value[0]?.id ?? "";
  await nextTick();
  updatePanelPosition();
});

watch(pickerTabs, (tabs) => {
  if (!tabs.length) {
    activeTab.value = "";
    return;
  }
  if (!tabs.some((tab) => tab.id === activeTab.value)) {
    activeTab.value = tabs[0].id;
  }
});

watch([searchQuery, activeTab], async () => {
  if (!open.value) return;
  await nextTick();
  updatePanelPosition();
});

onMounted(() => {
  document.addEventListener("click", closePicker);
  window.addEventListener("resize", updatePanelPosition);
  window.addEventListener("scroll", updatePanelPosition, true);
  void ensureCustomEmojiCatalog(getAccessToken());
});

onBeforeUnmount(() => {
  document.removeEventListener("click", closePicker);
  window.removeEventListener("resize", updatePanelPosition);
  window.removeEventListener("scroll", updatePanelPosition, true);
});
</script>

<template>
  <div ref="rootEl" class="relative" @click.stop>
    <button
      type="button"
      class="rounded-full p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-800"
      :class="open ? 'bg-neutral-100 text-neutral-900' : ''"
      :disabled="disabled"
      :title="open ? $t('views.compose.emojiPickerClose') : $t('views.compose.emojiPickerTitle')"
      :aria-label="open ? $t('views.compose.emojiPickerClose') : $t('views.compose.emojiPickerOpen')"
      :aria-expanded="open"
      @click.stop="togglePicker"
    >
      <span class="text-[18px] leading-none grayscale">{{ "🙂" }}</span>
    </button>

    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="translate-y-6 opacity-0 sm:translate-y-2"
      enter-to-class="translate-y-0 opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="translate-y-0 opacity-100"
      leave-to-class="translate-y-6 opacity-0 sm:translate-y-2"
    >
      <div
        v-if="open"
        ref="panelEl"
        class="fixed inset-x-0 bottom-0 z-[70] overflow-hidden rounded-t-3xl border-t border-neutral-200 bg-white px-3 pb-[calc(0.75rem+env(safe-area-inset-bottom,0px))] pt-2 shadow-2xl ring-1 ring-black/5 sm:inset-auto sm:rounded-2xl sm:border sm:p-2 sm:shadow-lg"
        :style="panelStyle"
      >
        <div class="mb-2 flex justify-center sm:hidden" aria-hidden="true">
          <span class="h-1.5 w-10 rounded-full bg-neutral-300"></span>
        </div>
        <div class="max-h-[min(32rem,calc(100vh-5rem-env(safe-area-inset-top,0px)))] space-y-3 overflow-y-auto pr-1 sm:h-[26rem] sm:max-h-[calc(100vh-7rem)]">
        <div class="space-y-2">
          <input
            v-model="searchQuery"
            type="text"
            class="w-full rounded-xl border border-neutral-200 bg-white px-3 py-2 text-sm text-neutral-900 outline-none ring-lime-500 placeholder:text-neutral-400 focus:ring-2"
            :placeholder="$t('views.compose.emojiPickerSearchPlaceholder')"
          />

          <div v-if="!normalizedSearch" class="-mx-1 flex gap-1 overflow-x-auto px-1 pb-1">
            <button
              v-for="tab in pickerTabs"
              :key="tab.id"
              type="button"
              class="shrink-0 rounded-full px-3 py-1.5 text-xs font-medium transition"
              :class="tab.id === activeTab ? 'bg-lime-100 text-lime-900' : 'bg-neutral-100 text-neutral-600 hover:bg-neutral-200'"
              @click="activeTab = tab.id"
            >
              {{ tab.label }}
            </button>
          </div>
        </div>

        <div v-if="normalizedSearch" class="space-y-3">
          <section v-for="group in filteredStandardResults" :key="group.slug" class="space-y-1">
            <p class="px-1 text-[11px] font-medium text-neutral-500">
              {{ categoryLabel(group.slug) }}
            </p>
            <div class="flex flex-wrap gap-1">
              <button
                v-for="emoji in group.emojis"
                :key="`${group.slug}-${emoji.slug}`"
                type="button"
                class="inline-flex h-10 w-10 items-center justify-center rounded-full text-lg transition hover:bg-lime-50"
                :disabled="disabled"
                :aria-label="$t('views.compose.emojiPickerInsert', { emoji: emoji.emoji })"
                :title="emoji.name"
                @click="selectEmoji(emoji.emoji)"
              >
                <EmojiInline :token="emoji.emoji" size-class="text-xl" image-class="h-7 w-7" custom-image-class="h-7 w-auto max-w-14" />
              </button>
            </div>
          </section>

          <section v-if="filteredCustomResults.length" class="space-y-1">
            <p class="px-1 text-[11px] font-medium text-neutral-500">
              {{ $t("views.compose.emojiPickerCustom") }}
            </p>
            <div class="flex flex-wrap gap-1">
              <button
                v-for="emoji in filteredCustomResults"
                :key="emoji.id"
                type="button"
                class="inline-flex h-10 w-10 items-center justify-center rounded-full transition hover:bg-lime-50"
                :disabled="disabled"
                :aria-label="$t('views.compose.emojiPickerInsert', { emoji: emoji.shortcode })"
                :title="emoji.shortcode"
                @click="selectEmoji(emoji.shortcode)"
              >
                <EmojiInline :token="emoji.shortcode" size-class="text-lg" image-class="h-6 w-6" custom-image-class="h-6 w-auto max-w-14" />
              </button>
            </div>
          </section>

          <p v-if="!hasSearchResults" class="px-1 text-xs text-neutral-500">
            {{ $t("views.compose.emojiPickerSearchEmpty") }}
          </p>
        </div>

        <div v-else-if="activeUnicodeGroup" class="space-y-2">
          <p class="px-1 text-[11px] font-semibold uppercase tracking-wide text-neutral-500">
            {{ categoryLabel(activeUnicodeGroup.slug) }}
          </p>
          <div class="flex flex-wrap gap-1">
            <button
              v-for="emoji in activeUnicodeGroup.emojis"
              :key="`${activeUnicodeGroup.slug}-${emoji.slug}`"
              type="button"
              class="inline-flex h-10 w-10 items-center justify-center rounded-full text-lg transition hover:bg-lime-50"
              :disabled="disabled"
              :aria-label="$t('views.compose.emojiPickerInsert', { emoji: emoji.emoji })"
              :title="emoji.name"
              @click="selectEmoji(emoji.emoji)"
            >
              <EmojiInline :token="emoji.emoji" size-class="text-xl" image-class="h-7 w-7" custom-image-class="h-7 w-auto max-w-14" />
            </button>
          </div>
        </div>

        <div v-else-if="activeTab === 'custom'" class="space-y-2">
          <p class="px-1 text-[11px] font-semibold uppercase tracking-wide text-neutral-500">
            {{ $t("views.compose.emojiPickerCustom") }}
          </p>
          <div v-if="customReactionOptions.length" class="flex flex-wrap gap-1">
            <button
              v-for="emoji in customReactionOptions"
              :key="emoji.id"
              type="button"
              class="inline-flex h-10 w-10 items-center justify-center rounded-full transition hover:bg-lime-50"
              :disabled="disabled"
              :aria-label="$t('views.compose.emojiPickerInsert', { emoji: emoji.shortcode })"
              :title="emoji.shortcode"
              @click="selectEmoji(emoji.shortcode)"
            >
              <EmojiInline :token="emoji.shortcode" size-class="text-lg" image-class="h-6 w-6" custom-image-class="h-6 w-auto max-w-14" />
            </button>
          </div>
          <p v-else class="px-1 text-xs text-neutral-500">
            {{ $t("views.compose.emojiPickerCustomEmpty") }}
          </p>
        </div>
        </div>
      </div>
    </Transition>
  </div>
</template>
