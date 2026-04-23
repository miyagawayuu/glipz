import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { translate } from "../i18n";

type BackLinkOptions = {
  fallbackTo: string;
  registerPath?: string;
};

export function useBackLink(options: BackLinkOptions) {
  const router = useRouter();
  const route = useRoute();
  const registerPath = options.registerPath ?? "/register";

  const backPath = computed(() => {
    if (typeof window === "undefined") return "";
    const candidate = typeof window.history.state?.back === "string" ? window.history.state.back.trim() : "";
    if (!candidate || candidate === route.fullPath) return "";
    return candidate;
  });

  const label = computed(() =>
    backPath.value.startsWith(registerPath)
      ? translate("common.back.register")
      : translate("common.back.previous"),
  );

  const to = computed(() => backPath.value || options.fallbackTo);

  function onClick(event: MouseEvent) {
    if (!backPath.value) return;
    event.preventDefault();
    void router.back();
  }

  return {
    label,
    to,
    onClick,
  };
}
