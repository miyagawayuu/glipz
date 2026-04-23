import { ref } from "vue";
import { fetchDMUnreadCount } from "./lib/dm";
import type { DmStreamPayload } from "./lib/dmStream";

export const unreadDMCount = ref(0);
export const dmReceivedTick = ref(0);
export const latestDMEvent = ref<DmStreamPayload | null>(null);
export const incomingDMCall = ref<DmStreamPayload | null>(null);

export async function refreshUnreadDMCount() {
  try {
    unreadDMCount.value = await fetchDMUnreadCount();
  } catch {
    unreadDMCount.value = 0;
  }
}

export function incrementUnreadDMCount() {
  unreadDMCount.value += 1;
}

export function resetUnreadDMCount() {
  unreadDMCount.value = 0;
}

export function pingDMReceived(event: DmStreamPayload) {
  latestDMEvent.value = event;
  dmReceivedTick.value += 1;
}

export function setIncomingDMCall(event: DmStreamPayload) {
  incomingDMCall.value = event;
}

export function clearIncomingDMCall() {
  incomingDMCall.value = null;
}
