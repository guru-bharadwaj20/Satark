/// <reference types="vitest" />
import { defineConfig } from "vite";

export default defineConfig({
  server: {
    // In development, proxy API calls to a local gateway-api.
    proxy: { "/api": { target: "http://localhost:8080", rewrite: (p: string) => p.replace(/^\/api/, "") } },
  },
  test: { environment: "jsdom" },
});
