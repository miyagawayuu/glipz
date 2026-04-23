import { createApp } from "vue";
import App from "./App.vue";
import { i18n } from "./i18n";
import { router } from "./router";
import { initTheme } from "./lib/theme";
import { registerPushServiceWorker } from "./lib/webPush";
import "./style.css";

// Disable zoom gestures on mobile webviews/browsers.
if (typeof document !== "undefined") {
  document.addEventListener(
    "gesturestart",
    (e) => {
      e.preventDefault();
    },
    { passive: false },
  );
  document.addEventListener(
    "gesturechange",
    (e) => {
      e.preventDefault();
    },
    { passive: false },
  );
  document.addEventListener(
    "gestureend",
    (e) => {
      e.preventDefault();
    },
    { passive: false },
  );
}

initTheme();
void registerPushServiceWorker();
createApp(App).use(i18n).use(router).mount("#app");
