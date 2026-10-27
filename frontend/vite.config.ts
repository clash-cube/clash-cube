import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";

export default defineConfig(({ mode }) => ({
  base: mode === "web" ? "/luci-static/resources/clashcube/" : "./",
  resolve: { alias: { "@platform": new URL(mode === "web" ? "./src/web.ts" : "./src/desktop.ts", import.meta.url).pathname } },
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  build: {
    outDir: mode === "web" ? "../bin/openwrt/www" : "../internal/gui/dist",
    emptyOutDir: true,
  },
  plugins: mode === "web" ? [react()] : [react(), wails("./bindings")],
}));
