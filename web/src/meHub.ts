import { ref } from "vue";

/** Bumping this value signals App and similar consumers to refetch `/api/v1/me`. */
export const meHubTick = ref(0);

export function bumpMeHub() {
  meHubTick.value++;
}
