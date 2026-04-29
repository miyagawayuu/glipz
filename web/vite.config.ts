import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

// Use backend:8080 from the Docker web container, or 127.0.0.1:8080 for host-based development.
const proxyTarget = process.env.VITE_PROXY_TARGET || "http://127.0.0.1:8080";
const devHost = process.env.VITE_DEV_HOST || "127.0.0.1";

export default defineConfig({
  plugins: [vue()],
  build: {
    rolldownOptions: {
      output: {
        strictExecutionOrder: true,
        codeSplitting: {
          minSize: 50 * 1024,
          maxSize: 450 * 1024,
          groups: [
            {
              name: "locales",
              test: /[\\/]src[\\/]locales[\\/]/,
              priority: 40,
              maxSize: 450 * 1024,
            },
            {
              name: "vendor-vue",
              test: /[\\/]node_modules[\\/](vue|@vue|vue-router|vue-i18n)[\\/]/,
              priority: 30,
            },
            {
              name: "vendor-scalar",
              test: /[\\/]node_modules[\\/]@scalar[\\/]/,
              priority: 25,
              maxSize: 450 * 1024,
            },
            {
              name: "vendor-codemirror",
              test: /[\\/]node_modules[\\/](@codemirror|codemirror|@lezer|lezer)[\\/]/,
              priority: 24,
              maxSize: 450 * 1024,
            },
            {
              name: "vendor-editor",
              test: /[\\/]node_modules[\\/](@tiptap|prosemirror-)[\\/]/,
              priority: 20,
              maxSize: 450 * 1024,
            },
            {
              name: "vendor-markdown",
              test: /[\\/]node_modules[\\/](dompurify|marked)[\\/]/,
              priority: 15,
            },
            {
              name: "vendor",
              test: /[\\/]node_modules[\\/]/,
              priority: 1,
              maxSize: 450 * 1024,
            },
          ],
        },
      },
    },
  },
  server: {
    host: devHost,
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
