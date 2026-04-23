import { ref } from "vue";
import { api } from "./lib/api";
import { getAccessToken } from "./auth";

/** Unread notification count used for badges. */
export const unreadNotificationCount = ref(0);

/** Incrementing trigger used to refetch the notification list after SSE events arrive. */
export const notificationReceivedTick = ref(0);

export async function refreshUnreadNotificationCount() {
  const token = getAccessToken();
  if (!token) {
    unreadNotificationCount.value = 0;
    return;
  }
  try {
    const r = await api<{ count: number }>("/api/v1/notifications/unread-count", { method: "GET", token });
    unreadNotificationCount.value = Number(r.count) || 0;
  } catch {
    unreadNotificationCount.value = 0;
  }
}

export function incrementUnreadNotificationCount() {
  unreadNotificationCount.value += 1;
}

export function resetUnreadNotificationCount() {
  unreadNotificationCount.value = 0;
}

export function pingNotificationReceived() {
  notificationReceivedTick.value += 1;
}
