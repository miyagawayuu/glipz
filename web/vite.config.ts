import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

// Use backend:8080 from the Docker web container, or 127.0.0.1:8080 for host-based development.
const proxyTarget = process.env.VITE_PROXY_TARGET || "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [vue()],
  server: {
    host: "0.0.0.0",
    port: 5173,
    proxy: {
      "/.well-known": {
        target: proxyTarget,
        changeOrigin: true,
        timeout: 60_000,
        proxyTimeout: 60_000,
      },
      "/ap": {
        target: proxyTarget,
        changeOrigin: true,
        timeout: 60_000,
        proxyTimeout: 60_000,
      },
      "/api": {
        target: proxyTarget,
        changeOrigin: true,
        // bcrypt checks or cold database connections can exceed the default short timeout.
        timeout: 120_000,
        proxyTimeout: 120_000,
        configure(proxy) {
          proxy.on("proxyRes", (proxyRes, req) => {
            if (req.url?.includes("/posts/feed/stream") || req.url?.includes("/notifications/stream")) {
              proxyRes.headers["x-accel-buffering"] = "no";
            }
          });
        },
      },
    },
  },
});
