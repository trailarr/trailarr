import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    middlewareMode: false,
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: "./src/setupTests.js",
  },
  appType: "spa",
  build: {
    rollupOptions: {
      output: {
        // Manual chunks to keep large libraries out of the main bundle
        manualChunks(id) {
          if (!id) return null;
          // Group deps from node_modules
          if (id.includes("node_modules")) {
            // Keep small network/util libs separate to avoid bundling CLI libs
            if (id.includes("node_modules/axios") || id.includes("node_modules/install") || id.includes("node_modules/npm")) {
              return "vendor_net";
            }
            // Default: put remaining node_modules into the React vendor chunk so
            // libraries that call React APIs (createContext, etc.) share the same runtime.
            // This avoids cross-chunk initialization order/circular issues (e.g. with MUI).
            return "vendor_react";
          }
          // App-level large pages/components — split them out so they don't bloat main
          if (id.includes("/src/components/MediaDetails")) return "page_media_details";
          if (id.includes("/src/components/ExtrasSettings")) return "page_extras_settings";
          if (id.includes("/src/components/MediaList")) return "page_media_list";
          // fall back to default
          return null;
        },
      },
    },
    // threshold for chunk size warning
    chunkSizeWarningLimit: 600,
  },
});
