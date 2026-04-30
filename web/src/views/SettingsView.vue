<script setup lang="ts">
import type { Ref } from "vue";
import { computed, inject } from "vue";
import { RouterLink } from "vue-router";
import FanclubPatreonSettings from "../components/FanclubPatreonSettings.vue";
import Icon from "../components/Icon.vue";

type AppMe = {
  handle: string;
  is_site_admin?: boolean;
  fanclub_patreon_enabled?: boolean;
} | null;

const appMe = inject<Ref<AppMe> | null>("appMe", null);

const profilePath = computed(() => (appMe?.value?.handle ? `/@${appMe.value.handle}` : "/feed"));
const showAdmin = computed(() => Boolean(appMe?.value?.is_site_admin));
const showFanclubPatreon = computed(() => Boolean(appMe?.value?.fanclub_patreon_enabled));
const showFanclub = computed(() => showFanclubPatreon.value);
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="flex h-14 items-center gap-3">
      <h1 class="truncate text-lg font-bold">{{ $t("views.settings.title") }}</h1>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="px-4 py-4">
      <h1 class="text-xl font-bold">{{ $t("views.settings.title") }}</h1>
      <p class="mt-1 text-sm text-neutral-600">{{ $t("views.settings.lead") }}</p>
    </div>
  </Teleport>

  <div class="w-full px-4 py-8">
    <div class="space-y-10">
      <section>
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.account") }}
        </h2>
        <div class="mt-3 divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
          <RouterLink
            :to="profilePath"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.profileEdit") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/bookmarks"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.bookmarks") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/settings/custom-emojis"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.customEmojis") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/settings/timeline"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.timelineSettings") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/settings/plugins"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.pluginSettings") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/settings/mfa"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.mfaSettings") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/settings/identity-portability"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.identityPortability") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/settings/account-deletion"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-red-700 transition-colors hover:bg-red-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.accountDeletion") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-red-300" decorative />
          </RouterLink>
        </div>
        <p class="mt-2 text-xs text-neutral-500">{{ $t("views.settings.hints.account") }}</p>
      </section>

      <section>
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.posts") }}
        </h2>
        <div class="mt-3 divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
          <RouterLink
            to="/feed/scheduled"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.scheduledPosts") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
        </div>
      </section>

      <section v-if="showFanclub">
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.fanclub") }}
        </h2>
        <FanclubPatreonSettings v-if="showFanclubPatreon" />
      </section>

      <section>
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.communication") }}
        </h2>
        <div class="mt-3 divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
          <RouterLink
            to="/settings/direct-messages"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.directMessageSettings") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/settings/notifications"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.notificationSettings") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
        </div>
      </section>

      <section>
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.developer") }}
        </h2>
        <div class="mt-3 divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
          <RouterLink
            to="/developer/api"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.apiDeveloper") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
          <RouterLink
            to="/legal/api-guidelines"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.apiReferencePublic") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
        </div>
      </section>

      <section>
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.appearance") }}
        </h2>
        <div class="mt-3 divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
          <RouterLink
            to="/settings/appearance"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.appearanceSettings") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
        </div>
      </section>

      <section>
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.language") }}
        </h2>
        <div class="mt-3 divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
          <RouterLink
            to="/settings/language"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.languageSettings") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
        </div>
      </section>

      <section v-if="showAdmin">
        <h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">
          {{ $t("views.settings.sections.admin") }}
        </h2>
        <div class="mt-3 divide-y divide-neutral-200 overflow-hidden rounded-2xl border border-neutral-200 bg-white shadow-sm">
          <RouterLink
            to="/admin"
            class="flex items-center justify-between gap-3 px-4 py-3.5 text-sm text-neutral-900 transition-colors hover:bg-lime-50"
          >
            <span class="font-medium">{{ $t("views.settings.items.adminControlPanel") }}</span>
            <Icon name="chevronDown" class="h-4 w-4 shrink-0 -rotate-90 text-neutral-400" decorative />
          </RouterLink>
        </div>
      </section>
    </div>
  </div>
</template>
