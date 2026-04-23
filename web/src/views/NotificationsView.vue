<script setup lang="ts">
import { onMounted, reactive, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRoute, useRouter } from "vue-router";
import PullToRefresh from "../components/PullToRefresh.vue";
import Icon from "../components/Icon.vue";
import UserBadges from "../components/UserBadges.vue";
import { getAccessToken } from "../auth";
import { api } from "../lib/api";
import { avatarInitials, formatRelativeTime, fullHandleAt } from "../lib/feedDisplay";
import {
  notificationReceivedTick,
  refreshUnreadNotificationCount,
  resetUnreadNotificationCount,
} from "../notificationHub";
import { translate } from "../i18n";

type NotificationRow = {
  id: string;
  kind: "reply" | "like" | "repost" | "follow" | "dm_invite";
  actor_handle: string;
  actor_display_name: string;
  actor_badges?: string[];
  subject_post_id?: string | null;
  actor_post_id?: string | null;
  subject_author_handle?: string | null;
  subject_author_display_name?: string | null;
  subject_author_badges?: string[] | null;
  created_at: string;
  read_at?: string | null;
  actor_avatar_url?: string | null;
  subject_caption_preview?: string | null;
  subject_media_url?: string | null;
  subject_author_avatar_url?: string | null;
};

const items = ref<NotificationRow[]>([]);
const err = ref("");
const busy = ref(false);
const avatarLoadFailed = reactive<Record<string, boolean>>({});
const { t } = useI18n();
const route = useRoute();
const router = useRouter();

function isRepliesTab(): boolean {
  const raw = route.query.tab;
  const tab = Array.isArray(raw) ? String(raw[0] ?? "") : String(raw ?? "");
  return tab === "replies";
}

function actorProfilePath(it: NotificationRow): string {
  return `/@${encodeURIComponent(it.actor_handle)}`;
}

function subjectAuthorProfilePath(it: NotificationRow): string | null {
  const h = it.subject_author_handle?.trim();
  if (!h) return null;
  return `/@${encodeURIComponent(h)}`;
}

function actorLabel(it: NotificationRow): string {
  return it.actor_display_name?.trim() || fullHandleAt(it.actor_handle);
}

function subjectAuthorLabel(it: NotificationRow): string {
  const n = it.subject_author_display_name?.trim();
  if (n) return n;
  const h = it.subject_author_handle?.trim();
  if (h) return fullHandleAt(h);
  return "";
}

function rowMessage(it: NotificationRow): string {
  const name = actorLabel(it);
  switch (it.kind) {
    case "reply":
      return translate("notifications.reply", { name });
    case "like":
      return translate("notifications.like", { name });
    case "repost":
      return translate("notifications.repost", { name });
    case "follow":
      return translate("notifications.follow", { name });
    case "dm_invite":
      return translate("notifications.dmInvite", { name });
    default:
      return translate("views.notifications.defaultLabel");
  }
}

function rowHref(it: NotificationRow): string {
  if (it.kind === "dm_invite") {
    return "/messages";
  }
  if (it.kind === "follow") {
    return `/@${encodeURIComponent(it.actor_handle)}`;
  }
  if (it.kind === "reply" && it.actor_post_id) {
    return `/@${encodeURIComponent(it.actor_handle)}#post-${it.actor_post_id}`;
  }
  if (it.subject_post_id && it.subject_author_handle) {
    return `/@${encodeURIComponent(it.subject_author_handle)}#post-${it.subject_post_id}`;
  }
  return "/feed";
}

function notificationsApiPath(): string {
  return isRepliesTab() ? "/api/v1/notifications?kind=reply" : "/api/v1/notifications";
}

function onAvatarError(id: string) {
  avatarLoadFailed[id] = true;
}

function goNotif(it: NotificationRow) {
  void router.push(rowHref(it));
}

async function loadList() {
  const token = getAccessToken();
  if (!token) return;
  busy.value = true;
  err.value = "";
  try {
    const res = await api<{ items: NotificationRow[] }>(notificationsApiPath(), { method: "GET", token });
    items.value = Array.isArray(res.items) ? res.items : [];
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : t("views.notifications.loadFailed");
    items.value = [];
  } finally {
    busy.value = false;
  }
}

async function markAllRead() {
  const token = getAccessToken();
  if (!token) return;
  try {
    await api("/api/v1/notifications/read-all", { method: "POST", token });
    resetUnreadNotificationCount();
    await refreshUnreadNotificationCount();
  } catch {
    /* ignore */
  }
}

async function refreshList() {
  await loadList();
  await markAllRead();
  await loadList();
}

async function setNotificationsTab(tab: "all" | "replies") {
  await router.replace({
    path: "/notifications",
    query: tab === "replies" ? { tab: "replies" } : {},
  });
}

onMounted(async () => {
  await refreshList();
});

watch(notificationReceivedTick, () => {
  void loadList();
});

watch(
  () => route.query.tab,
  () => {
    void refreshList();
  },
);
</script>

<template>
  <Teleport to="#app-view-header-slot-desktop">
    <div class="flex h-14 items-center">
      <h1 class="text-lg font-bold">{{ $t("views.notifications.title") }}</h1>
    </div>
  </Teleport>
  <Teleport to="#app-view-header-slot-mobile">
    <div class="flex h-14 items-center px-4">
      <h1 class="text-lg font-bold">{{ $t("views.notifications.title") }}</h1>
    </div>
  </Teleport>
  <PullToRefresh :on-refresh="refreshList">
    <div class="grid grid-cols-2 border-b border-neutral-200">
      <button
        type="button"
        class="relative border-b-2 py-3 text-base font-semibold transition-colors"
        :class="
          !isRepliesTab()
            ? 'border-lime-600 text-neutral-900'
            : 'border-transparent text-neutral-500 hover:text-neutral-800'
        "
        @click="setNotificationsTab('all')"
      >
        {{ $t("views.notifications.tabAll") }}
      </button>
      <button
        type="button"
        class="relative border-b-2 py-3 text-base font-semibold transition-colors"
        :class="
          isRepliesTab()
            ? 'border-lime-600 text-neutral-900'
            : 'border-transparent text-neutral-500 hover:text-neutral-800'
        "
        @click="setNotificationsTab('replies')"
      >
        {{ $t("views.notifications.tabReplies") }}
      </button>
    </div>
    <p v-if="err" class="border-b border-neutral-200 px-4 py-3 text-sm text-red-600">{{ err }}</p>
    <p v-else-if="busy && !items.length" class="px-4 py-10 text-center text-sm text-neutral-500">{{ $t("app.loading") }}</p>
    <p v-else-if="!items.length" class="px-4 py-10 text-center text-sm text-neutral-500">{{ $t("views.notifications.empty") }}</p>
    <ul v-else class="divide-y divide-neutral-200">
      <li v-for="it in items" :key="it.id">
        <div
          v-if="it.kind === 'like'"
          role="link"
          tabindex="0"
          class="flex cursor-pointer gap-3 px-4 py-3 transition-colors hover:bg-neutral-50"
          @click="goNotif(it)"
          @keydown.enter.prevent="goNotif(it)"
          @keydown.space.prevent="goNotif(it)"
        >
          <span
            class="mt-2 h-2 w-2 shrink-0 rounded-full"
            :class="it.read_at ? 'bg-transparent' : 'bg-lime-500'"
            aria-hidden="true"
          />
          <div class="flex shrink-0 items-start gap-2">
            <Icon name="heart" class="mt-1.5 size-[25px] shrink-0 text-rose-500" decorative />
            <div
              class="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-xs font-bold text-neutral-700"
            >
              <img
                v-if="it.actor_avatar_url && !avatarLoadFailed[it.id]"
                :src="it.actor_avatar_url"
                alt=""
                class="h-full w-full object-cover"
                @error="onAvatarError(it.id)"
              />
              <span v-else>{{ avatarInitials(actorLabel(it)) }}</span>
            </div>
          </div>
          <div class="flex min-w-0 flex-1 gap-2">
            <div class="min-w-0 flex-1">
              <p class="text-sm text-neutral-900">
                {{ $t("views.notifications.likeActivity", { name: actorLabel(it) }) }}
                <UserBadges :badges="it.actor_badges" size="xs" />
                <span class="text-neutral-500">·{{ formatRelativeTime(it.created_at) }}</span>
              </p>
              <p v-if="it.subject_caption_preview" class="mt-1 line-clamp-4 text-sm text-neutral-700">
                {{ it.subject_caption_preview }}
              </p>
            </div>
            <div v-if="it.subject_media_url" class="relative h-14 w-14 shrink-0 overflow-hidden rounded-md border border-neutral-200 bg-neutral-100">
              <img :src="it.subject_media_url" alt="" class="h-full w-full object-cover" />
            </div>
          </div>
        </div>

        <div
          v-else-if="it.kind === 'follow'"
          role="link"
          tabindex="0"
          class="flex cursor-pointer gap-3 px-4 py-3 transition-colors hover:bg-neutral-50"
          @click="goNotif(it)"
          @keydown.enter.prevent="goNotif(it)"
          @keydown.space.prevent="goNotif(it)"
        >
          <span
            class="mt-2 h-2 w-2 shrink-0 rounded-full"
            :class="it.read_at ? 'bg-transparent' : 'bg-lime-500'"
            aria-hidden="true"
          />
          <div class="flex shrink-0 items-start gap-2">
            <Icon name="user" class="mt-1.5 size-[25px] shrink-0 text-lime-600" decorative />
            <div
              class="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-xs font-bold text-neutral-700"
            >
              <img
                v-if="it.actor_avatar_url && !avatarLoadFailed[it.id]"
                :src="it.actor_avatar_url"
                alt=""
                class="h-full w-full object-cover"
                @error="onAvatarError(it.id)"
              />
              <span v-else>{{ avatarInitials(actorLabel(it)) }}</span>
            </div>
          </div>
          <div class="min-w-0 flex-1">
            <p class="text-sm text-neutral-900">
              {{ $t("views.notifications.followActivity", { name: actorLabel(it) }) }}
              <UserBadges :badges="it.actor_badges" size="xs" />
              <span class="text-neutral-500">·{{ formatRelativeTime(it.created_at) }}</span>
            </p>
          </div>
        </div>

        <div
          v-else-if="it.kind === 'repost'"
          role="link"
          tabindex="0"
          class="cursor-pointer transition-colors hover:bg-neutral-50"
          @click="goNotif(it)"
          @keydown.enter.prevent="goNotif(it)"
          @keydown.space.prevent="goNotif(it)"
        >
          <div class="flex gap-3 px-4 pt-3">
            <span
              class="mt-2 h-2 w-2 shrink-0 rounded-full"
              :class="it.read_at ? 'bg-transparent' : 'bg-lime-500'"
              aria-hidden="true"
            />
            <div class="min-w-0 flex-1">
              <div
                class="flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-neutral-200 bg-neutral-50/90 px-4 py-2 text-xs text-neutral-600 -mx-4"
              >
                <Icon name="repost" class="size-[25px] shrink-0 text-lime-600" decorative />
                <RouterLink
                  :to="actorProfilePath(it)"
                  class="font-medium text-neutral-800 hover:text-lime-700 hover:underline"
                  @click.stop
                >
                  {{ actorLabel(it) }}
                </RouterLink>
                <UserBadges :badges="it.actor_badges" size="xs" />
                <span>{{ $t("components.postTimeline.repostedBySuffix") }}</span>
                <time class="text-neutral-500" :datetime="it.created_at">{{ formatRelativeTime(it.created_at) }}</time>
              </div>
              <div class="px-3 pb-3 pt-2">
                <div class="flex gap-3 rounded-xl border border-neutral-200 bg-white px-3 py-2.5 shadow-sm">
                  <RouterLink
                    v-if="subjectAuthorProfilePath(it)"
                    :to="subjectAuthorProfilePath(it)!"
                    class="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-xs font-bold text-neutral-700 hover:ring-2 hover:ring-lime-300"
                    @click.stop
                  >
                    <img
                      v-if="it.subject_author_avatar_url && !avatarLoadFailed[`${it.id}-sub`]"
                      :src="it.subject_author_avatar_url"
                      alt=""
                      class="h-full w-full object-cover"
                      @error="onAvatarError(`${it.id}-sub`)"
                    />
                    <span v-else>{{ avatarInitials(subjectAuthorLabel(it)) }}</span>
                  </RouterLink>
                  <div
                    v-else
                    class="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-xs font-bold text-neutral-700"
                  >
                    <img
                      v-if="it.subject_author_avatar_url && !avatarLoadFailed[`${it.id}-sub`]"
                      :src="it.subject_author_avatar_url"
                      alt=""
                      class="h-full w-full object-cover"
                      @error="onAvatarError(`${it.id}-sub`)"
                    />
                    <span v-else>{{ avatarInitials(subjectAuthorLabel(it)) }}</span>
                  </div>
                  <div class="flex min-w-0 flex-1 gap-2">
                    <div class="min-w-0 flex-1">
                      <div class="flex flex-wrap items-center gap-1.5">
                        <p class="truncate text-sm font-bold text-neutral-900">{{ subjectAuthorLabel(it) }}</p>
                        <UserBadges :badges="it.subject_author_badges" size="xs" />
                      </div>
                      <p v-if="it.subject_caption_preview" class="mt-0.5 line-clamp-4 text-sm text-neutral-700">
                        {{ it.subject_caption_preview }}
                      </p>
                    </div>
                    <div
                      v-if="it.subject_media_url"
                      class="relative h-14 w-14 shrink-0 overflow-hidden rounded-md border border-neutral-200 bg-neutral-100"
                    >
                      <img :src="it.subject_media_url" alt="" class="h-full w-full object-cover" />
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div
          v-else
          role="link"
          tabindex="0"
          class="flex cursor-pointer gap-3 px-4 py-3 transition-colors hover:bg-neutral-50"
          @click="goNotif(it)"
          @keydown.enter.prevent="goNotif(it)"
          @keydown.space.prevent="goNotif(it)"
        >
          <span
            class="mt-1 h-2 w-2 shrink-0 rounded-full"
            :class="it.read_at ? 'bg-transparent' : 'bg-lime-500'"
            aria-hidden="true"
          />
          <div
            v-if="it.actor_avatar_url"
            class="mt-0.5 flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-neutral-200 text-xs font-bold text-neutral-700"
          >
            <img
              v-if="!avatarLoadFailed[it.id]"
              :src="it.actor_avatar_url"
              alt=""
              class="h-full w-full object-cover"
              @error="onAvatarError(it.id)"
            />
            <span v-else>{{ avatarInitials(actorLabel(it)) }}</span>
          </div>
          <div class="min-w-0 flex-1">
            <p class="text-sm text-neutral-900">{{ rowMessage(it) }}</p>
            <p class="mt-0.5 flex flex-wrap items-center gap-1.5 text-xs text-neutral-500">
              <UserBadges :badges="it.actor_badges" size="xs" />
              {{ fullHandleAt(it.actor_handle) }} · {{ formatRelativeTime(it.created_at) }}
            </p>
          </div>
        </div>
      </li>
    </ul>
  </PullToRefresh>
</template>
