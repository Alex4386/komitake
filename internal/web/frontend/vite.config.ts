import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

// Build output goes to dist/, which the Go binary embeds. During development,
// `npm run dev` proxies /v1 to a running `komitake web`.
export default defineConfig({
  base: "/ui/",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/v1": {
        target: "http://127.0.0.1:8080",
        ws: true,
      },
    },
  },
});
