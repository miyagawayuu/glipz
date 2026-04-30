import { createApp } from "vue";
import App from "./App.vue";
import { i18n } from "./i18n";
import { router } from "./router";
import { initTheme } from "./lib/theme";
import { registerPushServiceWorker } from "./lib/webPush";
import "./plugins/sidebar";
import "./style.css";

function printConsoleSafetyWarning() {
  if (typeof console === "undefined") return;
  console.log(
    "%cSTOP!",
    "color:#65a30d;font-size:48px;font-weight:800;line-height:1.2;",
  );
  console.log(
    "%cこのコンソールにコードを貼り付けないでください。攻撃者にアカウントや個人情報を奪われる可能性があります。",
    "color:#3f6212;font-size:16px;font-weight:700;line-height:1.5;",
  );
  console.log(
    "%cGlipz の開発やデバッグ目的で開いている場合を除き、この画面は閉じてください。",
    "color:#4d7c0f;font-size:13px;line-height:1.5;",
  );
}

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

printConsoleSafetyWarning();
initTheme();
void registerPushServiceWorker();
createApp(App).use(i18n).use(router).mount("#app");
