<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "./Icon.vue";
import PostComposerForm from "./PostComposerForm.vue";
import type { PostComposerMode } from "../composables/usePostComposerForm";

const props = defineProps<{
  open: boolean;
  mode: PostComposerMode;
  communityId?: string | null;
  viewerEmail?: string | null;
  viewerHandle?: string | null;
  viewerAvatarUrl?: string | null;
  patreonEnabled?: boolean;
}>();

const emit = defineEmits<{
  "update:open": [value: boolean];
  submitted: [{ mode: PostComposerMode; communityId?: string | null }];
}>();

const { t } = useI18n();
const composerRef = ref<{ focus: () => void } | null>(null);
const isCommunityMode = computed(() => props.mode === "community");
const modalTitle = computed(() =>
  isCommunityMode.value ? t("components.sidebarCompose.communityTitle") : t("components.sidebarCompose.normalTitle"),
);

function close() {
  emit("update:open", false);
}

function onSubmitted(detail: { mode: PostComposerMode; communityId?: string | null }) {
  emit("submitted", detail);
  emit("update:open", false);
}

watch(
  () => props.open,
  (open) => {
    if (open) {
      void nextTick(() => composerRef.value?.focus());
    }
  },
);
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-[120] flex items-start justify-center overflow-y-auto bg-black/50 p-4 pt-10 sm:items-center sm:pt-4"
      role="dialog"
      aria-modal="true"
      :aria-label="modalTitle"
      @click.self="close"
    >
      <section class="w-full max-w-2xl overflow-hidden rounded-3xl bg-white shadow-2xl ring-1 ring-black/10">
        <header class="flex items-center justify-between gap-3 border-b border-neutral-200 px-4 py-3">
          <div>
            <h2 class="text-base font-bold text-neutral-900">{{ modalTitle }}</h2>
            <p class="text-xs text-neutral-500">
              {{ isCommunityMode ? $t("components.sidebarCompose.communityHint") : $t("components.sidebarCompose.normalHint") }}
            </p>
          </div>
          <button
            type="button"
            class="rounded-full p-2 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900"
            :aria-label="$t('views.feed.lightboxClose')"
            @click="close"
          >
            <Icon name="close" class="h-5 w-5" />
          </button>
        </header>
        <PostComposerForm
          ref="composerRef"
          :mode="mode"
          :community-id="communityId"
          :viewer-email="viewerEmail"
          :viewer-handle="viewerHandle"
          :viewer-avatar-url="viewerAvatarUrl"
          :patreon-enabled="!!patreonEnabled"
          layout="modal"
          @submitted="onSubmitted"
        />
      </section>
    </div>
  </Teleport>
</template>
