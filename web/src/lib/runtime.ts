import { Capacitor } from "@capacitor/core";

const MOBILE_LIVE_MEDIA_QUERY = "(max-width: 1023px)";

export function isNativeApp(): boolean {
  try {
    return Capacitor.isNativePlatform();
  } catch {
    return false;
  }
}

export function isMobileViewport(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return false;
  return window.matchMedia(MOBILE_LIVE_MEDIA_QUERY).matches;
}

export function isLiveMobileClient(): boolean {
  return isNativeApp() || isMobileViewport();
}

