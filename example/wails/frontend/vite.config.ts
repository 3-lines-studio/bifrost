import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), wails("./bindings")],
  resolve: {
    dedupe: ["react", "react-dom"],
  },
  server: {
    cors: true,
  },
});
